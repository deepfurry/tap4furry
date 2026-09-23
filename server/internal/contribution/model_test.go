package contribution

import (
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/resource"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"testing"
	"time"
	"uuid"
)

func pointer[T any](v T) *T { return &v }
func TestNormalizationPatchAndUnicode(t *testing.T) {
	base := Content{DefaultLocale: "en", CategoryID: uuid.NewV7(), Name: "Original", Summary: pointer("Summary"), Description: pointer("Description"), Lifecycle: resource.Unknown, ContentRating: resource.General}
	changed, err := apply(base, Patch{Name: pointer(" Changed 🦊 "), Summary: NullableText{Set: true}})
	if err != nil || changed.Name != "Changed 🦊" || changed.Summary != nil || *changed.Description != "Description" {
		t.Fatal("omission/null or normalization failed")
	}
	base.Description = pointer(strings.Repeat("🦊", 50000))
	if _, err := normalizeContent(base); err != nil {
		t.Fatal("Unicode limit counts bytes/UTF-16 units")
	}
	base.Description = pointer(*base.Description + "🦊")
	if _, err := normalizeContent(base); !errors.Is(err, ErrValidation) {
		t.Fatal("description limit not enforced")
	}
	for _, raw := range []string{"javascript:alert(1)", "data:text/plain,hello", "https://user:password@example.invalid/"} {
		if _, err := normalizeSource(&Source{URL: raw, Type: "unknown", Availability: "active"}); !errors.Is(err, ErrValidation) {
			t.Fatal("unsafe source accepted")
		}
	}
	source, err := normalizeSource(&Source{URL: "HTTPS://EXAMPLE.INVALID:443/a#fragment", Type: "unknown", Availability: "active"})
	if err != nil || source.URL != "https://example.invalid/a" {
		t.Fatal("source normalization failed")
	}
}
func TestQuotaEdges(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		quota  sqlc.ContributionQuotaRow
		reason string
		retry  int
	}{
		{sqlc.ContributionQuotaRow{Pending: 5}, "pending_limit", 0},
		{sqlc.ContributionQuotaRow{Recent: 10, FirstAt: stamp(now.Add(-23 * time.Hour))}, "daily_limit", 3600},
		{sqlc.ContributionQuotaRow{LastAt: stamp(now.Add(-59 * time.Second))}, "submission_interval", 1},
	} {
		var limit *LimitError
		if !errors.As(checkQuota(test.quota, now, governance.ReportBudget()), &limit) || limit.Reason != test.reason || limit.RetryAfter != test.retry {
			t.Fatal("quota boundary failed")
		}
	}
	if checkQuota(sqlc.ContributionQuotaRow{Recent: 9, Pending: 4, LastAt: stamp(now.Add(-time.Minute))}, now, governance.ReportBudget()) != nil {
		t.Fatal("quota exact boundary rejected")
	}
}
func TestRevisionBoundToActorResourceLocaleAndVersion(t *testing.T) {
	a := New(nil, "fixture-secret")
	actor := uuid.NewV7()
	row := sqlc.ContributionResourceRow{ID: pgtype.UUID{Bytes: uuid.NewV7(), Valid: true}, Version: 4, DefaultLocale: "en"}
	original := a.revision(actor, row)
	if len(original) != 43 || original == a.revision(uuid.NewV7(), row) {
		t.Fatal("actor not bound")
	}
	row.Version++
	if original == a.revision(actor, row) {
		t.Fatal("version not bound")
	}
	row.Version--
	row.DefaultLocale = "zh-Hans"
	if original == a.revision(actor, row) {
		t.Fatal("locale not bound")
	}
	row.DefaultLocale = "en"
	row.ID.Bytes = uuid.NewV7()
	if original == a.revision(actor, row) {
		t.Fatal("resource not bound")
	}
}
