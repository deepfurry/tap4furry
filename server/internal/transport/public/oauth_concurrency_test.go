package public

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
)

func TestIntegrationOAuthRacingPasswordMutation(t *testing.T) {
	for _, mutation := range []string{"reset", "change"} {
		for _, winner := range []string{"password", "oauth"} {
			t.Run(mutation+"-wins-"+winner, func(t *testing.T) {
				f := newOAuthFixture(t)
				ctx := t.Context()
				local, err := f.auth.Register(ctx, emailFixture(), testPassword)
				if err != nil {
					t.Fatal("local fixture failed")
				}
				actor := f.actorFor(local)
				google := fakeIdentity(auth.Google)
				linked, err := f.complete(f.begin(auth.Google, auth.OAuthLink, &actor), google, &actor)
				if err != nil {
					t.Fatal("link fixture failed")
				}
				actor = f.actorFor(linked.Grant)
				f.auth.RequestPasswordReset(ctx, local.Me.Email)
				resetToken := f.lastToken("password_reset")
				start := f.begin(auth.Google, auth.OAuthLogin, nil)
				provider := f.providers[auth.Google]
				provider.entered, provider.resume = make(chan struct{}), make(chan struct{})
				type outcome struct {
					result auth.OAuthResult
					err    error
				}
				done := make(chan outcome, 1)
				go func() { result, err := f.complete(start, google, nil); done <- outcome{result, err} }()
				select {
				case <-provider.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("provider exchange did not start")
				}
				var login outcome
				if winner == "oauth" {
					close(provider.resume)
					login = <-done
					if login.err != nil {
						t.Fatal("OAuth-first login failed")
					}
				}
				var replacement auth.Grant
				if mutation == "reset" {
					replacement, err = f.auth.ResetPassword(ctx, resetToken, "a new racing password 🦊")
				} else {
					replacement, err = f.auth.ChangePassword(ctx, actor, testPassword, "a new racing password 🦊")
				}
				if err != nil {
					t.Fatal("password mutation failed while provider exchange was outside transaction")
				}
				if winner == "password" {
					close(provider.resume)
					login = <-done
					if !errors.Is(login.err, auth.ErrReauthRequired) {
						t.Fatal("in-flight OAuth survived a newer password epoch")
					}
				}
				if login.err == nil {
					if _, err = f.auth.Resolve(ctx, login.result.Grant.Token); !errors.Is(err, auth.ErrUnauthenticated) {
						t.Fatal("OAuth session escaped password revocation")
					}
				}
				if f.scalar("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND kind='public' AND revoked_at IS NULL", local.Me.ID.String()) != 1 {
					t.Fatal("password mutation left extra session")
				}
				f.actorFor(replacement)
			})
		}
	}
}

func TestIntegrationOAuthConcurrentAccountCreation(t *testing.T) {
	for _, sameSubject := range []bool{true, false} {
		t.Run(map[bool]string{true: "same-subject", false: "same-email"}[sameSubject], func(t *testing.T) {
			f := newOAuthFixture(t)
			first := fakeIdentity(auth.Google)
			second := first
			if !sameSubject {
				second = fakeIdentity(auth.GitHub)
				second.Email = first.Email
			}
			starts := []auth.OAuthStart{f.begin(first.Provider, auth.OAuthLogin, nil), f.begin(second.Provider, auth.OAuthLogin, nil)}
			identities := []auth.ProviderIdentity{first, second}
			var results [2]auth.OAuthResult
			var errs [2]error
			var wg sync.WaitGroup
			gate := make(chan struct{})
			for i := range 2 {
				wg.Go(func() { <-gate; results[i], errs[i] = f.complete(starts[i], identities[i], nil) })
			}
			close(gate)
			wg.Wait()
			successes := 0
			for _, err := range errs {
				if err == nil {
					successes++
				} else if !errors.Is(err, auth.ErrAccountLinkRequired) {
					t.Fatal("concurrent creation unsafe error")
				}
			}
			if sameSubject {
				if successes != 2 || results[0].Grant.Me.ID != results[1].Grant.Me.ID {
					t.Fatal("same subject created different accounts")
				}
			} else if successes != 1 {
				t.Fatal("same email created multiple accounts")
			}
			if f.scalar("SELECT count(*) FROM app.auth_identities WHERE provider='email' AND provider_subject=$1", first.Email) != 1 {
				t.Fatal("canonical email duplicated")
			}
			if f.scalar("SELECT count(*) FROM app.auth_identities WHERE provider=$1 AND provider_subject=$2", string(first.Provider), first.Subject) > 1 {
				t.Fatal("provider identity duplicated")
			}
		})
	}
}

func TestIntegrationOAuthConcurrentLinkAndUnlink(t *testing.T) {
	f := newOAuthFixture(t)
	ctx := t.Context()
	local, err := f.auth.Register(ctx, emailFixture(), testPassword)
	if err != nil {
		t.Fatal("local fixture failed")
	}
	actor := f.actorFor(local)
	google := fakeIdentity(auth.Google)
	starts := []auth.OAuthStart{f.begin(auth.Google, auth.OAuthLink, &actor), f.begin(auth.Google, auth.OAuthLink, &actor)}
	var results [2]auth.OAuthResult
	var errs [2]error
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := range 2 {
		wg.Go(func() { <-gate; results[i], errs[i] = f.complete(starts[i], google, &actor) })
	}
	close(gate)
	wg.Wait()
	successes := 0
	var current auth.Actor
	for i, err := range errs {
		if err == nil {
			successes++
			current = f.actorFor(results[i].Grant)
		} else if !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatal("concurrent link unsafe error")
		}
	}
	if successes != 1 || f.scalar("SELECT count(*) FROM app.auth_identities WHERE user_id=$1 AND provider='google'", local.Me.ID.String()) != 1 {
		t.Fatal("two link callbacks created duplicate identity")
	}
	// Independent password sessions allow both orderings; the common User lock
	// gives a serial result and never moves the provider or revives revoked IDs.
	other, err := f.auth.Login(ctx, local.Me.Email, testPassword)
	if err != nil {
		t.Fatal("second password session failed")
	}
	otherActor := f.actorFor(other)
	relink := f.begin(auth.Google, auth.OAuthLink, &otherActor)
	var relinked auth.OAuthResult
	var unlinked auth.Grant
	var linkErr, unlinkErr error
	wg.Go(func() { relinked, linkErr = f.complete(relink, google, &otherActor) })
	wg.Go(func() { unlinked, unlinkErr = f.auth.UnlinkProvider(ctx, current, auth.Google) })
	wg.Wait()
	if linkErr != nil || unlinkErr != nil {
		t.Fatal("link versus unlink did not serialize")
	}
	f.actorFor(relinked.Grant)
	f.actorFor(unlinked)
	if f.scalar("SELECT count(*) FROM app.auth_identities WHERE provider='google' AND provider_subject=$1 AND user_id<>$2", google.Subject, local.Me.ID.String()) != 0 {
		t.Fatal("provider identity moved user")
	}
	if f.scalar("SELECT count(*) FROM app.auth_identities WHERE user_id=$1 AND provider='google'", local.Me.ID.String()) > 1 {
		t.Fatal("link/unlink duplicated provider")
	}
}
