package redisstore

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/redis/go-redis/v9"
)

var _ auth.OAuthFlowStore = (*Store)(nil)

func (s *Store) PutOAuthFlow(ctx context.Context, state string, flow auth.OAuthFlow) error {
	if len(state) != 43 {
		return auth.ErrProviderInvalid
	}
	data, err := json.Marshal(flow)
	if err != nil {
		return auth.ErrProviderInvalid
	}
	ok, err := s.client.SetNX(ctx, s.Key("auth:oauth:flow:"+auth.OAuthStateDigest(state)), data, auth.OAuthLifetime).Result()
	if err != nil {
		return auth.ErrProviderUnavailable
	}
	if !ok {
		return auth.ErrProviderInvalid
	}
	return nil
}
func (s *Store) ConsumeOAuthFlow(ctx context.Context, state string) (auth.OAuthFlow, error) {
	var flow auth.OAuthFlow
	if len(state) != 43 {
		return flow, auth.ErrProviderInvalid
	}
	data, err := s.client.GetDel(ctx, s.Key("auth:oauth:flow:"+auth.OAuthStateDigest(state))).Bytes()
	if errors.Is(err, redis.Nil) {
		return flow, auth.ErrProviderInvalid
	}
	if err != nil {
		return flow, auth.ErrProviderUnavailable
	}
	if len(data) > 4096 || json.Unmarshal(data, &flow) != nil {
		return flow, auth.ErrProviderInvalid
	}
	return flow, nil
}
