package redisstore

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

func TestIntegrationOAuthFlowStore(t *testing.T) {
	if os.Getenv("GFP_AUTH_INTEGRATION") != "1" {
		t.Skip("disposable integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("disposable guards required")
	}
	store, err := Open("redis://gfp_runtime:gfp_ci_only@127.0.0.1:6379/0", "gfp:")
	if err != nil {
		t.Fatal("disposable Redis open failed")
	}
	defer store.Close()
	inspector := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DisableIdentity: true})
	defer inspector.Close()
	state := oauth2.GenerateVerifier()
	key := store.Key("auth:oauth:flow:" + auth.OAuthStateDigest(state))
	flow := auth.OAuthFlow{Provider: auth.Google, Mode: auth.OAuthLogin, Verifier: oauth2.GenerateVerifier(), Nonce: oauth2.GenerateVerifier(), CreatedAt: time.Now().UTC()}
	if err = store.PutOAuthFlow(t.Context(), state, flow); err != nil {
		t.Fatal("flow store unavailable")
	}
	defer store.client.Del(t.Context(), key)
	ttl, err := inspector.TTL(t.Context(), key).Result()
	if err != nil || ttl > 10*time.Minute || ttl < 9*time.Minute {
		t.Fatal("flow lifetime differs")
	}
	payload, err := store.client.Get(t.Context(), key).Bytes()
	if err != nil {
		t.Fatal("flow payload unavailable")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || len(fields) != 5 {
		t.Fatal("flow payload fields differ")
	}
	for _, name := range []string{"provider", "mode", "verifier", "nonce", "created_at"} {
		if fields[name] == nil {
			t.Fatal("flow payload missing field")
		}
	}
	if strings.Contains(key, state) || strings.Contains(string(payload), state) {
		t.Fatal("raw state persisted")
	}
	if err = store.PutOAuthFlow(t.Context(), state, flow); !errors.Is(err, auth.ErrProviderInvalid) {
		t.Fatal("state collision overwrote flow")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			got, err := store.ConsumeOAuthFlow(t.Context(), state)
			if err == nil {
				if got.Verifier != flow.Verifier || got.Nonce != flow.Nonce {
					t.Error("flow round trip differs")
				}
				successes.Add(1)
			} else if !errors.Is(err, auth.ErrProviderInvalid) {
				t.Error("unexpected consume failure")
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("GETDEL did not consume exactly once")
	}
	if err = store.PutOAuthFlow(t.Context(), state, flow); err != nil {
		t.Fatal("expiry fixture failed")
	}
	if err = inspector.PExpire(t.Context(), key, time.Millisecond).Err(); err != nil {
		t.Fatal("expiry fixture failed")
	}
	time.Sleep(5 * time.Millisecond)
	if _, err = store.ConsumeOAuthFlow(t.Context(), state); !errors.Is(err, auth.ErrProviderInvalid) {
		t.Fatal("expired flow accepted")
	}
	if err = store.client.Set(t.Context(), key, "invalid-json", time.Minute).Err(); err != nil {
		t.Fatal("corrupt fixture failed")
	}
	if _, err = store.ConsumeOAuthFlow(t.Context(), state); !errors.Is(err, auth.ErrProviderInvalid) {
		t.Fatal("corrupt flow accepted")
	}
	if err = store.Close(); err != nil {
		t.Fatal("close failed")
	}
	if err = store.PutOAuthFlow(t.Context(), state, flow); !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Fatal("Redis outage not isolated")
	}
}
