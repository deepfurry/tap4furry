package mail

import (
	"context"
	"html"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
)

// emailService keeps provider requests inside this package and permits offline tests.
type emailService interface {
	SendWithContext(context.Context, *resend.SendEmailRequest) (*resend.SendEmailResponse, error)
}

type Resend struct {
	emails                emailService
	from, replyTo, origin string
}

func NewResend(apiKey, from, replyTo, publicOrigin string) (*Resend, error) {
	if strings.TrimSpace(apiKey) == "" || !validMailbox(from) || !validMailbox(replyTo) {
		return nil, errDelivery
	}
	u, err := url.Parse(publicOrigin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errDelivery
	}
	client := resend.NewCustomClient(&http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, apiKey)
	// Do not inherit the SDK's RESEND_BASE_URL override for credential-bearing mail.
	client.BaseURL = &url.URL{Scheme: "https", Host: "api.resend.com", Path: "/"}
	return &Resend{emails: client.Emails, from: from, replyTo: replyTo, origin: publicOrigin}, nil
}

func validMailbox(value string) bool {
	_, err := mail.ParseAddress(value)
	return err == nil && !strings.ContainsAny(value, "\r\n")
}

func (r *Resend) SendEmailVerification(ctx context.Context, email, token string) error {
	return r.send(ctx, email, token, "Verify your Tap4Furry email", "/verify-email", "Verify your Tap4Furry email.", "Verify email", "24 hours")
}

func (r *Resend) SendPasswordReset(ctx context.Context, email, token string) error {
	return r.send(ctx, email, token, "Reset your Tap4Furry password", "/reset-password", "A password reset was requested for your Tap4Furry account.", "Reset password", "30 minutes")
}

func (r *Resend) send(ctx context.Context, email, token, subject, path, explanation, action, lifetime string) error {
	if ctx.Err() != nil || !validMailbox(email) || token == "" {
		return errDelivery
	}
	link := r.origin + path + "#token=" + url.QueryEscape(token)
	expiry := "The link expires in " + lifetime + "."
	ignore := "If you did not request this, you can ignore this message."
	message := &resend.SendEmailRequest{
		From: r.from, ReplyTo: r.replyTo, To: []string{email}, Subject: subject,
		Text: explanation + "\n\n" + expiry + "\n\n" + link + "\n\n" + ignore + "\n",
		Html: "<!doctype html><html><body><h1>Tap4Furry</h1><p>" + html.EscapeString(explanation) + "</p><p><a href=\"" + html.EscapeString(link) + "\">" + html.EscapeString(action) + "</a></p><p>" + html.EscapeString(expiry) + "</p><p>" + ignore + "</p></body></html>",
	}
	// One bounded attempt. Neither payload nor provider errors leave the adapter.
	deliveryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := r.emails.SendWithContext(deliveryCtx, message)
	if err != nil || deliveryCtx.Err() != nil || response == nil || response.Id == "" {
		return errDelivery
	}
	return nil
}
