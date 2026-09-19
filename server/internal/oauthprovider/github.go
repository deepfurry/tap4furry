package oauthprovider

import (
	"context"
	"strconv"

	"github.com/deepfurry/tap4furry/server/internal/auth"
)

func (p *provider) githubIdentity(ctx context.Context, token string) (auth.ProviderIdentity, error) {
	var user struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := p.getJSON(ctx, "/user", token, &user); err != nil {
		return auth.ProviderIdentity{}, err
	}
	if user.ID <= 0 {
		return auth.ProviderIdentity{}, auth.ErrProviderInvalid
	}
	identity := auth.ProviderIdentity{Provider: auth.GitHub, Subject: strconv.FormatInt(user.ID, 10), DisplayName: user.Name}
	// Follow numbered pages at the fixed API origin; never follow a response URL
	// with the bearer token. Bound work even if an upstream response is malformed.
	for page := 1; page <= 10; page++ {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := p.getJSON(ctx, "/user/emails?per_page=100&page="+strconv.Itoa(page), token, &emails); err != nil {
			return auth.ProviderIdentity{}, err
		}
		for _, entry := range emails {
			email, err := auth.NormalizeEmail(entry.Email)
			if !entry.Verified || err != nil {
				continue
			}
			if identity.Email == "" || entry.Primary {
				identity.Email = email
				identity.EmailVerified = true
				identity.EmailAuthoritative = true
			}
			if entry.Primary {
				return identity, nil
			}
		}
		if len(emails) < 100 {
			return identity, nil
		}
	}
	return auth.ProviderIdentity{}, auth.ErrProviderInvalid
}
