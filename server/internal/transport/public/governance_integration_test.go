package public

import (
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/database/governancecheck"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	"net/http"
	"os"
	"testing"
)

type governanceFixture struct {
	*contributionFixture
	pubGov, adminGov *moderation.App
	records          *governancecheck.Fixture
}

func newGovernanceFixture(t *testing.T) *governanceFixture {
	t.Helper()
	if os.Getenv("GFP_GOVERNANCE_INTEGRATION") != "1" {
		t.Skip("explicit disposable governance integration not enabled")
	}
	t.Setenv("GFP_CONTRIBUTION_INTEGRATION", "1")
	f := newContributionFixture(t)
	if _, err := f.operator.Grant(t.Context(), f.email, auth.Administrator); err != nil {
		t.Fatal(err)
	}
	records := &governancecheck.Fixture{Users: []string{f.author.UserID.String()}}
	t.Cleanup(func() {
		if err := records.Cleanup(f.owner); err != nil {
			t.Error(err)
		}
	})
	return &governanceFixture{f, moderation.New(f.api), moderation.New(f.adminPool), records}
}
func TestIntegrationGovernanceHTTP(t *testing.T) {
	f := newGovernanceFixture(t)
	authors := [7]*http.Cookie{f.authorCookie}
	for i := 1; i < len(authors); i++ {
		_, _, cookie := f.register()
		f.request("POST", "/auth/email/verification", map[string]string{"token": f.lastToken("email_verify")}, nil, 204)
		authors[i] = cookie
	}
	if err := governancecheck.Run(t.Context(), f.app, f.adminApp, f.owner, f.operator, authors, f.email, testPassword, testOrigin, adminOrigin); err != nil {
		t.Fatal(err)
	}
}
