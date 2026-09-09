package redisstore

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/deepfurry/gofurry-platform/server/internal/auth"
)

func TestIntegrationAuthThrottle(t *testing.T) {
	if os.Getenv("GFP_AUTH_INTEGRATION") != "1" {
		t.Skip("disposable integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("disposable guards required")
	}
	store, err := Open("redis://gfp_runtime:gfp_ci_only@127.0.0.1:6379/0", "gfp:")
	if err != nil {
		t.Fatal("Redis fixture failed")
	}
	defer store.Close()
	throttle, err := store.AuthThrottle(uuid.NewV7().String())
	if err != nil {
		t.Fatal("throttle fixture failed")
	}
	ctx := t.Context()
	subject := "normalized@example.invalid"
	keys := throttle.keys(auth.AdminLoginLimit, subject)
	defer store.client.Del(ctx, keys...)
	for _, key := range keys {
		if !strings.HasPrefix(key, "gfp:auth:limit:") || len(key) != len("gfp:auth:limit:")+64 || strings.Contains(key, subject) {
			t.Fatal("throttle key privacy failed")
		}
	}
	other, _ := store.AuthThrottle(uuid.NewV7().String())
	if keys[0] == other.keys(auth.AdminLoginLimit, subject)[0] {
		t.Fatal("key fingerprint is not keyed")
	}
	if keys[0] == throttle.keys(auth.PublicLoginLimit, subject)[0] || keys[0] == keys[1] {
		t.Fatal("throttle dimensions overlap")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			d, err := throttle.Record(ctx, auth.AdminLoginLimit, subject)
			if err != nil {
				t.Error("atomic record failed")
			} else if d.Allowed {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 5 {
		t.Fatal("atomic subject limit exceeded")
	}
	for _, key := range keys {
		value, err := store.client.Get(ctx, key).Result()
		if err != nil || value != "5" {
			t.Fatal("counter is not saturated numeric state")
		}
		ttl, err := store.client.TTL(ctx, key).Result()
		if err != nil || ttl > 15*time.Minute || ttl < 14*time.Minute {
			t.Fatal("counter TTL failed")
		}
	}
	decision, err := throttle.Check(ctx, auth.AdminLoginLimit, subject)
	if err != nil || decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatal("limited check failed")
	}
	if err = throttle.ClearSubject(ctx, auth.AdminLoginLimit, subject); err != nil {
		t.Fatal("subject clear failed")
	}
	if d, err := throttle.Check(ctx, auth.AdminLoginLimit, subject); err != nil || !d.Allowed {
		t.Fatal("successful login did not clear subject")
	}
	if value, _ := store.client.Get(ctx, keys[1]).Result(); value != "5" {
		t.Fatal("clearing subject erased global counter")
	}
	if err = store.client.Set(ctx, keys[1], "100", time.Minute).Err(); err != nil {
		t.Fatal("global fixture failed")
	}
	if d, err := throttle.Check(ctx, auth.AdminLoginLimit, "other@example.invalid"); err != nil || d.Allowed {
		t.Fatal("global policy bypassed")
	}
	store.client.Del(ctx, keys...)
	for _, op := range []auth.ThrottleOperation{auth.PublicLoginLimit, auth.RegistrationLimit, auth.PasswordResetLimit, auth.PublicReauthLimit, auth.AdminReauthLimit} {
		policy, _ := auth.PolicyFor(op)
		opKeys := throttle.keys(op, subject)
		defer store.client.Del(ctx, opKeys...)
		for i := 0; i < policy.Subject; i++ {
			d, err := throttle.Record(ctx, op, subject)
			if err != nil || !d.Allowed {
				t.Fatal("policy ended prematurely")
			}
		}
		if d, _ := throttle.Record(ctx, op, subject); d.Allowed {
			t.Fatal("policy limit exceeded")
		}
		if v, _ := store.client.Get(ctx, opKeys[0]).Result(); v != strconv.Itoa(policy.Subject) {
			t.Fatal("counter changed when blocked")
		}
		if err := store.client.Set(ctx, opKeys[0], strconv.Itoa(policy.Subject), 5*time.Millisecond).Err(); err != nil {
			t.Fatal("short expiry fixture failed")
		}
		deadline := time.Now().Add(time.Second)
		for {
			v, err := store.client.Get(ctx, opKeys[0]).Result()
			if err != nil && v == "" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("expiry failed")
			}
			time.Sleep(time.Millisecond)
		}
		if d, err := throttle.Check(ctx, op, subject); err != nil || !d.Allowed {
			t.Fatal("expiry did not restore attempts")
		}
	}
	if _, err = throttle.Check(ctx, "unknown", subject); !errors.Is(err, auth.ErrThrottleUnavailable) {
		t.Fatal("unknown policy accepted")
	}
	store.Close()
	if _, err = throttle.Check(ctx, auth.AdminLoginLimit, subject); !errors.Is(err, auth.ErrThrottleUnavailable) {
		t.Fatal("driver error leaked")
	}
}
