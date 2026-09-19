package public

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/gofurry/easyhash"
)

func TestIntegrationResetEnumerationAndDeliveryFailure(t *testing.T) {
	f := newFixture(t)
	id, email, cookie := f.register()
	request := func(email string) map[string]any {
		body, responseCookie := f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
		if responseCookie != nil {
			t.Fatal("reset request revealed session state")
		}
		assertKeys(t, body, []string{"message"})
		return body
	}
	accepted := request(email)
	token := f.lastToken("password_reset")
	var lifetime time.Duration
	var seconds float64
	if err := f.owner.QueryRow(t.Context(), "SELECT extract(epoch FROM expires_at-created_at) FROM app.auth_challenges WHERE user_id=$1 AND purpose='password_reset'", id).Scan(&seconds); err != nil {
		t.Fatal("reset lifetime inspection failed")
	}
	lifetime = time.Duration(seconds) * time.Second
	if lifetime != auth.ResetLifetime {
		t.Fatal("wrong reset lifetime")
	}
	if !reflect.DeepEqual(accepted, request(email)) || len(f.mail.captured()) != 2 {
		t.Fatal("reset cooldown changed response or sent twice")
	}
	if !reflect.DeepEqual(accepted, request(emailFixture())) {
		t.Fatal("nonexistent reset response reveals existence")
	}
	for _, mutation := range []string{"UPDATE app.users SET account_state='disabled' WHERE id=$1", "UPDATE app.users SET deleted_at=now() WHERE id=$1", "DELETE FROM app.password_credentials WHERE user_id=$1"} {
		otherID, otherEmail, _ := f.register()
		f.exec(mutation, otherID)
		before := len(f.mail.captured())
		if !reflect.DeepEqual(accepted, request(otherEmail)) || len(f.mail.captured()) != before {
			t.Fatal("ineligible reset reveals account state or sends mail")
		}
		// Restore the disposable OAuth-only fixture so P0-1A's global orphan check
		// remains meaningful when this package is run repeatedly on the same DB.
		if strings.HasPrefix(mutation, "DELETE") {
			hash, _ := auth.HashPassword(testPassword)
			f.exec("INSERT INTO app.password_credentials(user_id,password_hash,password_updated_at,created_at,updated_at) VALUES($1,$2,now(),now(),now())", otherID, hash)
		}
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	f.mail.fail = true
	f.exec("UPDATE app.auth_challenges SET created_at=created_at-interval '61 seconds' WHERE user_id=$1", id)
	if !reflect.DeepEqual(accepted, request(email)) {
		t.Fatal("delivery failure revealed eligibility")
	}
	if f.count("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1 AND purpose='password_reset' AND invalidated_at IS NOT NULL", id) != 1 {
		t.Fatal("failed delivery rolled back challenge replacement")
	}
	f.request("POST", "/auth/email/verification/request", nil, cookie, 503)
	before := len(f.mail.captured())
	registeredID, registeredEmail, registeredCookie := f.register()
	if len(f.mail.captured()) != before || registeredCookie == nil {
		t.Fatal("registration failed to preserve cookie after mail failure")
	}
	f.request("GET", "/me", nil, registeredCookie, 200)
	if f.count("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='account_registered'", registeredID) != 1 {
		t.Fatal("registration event lost after delivery failure")
	}
	for _, secret := range []string{email, registeredEmail, token, cookie.Value, registeredCookie.Value, testPassword} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("mail failure logged private content")
		}
	}
	// No serialized table row may contain plaintext authentication material.
	for _, table := range []string{"auth_challenges", "security_events", "password_credentials", "sessions"} {
		var rows string
		if err := f.owner.QueryRow(t.Context(), "SELECT coalesce(json_agg(t)::text,'[]') FROM app."+table+" t WHERE user_id=$1", id).Scan(&rows); err != nil {
			t.Fatal("privacy inspection failed")
		}
		for _, secret := range []string{email, token, cookie.Value, testPassword} {
			if strings.Contains(rows, secret) {
				t.Fatal("private plaintext in auth persistence")
			}
		}
	}
}

func TestIntegrationConcurrentResetAndChange(t *testing.T) {
	f := newFixture(t)
	id, email, cookie := f.register()
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	token := f.lastToken("password_reset")
	password := "concurrent replacement password"
	type outcome struct {
		grant auth.Grant
		err   error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			grant, err := f.auth.ResetPassword(t.Context(), token, password)
			results <- outcome{grant, err}
		})
	}
	wg.Wait()
	close(results)
	wins := 0
	var successful auth.Grant
	for result := range results {
		if result.err == nil {
			wins++
			successful = result.grant
		} else if !errors.Is(result.err, auth.ErrChallengeInvalid) {
			t.Fatal("concurrent reset unexpected error")
		}
	}
	if wins != 1 || f.count("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND revoked_at IS NULL AND kind='public'", id) != 1 {
		t.Fatal("concurrent reset issued more than one valid session")
	}
	f.request("GET", "/me", nil, cookie, 401)
	actor, err := f.auth.Resolve(t.Context(), successful.Token)
	if err != nil {
		t.Fatal("reset winner session invalid")
	}
	// Consuming a token does not bypass the per-identity issuance cooldown.
	before := len(f.mail.captured())
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	if len(f.mail.captured()) != before {
		t.Fatal("consume bypasses cooldown")
	}
	results = make(chan outcome, 2)
	for range 2 {
		wg.Go(func() {
			grant, err := f.auth.ChangePassword(t.Context(), actor, password, testPassword)
			results <- outcome{grant, err}
		})
	}
	wg.Wait()
	close(results)
	wins = 0
	for result := range results {
		if result.err == nil {
			wins++
		} else if !errors.Is(result.err, auth.ErrUnauthenticated) && !errors.Is(result.err, auth.ErrReauthFailed) {
			t.Fatal("concurrent change unexpected error")
		}
	}
	if wins != 1 || f.count("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND revoked_at IS NULL AND kind='public'", id) != 1 {
		t.Fatal("concurrent change issued multiple valid sessions")
	}
	var stored string
	if err := f.owner.QueryRow(t.Context(), "SELECT password_hash FROM app.password_credentials WHERE user_id=$1", id).Scan(&stored); err != nil {
		t.Fatal("credential inspection failed")
	}
	if algorithm, _ := easyhash.Identify(stored); algorithm != easyhash.AlgorithmArgon2id {
		t.Fatal("password change did not store Argon2id")
	}
	if _, err := f.auth.Login(t.Context(), email, password); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatal("old password accepted after change")
	}
	if f.count("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type IN ('password_reset_completed','password_changed')", id) != 2 {
		t.Fatal("rotation events not atomic")
	}
}

func TestIntegrationResetRollbackAndExpiredTokens(t *testing.T) {
	f := newFixture(t)
	id, email, cookie := f.register()
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	token := f.lastToken("password_reset")
	// Force the final event write to fail only for this random fixture. Earlier
	// password/challenge/session writes in the same transaction must roll back.
	f.exec("ALTER TABLE app.security_events ADD CONSTRAINT reset_atomicity_fixture CHECK (user_id <> '" + id + "'::uuid OR event_type <> 'password_reset_completed')")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := f.owner.Exec(ctx, "ALTER TABLE app.security_events DROP CONSTRAINT IF EXISTS reset_atomicity_fixture"); err != nil {
			t.Error("disposable constraint cleanup failed")
		}
	})
	f.request("POST", "/auth/password/reset", map[string]string{"token": token, "new_password": "rollback new password"}, nil, 500)
	f.request("GET", "/me", nil, cookie, 200)
	if f.count("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1 AND purpose='password_reset' AND consumed_at IS NULL AND invalidated_at IS NULL", id) != 1 {
		t.Fatal("failed reset consumed challenge")
	}
	if _, err := f.auth.Login(t.Context(), email, testPassword); err != nil {
		t.Fatal("failed reset changed password")
	}
	f.exec("ALTER TABLE app.security_events DROP CONSTRAINT reset_atomicity_fixture")
	f.exec("UPDATE app.auth_challenges SET created_at=now()-interval '1 hour',expires_at=now()-interval '1 second' WHERE user_id=$1 AND purpose='password_reset'", id)
	expired, _ := f.request("POST", "/auth/password/reset", map[string]string{"token": token, "new_password": testPassword}, nil, 400)
	unknown, _ := easyhash.GenerateToken()
	invalid, _ := f.request("POST", "/auth/password/reset", map[string]string{"token": unknown, "new_password": testPassword}, nil, 400)
	if !reflect.DeepEqual(expired, invalid) || expired["code"] != "AUTH_CHALLENGE_INVALID" {
		t.Fatal("reset invalid/expired errors differ")
	}
	for _, mutation := range []string{"UPDATE app.users SET account_state='disabled' WHERE id=$1", "UPDATE app.users SET deleted_at=now() WHERE id=$1"} {
		otherID, otherEmail, _ := f.register()
		f.request("POST", "/auth/password/reset/request", map[string]string{"email": otherEmail}, nil, 202)
		bad := f.lastToken("password_reset")
		f.exec(mutation, otherID)
		body, _ := f.request("POST", "/auth/password/reset", map[string]string{"token": bad, "new_password": testPassword}, nil, 400)
		if !reflect.DeepEqual(invalid, body) {
			t.Fatal("inactive account reset error differs")
		}
	}
}

func TestIntegrationSecuritySchemaAndPrivileges(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	var columns string
	if err := f.owner.QueryRow(ctx, `SELECT string_agg(column_name,',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='app' AND table_name='security_events'`).Scan(&columns); err != nil || columns != "id,user_id,session_id,event_type,occurred_at" {
		t.Fatal("security events permit private/arbitrary payload")
	}
	for _, table := range []string{"auth_challenges", "security_events"} {
		var readonly, apiDelete bool
		if err := f.owner.QueryRow(ctx, "SELECT has_table_privilege('gfp_readonly',$1,'SELECT'),has_table_privilege('gfp_api',$1,'DELETE,TRUNCATE,TRIGGER')", "app."+table).Scan(&readonly, &apiDelete); err != nil || !readonly || apiDelete {
			t.Fatal("security table privileges too broad or readonly missing")
		}
	}
	var eventRead, eventUpdate, sequence, identityUpdate bool
	if err := f.owner.QueryRow(ctx, `SELECT has_table_privilege('gfp_api','app.security_events','SELECT'),has_table_privilege('gfp_api','app.security_events','UPDATE'),has_sequence_privilege('gfp_api','app.security_events_id_seq','USAGE,SELECT,UPDATE'),has_column_privilege('gfp_api','app.auth_identities','provider_subject','UPDATE')`).Scan(&eventRead, &eventUpdate, &sequence, &identityUpdate); err != nil || eventRead || eventUpdate || sequence || identityUpdate {
		t.Fatal("unnecessary event/identity privileges")
	}
	id, _, cookie := f.register()
	// Historical event session_id deliberately has no FK and survives deletion
	// of the referenced session; user references remain restricted.
	sessionID := f.sessionID(cookie)
	f.request("POST", "/auth/logout", nil, cookie, 204)
	f.exec("DELETE FROM app.sessions WHERE id=$1", sessionID)
	if f.count("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND session_id=$2", id, sessionID) < 1 {
		t.Fatal("historical session reference disappeared")
	}
	for _, query := range []string{
		"UPDATE app.auth_challenges SET consumed_at=now(),invalidated_at=now() WHERE user_id=$1",
		"UPDATE app.auth_challenges SET expires_at=created_at WHERE user_id=$1",
		"UPDATE app.auth_challenges SET purpose='arbitrary' WHERE user_id=$1",
		"UPDATE app.security_events SET event_type='arbitrary' WHERE user_id=$1",
		"INSERT INTO app.auth_challenges SELECT gen_random_uuid(),user_id,auth_identity_id,purpose,token_hash || 'other',created_at,expires_at,NULL,NULL FROM app.auth_challenges WHERE user_id=$1",
	} {
		if _, err := f.owner.Exec(ctx, query, id); err == nil {
			t.Fatal("security database invariant absent")
		}
	}
}

// Check a login racing a reset cannot escape with an old-password session.
func TestIntegrationLoginRacingReset(t *testing.T) {
	f := newFixture(t)
	id, email, _ := f.register()
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	token := f.lastToken("password_reset")
	var wg sync.WaitGroup
	var login auth.Grant
	var loginErr, resetErr error
	wg.Go(func() { login, loginErr = f.auth.Login(context.Background(), email, testPassword) })
	wg.Go(func() { _, resetErr = f.auth.ResetPassword(context.Background(), token, "racing reset new password") })
	wg.Wait()
	if resetErr != nil {
		t.Fatal("racing reset failed")
	}
	if loginErr == nil {
		if _, err := f.auth.Resolve(t.Context(), login.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatal("old-password login escaped reset revocation")
		}
	} else if !errors.Is(loginErr, auth.ErrInvalidCredentials) {
		t.Fatal("racing login returned unexpected error")
	}
	if f.count("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND revoked_at IS NULL AND kind='public'", id) != 1 {
		t.Fatal("racing reset left extra public session")
	}
}
