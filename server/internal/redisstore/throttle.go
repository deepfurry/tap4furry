package redisstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/deepfurry/gofurry-platform/server/internal/auth"
)

type AuthThrottle struct {
	store  *Store
	secret []byte
}

func (s *Store) AuthThrottle(secret string) (*AuthThrottle, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth throttle secret must contain at least 32 bytes")
	}
	return &AuthThrottle{store: s, secret: []byte(secret)}, nil
}

var _ auth.Throttle = (*AuthThrottle)(nil)

func (t *AuthThrottle) keys(op auth.ThrottleOperation, subject string) []string {
	fingerprint := func(dimension, value string) string {
		mac := hmac.New(sha256.New, t.secret)
		mac.Write([]byte(string(op) + "\x00" + dimension + "\x00" + value))
		return t.store.Key("auth:limit:" + hex.EncodeToString(mac.Sum(nil)))
	}
	return []string{fingerprint("subject", subject), fingerprint("global", "")}
}

// Both dimensions are checked/recorded in one atomic script. Counters saturate;
// blocked calls cannot extend expiry or flood known-account security events.
// EVAL is intentional: runtime ACLs need no SCRIPT LOAD/EVALSHA fallback.
const throttleScript = `
local retry = 0
for i=1,2 do
 local count=tonumber(redis.call('GET',KEYS[i]) or '0')
 if count>=tonumber(ARGV[i]) then retry=math.max(retry,redis.call('TTL',KEYS[i]),1) end
end
if retry>0 then return {0,retry} end
if ARGV[4]=='record' then
 for i=1,2 do
  local count=redis.call('INCR',KEYS[i])
  if count==1 then redis.call('EXPIRE',KEYS[i],ARGV[3]) end
 end
end
return {1,0}`

func (t *AuthThrottle) Check(ctx context.Context, op auth.ThrottleOperation, subject string) (auth.ThrottleDecision, error) {
	return t.evaluate(ctx, op, subject, "check")
}
func (t *AuthThrottle) Record(ctx context.Context, op auth.ThrottleOperation, subject string) (auth.ThrottleDecision, error) {
	return t.evaluate(ctx, op, subject, "record")
}
func (t *AuthThrottle) evaluate(ctx context.Context, op auth.ThrottleOperation, subject, action string) (auth.ThrottleDecision, error) {
	policy, ok := auth.PolicyFor(op)
	if !ok {
		return auth.ThrottleDecision{}, auth.ErrThrottleUnavailable
	}
	result, err := t.store.client.Eval(ctx, throttleScript, t.keys(op, subject), policy.Subject, policy.Global, int64(policy.Window/time.Second), action).Int64Slice()
	if err != nil || len(result) != 2 {
		return auth.ThrottleDecision{}, auth.ErrThrottleUnavailable
	}
	return auth.ThrottleDecision{Allowed: result[0] == 1, RetryAfter: time.Duration(result[1]) * time.Second}, nil
}
func (t *AuthThrottle) ClearSubject(ctx context.Context, op auth.ThrottleOperation, subject string) error {
	if _, ok := auth.PolicyFor(op); !ok {
		return auth.ErrThrottleUnavailable
	}
	if t.store.client.Del(ctx, t.keys(op, subject)[0]).Err() != nil {
		return auth.ErrThrottleUnavailable
	}
	return nil
}
