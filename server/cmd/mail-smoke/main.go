// mail-smoke sends one synthetic verification message, without application state.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/mail"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Resend mail smoke FAIL (private values withheld)")
		os.Exit(1)
	}
	fmt.Println("Resend mail smoke PASS (one synthetic verification email accepted)")
}

func run() error {
	if os.Getenv("CI") != "" || os.Getenv("RESEND_SMOKE_OPT_IN") != "1" {
		return errors.New("explicit local smoke opt-in required")
	}
	mailer, err := mail.NewResend(os.Getenv("RESEND_API_KEY"), os.Getenv("MAIL_FROM"), os.Getenv("MAIL_REPLY_TO"), os.Getenv("PUBLIC_ORIGIN"))
	if err != nil {
		return err
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return errors.New("synthetic token unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mailer.SendEmailVerification(ctx, os.Getenv("RESEND_TEST_RECIPIENT"), base64.RawURLEncoding.EncodeToString(raw[:]))
}
