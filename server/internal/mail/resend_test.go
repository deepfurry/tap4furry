package mail

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/resend/resend-go/v3"
)

type emailServiceFunc func(context.Context, *resend.SendEmailRequest) (*resend.SendEmailResponse, error)

func (f emailServiceFunc) SendWithContext(ctx context.Context, req *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
	return f(ctx, req)
}

func TestResendMessages(t *testing.T) {
	for _, test := range []struct{ name, subject, path, explanation, action, lifetime string }{
		{"verification", "Verify your Tap4Furry email", "/verify-email", "Verify your Tap4Furry email.", "Verify email", "24 hours"},
		{"reset", "Reset your Tap4Furry password", "/reset-password", "A password reset was requested for your Tap4Furry account.", "Reset password", "30 minutes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, err := NewResend("test-only-key", "Tap4Furry <sender@example.invalid>", "reply@example.invalid", "https://example.invalid")
			if err != nil {
				t.Fatal("valid constructor input rejected")
			}
			const token = "synthetic<&\"?#/+ token"
			link := "https://example.invalid" + test.path + "#token=synthetic%3C%26%22%3F%23%2F%2B+token"
			calls := 0
			r.emails = emailServiceFunc(func(ctx context.Context, req *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
				calls++
				deadline, ok := ctx.Deadline()
				if remaining := time.Until(deadline); !ok || remaining <= 0 || remaining > 5*time.Second {
					t.Fatal("provider request lacks bounded context")
				}
				expected := &resend.SendEmailRequest{
					From: "Tap4Furry <sender@example.invalid>", ReplyTo: "reply@example.invalid", To: []string{"recipient@example.invalid"}, Subject: test.subject,
					Text: test.explanation + "\n\nThe link expires in " + test.lifetime + ".\n\n" + link + "\n\nIf you did not request this, you can ignore this message.\n",
					Html: "<!doctype html><html><body><h1>Tap4Furry</h1><p>" + test.explanation + "</p><p><a href=\"" + html.EscapeString(link) + "\">" + test.action + "</a></p><p>The link expires in " + test.lifetime + ".</p><p>If you did not request this, you can ignore this message.</p></body></html>",
				}
				// Full struct equality also excludes CC/BCC, attachments, tags, templates,
				// scheduling, headers and other provider metadata.
				if !reflect.DeepEqual(req, expected) {
					t.Fatal("unexpected provider fields (payload withheld)")
				}
				for _, prohibited := range []string{"<img", "<script", "tracking", "?token="} {
					if strings.Contains(req.Html, prohibited) {
						t.Fatal("unsafe email content")
					}
				}
				u, err := url.Parse(link)
				if err != nil || u.RawQuery != "" || u.Path != test.path {
					t.Fatal("incorrect link destination")
				}
				fragment, err := url.ParseQuery(u.EscapedFragment())
				if err != nil || fragment.Get("token") != token || len(fragment) != 1 {
					t.Fatal("token did not survive fragment encoding")
				}
				return &resend.SendEmailResponse{Id: "fixture-id"}, nil
			})
			send := r.SendEmailVerification
			if test.name == "reset" {
				send = r.SendPasswordReset
			}
			if err := send(t.Context(), "recipient@example.invalid", token); err != nil || calls != 1 {
				t.Fatal("expected exactly one successful send")
			}
		})
	}
}

func TestResendFailuresAreFlattenedWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *resend.SendEmailResponse
		err      error
	}{
		{"provider", nil, errors.New("private recipient, key, token and provider body")},
		{"canceled-provider", nil, context.Canceled},
		{"nil-response", nil, nil},
		{"missing-id", &resend.SendEmailResponse{}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			r := &Resend{origin: "http://localhost:4321", emails: emailServiceFunc(func(context.Context, *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
				calls++
				return test.response, test.err
			})}
			for _, send := range []func(context.Context, string, string) error{r.SendEmailVerification, r.SendPasswordReset} {
				err := send(t.Context(), "recipient@example.invalid", "synthetic-token")
				if err != errDelivery || errors.Unwrap(err) != nil {
					t.Fatal("provider failure escaped mail boundary")
				}
			}
			if calls != 2 {
				t.Fatal("provider failure caused a retry")
			}
		})
	}
}

func TestResendCancellationAndDeadline(t *testing.T) {
	calls := 0
	r := &Resend{origin: "http://localhost:4321", emails: emailServiceFunc(func(ctx context.Context, _ *resend.SendEmailRequest) (*resend.SendEmailResponse, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	})}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := r.SendEmailVerification(ctx, "recipient@example.invalid", "synthetic-token"); err != errDelivery || calls != 0 {
		t.Fatal("canceled context still sent mail")
	}
	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := r.SendPasswordReset(ctx, "recipient@example.invalid", "synthetic-token"); err != errDelivery || calls != 1 || ctx.Err() != context.DeadlineExceeded {
		t.Fatal("earlier caller deadline was not respected")
	}
}

func TestResendRejectsUnsafeConfiguration(t *testing.T) {
	base := []string{"test-only-key", "sender@example.invalid", "reply@example.invalid", "https://example.invalid"}
	for i, invalid := range [][]string{
		{"", "   "},
		{"", "invalid", "one@example.invalid,two@example.invalid", "sender@example.invalid\r\nBcc: other@example.invalid"},
		{"", "invalid", "reply@example.invalid>"},
		{"", "https://private@example.invalid", "https://example.invalid/", "https://example.invalid?", "https://example.invalid?token=value", "https://example.invalid#token=value", "javascript:alert(1)"},
	} {
		for _, value := range invalid {
			args := append([]string(nil), base...)
			args[i] = value
			if _, err := NewResend(args[0], args[1], args[2], args[3]); err != errDelivery {
				t.Fatal("unsafe adapter input accepted or error disclosed")
			}
		}
	}
}

type mailTransportFunc func(*http.Request) (*http.Response, error)

func (f mailTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestResendSDKUsesFixedEndpointWithoutRedirectOrRetry(t *testing.T) {
	// Exercise the real SDK with an in-memory transport; no provider network I/O.
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	calls := 0
	http.DefaultTransport = mailTransportFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "POST" || req.URL.String() != "https://api.resend.com/emails" || req.Header.Get("Authorization") != "Bearer test-only-key" {
			t.Fatal("SDK request destination or authentication differs")
		}
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect, Header: http.Header{"Location": []string{"https://api.resend.com/unexpected"}},
			Body: io.NopCloser(strings.NewReader(`{"message":"private provider payload"}`)), Request: req,
		}, nil
	})
	r, err := NewResend("test-only-key", "sender@example.invalid", "reply@example.invalid", "https://example.invalid")
	if err != nil {
		t.Fatal("constructor failed")
	}
	if err := r.SendEmailVerification(t.Context(), "recipient@example.invalid", "synthetic-token"); err != errDelivery || calls != 1 {
		t.Fatal("SDK redirected/retried or leaked a provider error")
	}
}
