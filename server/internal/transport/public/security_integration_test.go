package public

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/config"
	"github.com/gofurry/easyhash"
)

type capturedMail struct{ email, token, purpose string }
type fakeMailer struct {
	mu          sync.Mutex
	messages    []capturedMail
	fail        bool
	afterCommit func(capturedMail)
}

func (m *fakeMailer) SendEmailVerification(_ context.Context, email, token string) error {
	return m.send(email, token, "email_verify")
}
func (m *fakeMailer) SendPasswordReset(_ context.Context, email, token string) error {
	return m.send(email, token, "password_reset")
}
func (m *fakeMailer) send(email, token, purpose string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg := capturedMail{email, token, purpose}
	if m.afterCommit != nil {
		m.afterCommit(msg)
	}
	if m.fail {
		// Deliberately sensitive sender error: the application must never log it.
		return errors.New(email + " " + token + " " + testPassword)
	}
	m.messages = append(m.messages, msg)
	return nil
}
func (m *fakeMailer) captured() []capturedMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]capturedMail(nil), m.messages...)
}

func (f *fixture) register() (string, string, *http.Cookie) {
	f.t.Helper()
	email := emailFixture()
	me, cookie := f.request("POST", "/auth/register", map[string]string{"email": email, "password": testPassword}, nil, 201)
	return me["id"].(string), email, cookie
}
func (f *fixture) count(query string, args ...any) int {
	f.t.Helper()
	var count int
	if err := f.owner.QueryRow(f.t.Context(), query, args...).Scan(&count); err != nil {
		f.t.Fatal("private state inspection failed (values withheld)")
	}
	return count
}
func (f *fixture) lastToken(purpose string) string {
	f.t.Helper()
	messages := f.mail.captured()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].purpose == purpose {
			return messages[i].token
		}
	}
	f.t.Fatal("challenge was not delivered")
	return ""
}
func (f *fixture) sessionID(cookie *http.Cookie) string {
	f.t.Helper()
	actor, err := f.auth.Resolve(f.t.Context(), cookie.Value)
	if err != nil {
		f.t.Fatal("session fixture invalid")
	}
	return actor.SessionID.String()
}

func TestIntegrationCSRF(t *testing.T) {
	f := newFixture(t)
	_, _, cookie := f.register()
	_, _, other := f.register()
	view, _ := f.request("GET", "/auth/csrf", nil, cookie, 200)
	assertKeys(t, view, []string{"csrf_token"})
	value := view["csrf_token"].(string)
	if value == cookie.Value || value != csrfToken(config.DevelopmentCSRFSecret, cookie.Value) {
		t.Fatal("CSRF binding exposes session or differs from HMAC")
	}
	f.request("GET", "/auth/csrf", nil, nil, 401)
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/auth/logout"}, {"PATCH", "/me/profile"}, {"POST", "/auth/email/verification/request"},
		{"POST", "/auth/password/change"}, {"POST", "/auth/reauthenticate"}, {"DELETE", "/me/sessions/" + uuid.NewV7().String()}, {"POST", "/me/sessions/revoke-others"},
		{"POST", "/AUTH/LOGOUT"}, {"PATCH", "/ME/PROFILE/"},
	} {
		for _, header := range []string{"", "incorrect", csrfToken(config.DevelopmentCSRFSecret, other.Value)} {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			req.Header.Set("Origin", testOrigin)
			req.Header.Set("X-CSRF-Token", header)
			req.AddCookie(cookie)
			response, err := f.app.Test(req)
			if err != nil {
				t.Fatal("CSRF request failed")
			}
			var body map[string]any
			_ = json.NewDecoder(response.Body).Decode(&body)
			response.Body.Close()
			if response.StatusCode != 403 || body["code"] != "CSRF_INVALID" {
				t.Fatal("authenticated unsafe route bypasses session CSRF")
			}
		}
	}
	for _, path := range []string{"/auth/password/reset/request", "/auth/password/reset", "/auth/email/verification", "/auth/password/change", "/auth/reauthenticate", "/me/sessions/revoke-others"} {
		for _, origin := range []string{"", "https://attacker.invalid"} {
			req := httptest.NewRequest("POST", path, nil)
			req.AddCookie(cookie)
			req.Header.Set("Origin", origin)
			req.Header.Set("X-CSRF-Token", value)
			res, err := f.app.Test(req)
			if err != nil {
				t.Fatal("Origin request failed")
			}
			res.Body.Close()
			if res.StatusCode != 403 {
				t.Fatal("new route bypasses Origin")
			}
		}
	}
	// Safe requests work without a CSRF header; the helper's header is not used.
	for _, path := range []string{"/me", "/me/sessions", "/auth/csrf"} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(cookie)
		res, err := f.app.Test(req)
		if err != nil {
			t.Fatal("safe request failed")
		}
		res.Body.Close()
		if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("safe route requires CSRF or permits caching")
		}
	}
}

func TestIntegrationVerificationChallenges(t *testing.T) {
	f := newFixture(t)
	// A separate pooled connection must see the committed challenge and account
	// during delivery, proving that SMTP-like work is outside the transaction.
	f.mail.afterCommit = func(msg capturedMail) {
		hash, _ := easyhash.HashToken(msg.token)
		if f.count(`SELECT count(*) FROM app.auth_challenges ch JOIN app.auth_identities a ON a.id=ch.auth_identity_id JOIN app.sessions s ON s.user_id=ch.user_id WHERE ch.token_hash=$1 AND a.email=$2`, hash, msg.email) != 1 {
			t.Fatal("mail sent before commit")
		}
	}
	id, _, cookie := f.register()
	f.mail.afterCommit = nil
	token := f.lastToken("email_verify")
	var stored, challengeID string
	var created, expires time.Time
	if err := f.owner.QueryRow(t.Context(), "SELECT id,token_hash,created_at,expires_at FROM app.auth_challenges WHERE user_id=$1", id).Scan(&challengeID, &stored, &created, &expires); err != nil {
		t.Fatal("challenge inspection failed")
	}
	parsed, err := uuid.Parse(challengeID)
	if err != nil || parsed[6]>>4 != 7 || stored == token || strings.Contains(stored, token) || expires.Sub(created) != auth.VerificationLifetime {
		t.Fatal("challenge persistence or lifetime invalid")
	}
	if ok, err := easyhash.VerifyToken(token, stored); err != nil || !ok {
		t.Fatal("challenge is not an easyhash token hash")
	}
	f.request("POST", "/auth/email/verification/request", nil, cookie, 202)
	if len(f.mail.captured()) != 1 {
		t.Fatal("cooldown delivered another message")
	}
	f.exec("UPDATE app.auth_challenges SET created_at=created_at-interval '61 seconds' WHERE user_id=$1", id)
	f.request("POST", "/auth/email/verification/request", nil, cookie, 202)
	replacement := f.lastToken("email_verify")
	if replacement == token || f.count("SELECT count(*) FROM app.auth_challenges WHERE user_id=$1 AND invalidated_at IS NOT NULL", id) != 1 {
		t.Fatal("reissue did not invalidate old token")
	}
	invalid, _ := f.request("POST", "/auth/email/verification", map[string]string{"token": token}, nil, 400)
	if invalid["code"] != "AUTH_CHALLENGE_INVALID" {
		t.Fatal("invalidated link error unstable")
	}
	for _, bad := range []string{"", "short", strings.Repeat("!", 43)} {
		f.request("POST", "/auth/email/verification", map[string]string{"token": bad}, nil, 400)
	}
	unknown, _ := easyhash.GenerateToken()
	f.request("POST", "/auth/email/verification", map[string]string{"token": unknown}, nil, 400)
	// One challenge may succeed only once even if two HTTP requests overlap.
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { results <- f.auth.VerifyEmail(t.Context(), replacement) })
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, auth.ErrChallengeInvalid) {
			t.Fatal("concurrent verification unexpected error")
		}
	}
	if wins != 1 {
		t.Fatal("verification not single-use")
	}
	me, _ := f.request("GET", "/me", nil, cookie, 200)
	if me["email_verified"] != true {
		t.Fatal("verification not reflected in me")
	}
	f.request("POST", "/auth/email/verification", map[string]string{"token": replacement}, nil, 400)
	f.request("POST", "/auth/email/verification/request", nil, cookie, 202)
	if len(f.mail.captured()) != 2 {
		t.Fatal("already verified identity mailed again")
	}
	if f.count("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type='email_verified'", id) != 1 {
		t.Fatal("verification event not atomic/exactly once")
	}
	// Expiry and inactive accounts must be rechecked on consume.
	for _, mutation := range []string{"UPDATE app.auth_challenges SET created_at=now()-interval '2 days',expires_at=now()-interval '1 second' WHERE user_id=$1", "UPDATE app.users SET account_state='disabled' WHERE id=$1", "UPDATE app.users SET deleted_at=now() WHERE id=$1"} {
		otherID, _, _ := f.register()
		bad := f.lastToken("email_verify")
		f.exec(mutation, otherID)
		f.request("POST", "/auth/email/verification", map[string]string{"token": bad}, nil, 400)
	}
}

func TestIntegrationRecoveryRotationAndSessions(t *testing.T) {
	f := newFixture(t)
	id, email, first := f.register()
	_, second := f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
	_, _, foreign := f.register()
	foreignID := f.sessionID(foreign)
	missing, _ := f.request("DELETE", "/me/sessions/"+foreignID, nil, first, 404)
	missingOther, _ := f.request("DELETE", "/me/sessions/"+uuid.NewV7().String(), nil, first, 404)
	if !reflect.DeepEqual(missing, missingOther) || missing["code"] != "AUTH_SESSION_NOT_FOUND" {
		t.Fatal("session ownership oracle")
	}
	sessions, _ := f.request("GET", "/me/sessions", nil, first, 200)
	assertKeys(t, sessions, []string{"sessions"})
	listed := sessions["sessions"].([]any)
	if len(listed) != 2 {
		t.Fatal("session list includes another user or omits own")
	}
	currentCount := 0
	for _, value := range listed {
		session := value.(map[string]any)
		assertKeys(t, session, []string{"id", "auth_method", "authenticated_at", "created_at", "last_seen_at", "idle_expires_at", "absolute_expires_at", "current"})
		if session["current"] == true {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatal("current session not unique")
	}
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	reset := f.lastToken("password_reset")
	f.request("POST", "/auth/email/verification", map[string]string{"token": reset}, nil, 400)
	f.request("POST", "/auth/password/reset", map[string]string{"token": f.mail.captured()[0].token, "new_password": testPassword}, nil, 400)
	f.request("POST", "/auth/password/reset", map[string]string{"token": reset, "new_password": "short"}, nil, 400)
	newPassword := "replacement password value 🦊"
	me, resetCookie := f.request("POST", "/auth/password/reset", map[string]string{"token": reset, "new_password": newPassword}, nil, 200)
	assertMePrivacy(t, me)
	if me["email_verified"] != false || resetCookie == nil || resetCookie.Value == first.Value {
		t.Fatal("reset verified email or failed rotation")
	}
	for _, old := range []*http.Cookie{first, second} {
		f.request("GET", "/me", nil, old, 401)
	}
	f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 401)
	f.request("POST", "/auth/password/reset", map[string]string{"token": reset, "new_password": newPassword}, nil, 400)
	if f.count("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND revoked_at IS NULL AND auth_method='password_reset'", id) != 1 {
		t.Fatal("reset replacement session contract")
	}
	// A new outstanding reset is invalidated by an authenticated password change.
	f.exec("UPDATE app.auth_challenges SET created_at=created_at-interval '61 seconds' WHERE user_id=$1 AND purpose='password_reset'", id)
	f.request("POST", "/auth/password/reset/request", map[string]string{"email": email}, nil, 202)
	pending := f.lastToken("password_reset")
	change := map[string]string{"current_password": "wrong current password", "new_password": testPassword}
	errBody, _ := f.request("POST", "/auth/password/change", change, resetCookie, 401)
	if errBody["code"] != "AUTH_REAUTH_FAILED" {
		t.Fatal("current-password error mapping")
	}
	change["current_password"] = newPassword
	_, changed := f.request("POST", "/auth/password/change", change, resetCookie, 200)
	f.request("GET", "/me", nil, resetCookie, 401)
	f.request("POST", "/auth/password/reset", map[string]string{"token": pending, "new_password": newPassword}, nil, 400)
	f.request("POST", "/auth/login", map[string]string{"email": email, "password": newPassword}, nil, 401)
	_, another := f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
	oldCSRF, _ := f.request("GET", "/auth/csrf", nil, changed, 200)
	oldActor, err := f.auth.Resolve(t.Context(), changed.Value)
	if err != nil {
		t.Fatal("actor missing")
	}
	f.exec("UPDATE app.sessions SET authenticated_at=now()-interval '1 hour' WHERE id=$1", oldActor.SessionID.String())
	f.request("POST", "/auth/reauthenticate", map[string]string{"password": "incorrect password value"}, changed, 401)
	_, reauthed := f.request("POST", "/auth/reauthenticate", map[string]string{"password": testPassword}, changed, 204)
	f.request("GET", "/me", nil, changed, 401)
	f.request("GET", "/me", nil, another, 200)
	actor, err := f.auth.Resolve(t.Context(), reauthed.Value)
	if err != nil || actor.SessionID == oldActor.SessionID || time.Since(actor.AuthenticatedAt) > time.Minute {
		t.Fatal("reauthentication did not rotate/freshen session")
	}
	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(reauthed)
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("X-CSRF-Token", oldCSRF["csrf_token"].(string))
	response, err := f.app.Test(req)
	if err != nil {
		t.Fatal("rotation CSRF request failed")
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("old CSRF accepted after rotation")
	}
	f.request("DELETE", "/me/sessions/"+f.sessionID(another), nil, reauthed, 204)
	f.request("GET", "/me", nil, another, 401)
	_, another = f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
	f.request("POST", "/me/sessions/revoke-others", nil, reauthed, 204)
	f.request("GET", "/me", nil, another, 401)
	f.request("GET", "/me", nil, reauthed, 200)
	_, cleared := f.request("DELETE", "/me/sessions/"+actor.SessionID.String(), nil, reauthed, 204)
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatal("self revocation did not clear cookie")
	}
	f.request("GET", "/me", nil, reauthed, 401)
	f.request("GET", "/me", nil, foreign, 200)
	_, last := f.request("POST", "/auth/login", map[string]string{"email": email, "password": testPassword}, nil, 200)
	f.request("POST", "/auth/logout", nil, last, 204)
	for _, event := range []string{"account_registered", "login_succeeded", "logout", "email_verification_requested", "password_reset_requested", "password_reset_completed", "password_changed", "reauthenticated", "session_revoked", "other_sessions_revoked"} {
		if f.count("SELECT count(*) FROM app.security_events WHERE user_id=$1 AND event_type=$2", id, event) < 1 {
			t.Fatalf("missing security event: %s", event)
		}
	}
}

func TestIntegrationSessionListExcludesInactiveAndAdmin(t *testing.T) {
	f := newFixture(t)
	id, email, current := f.register()
	credentials := map[string]string{"email": email, "password": testPassword}
	_, adminCookie := f.request("POST", "/auth/login", credentials, nil, 200)
	adminID := f.sessionID(adminCookie)
	_, expiredCookie := f.request("POST", "/auth/login", credentials, nil, 200)
	expiredID := f.sessionID(expiredCookie)
	f.exec("UPDATE app.sessions SET kind='admin' WHERE id=$1", adminID)
	f.exec("UPDATE app.sessions SET idle_expires_at=now()-interval '1 second' WHERE id=$1", expiredID)
	listed, _ := f.request("GET", "/me/sessions", nil, current, 200)
	if len(listed["sessions"].([]any)) != 1 {
		t.Fatal("list exposes inactive/Admin sessions")
	}
	for _, target := range []string{adminID, expiredID} {
		f.request("DELETE", "/me/sessions/"+target, nil, current, 404)
	}
	f.request("POST", "/me/sessions/revoke-others", nil, current, 204)
	if f.count("SELECT count(*) FROM app.sessions WHERE user_id=$1 AND kind='admin' AND revoked_at IS NULL", id) != 1 {
		t.Fatal("public revocation affected Admin session")
	}
	f.request("GET", "/me", nil, current, 200)
}
