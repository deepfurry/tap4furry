package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type deliveryProbe struct {
	t     *testing.T
	calls int
}

func (p *deliveryProbe) SendEmailVerification(ctx context.Context, _, _ string) error {
	p.calls++
	deadline, ok := ctx.Deadline()
	remaining := time.Until(deadline)
	if ctx.Err() != nil || !ok || remaining < 4*time.Second || remaining > 5*time.Second {
		p.t.Fatal("post-commit delivery must detach cancellation and allow five seconds")
	}
	return errors.New("private provider failure")
}

func (p *deliveryProbe) SendPasswordReset(ctx context.Context, email, token string) error {
	return p.SendEmailVerification(ctx, email, token)
}

func TestPostCommitDeliveryBudgetAndSafeFailure(t *testing.T) {
	probe := &deliveryProbe{t: t}
	a := &App{mailer: probe}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, purpose := range []challengePurpose{purposeVerify, purposeReset} {
		if err := a.deliver(ctx, &pendingChallenge{recipient: "recipient@example.invalid", token: "synthetic-token", purpose: purpose}); err != ErrMailUnavailable || errors.Unwrap(err) != nil {
			t.Fatal("delivery error escaped application boundary")
		}
	}
	if err := a.deliver(ctx, nil); err != nil || probe.calls != 2 {
		t.Fatal("unexpected retry or empty-challenge delivery")
	}
}
