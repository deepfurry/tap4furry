package governance

import (
	"strings"
	"testing"
	"time"
)

func TestProtectedChannelsAndBudgets(t *testing.T) {
	for _, scope := range []Scope{ContributionSubmit, PublicProfileWrite, AllWrite} {
		for _, action := range []Scope{"report_submit", "login", "password_change", "session_revoke", "oauth_unlink", "own_history", "withdraw", "disable_indexing"} {
			if scope.Blocks(action) {
				t.Fatalf("protected action %s blocked", action)
			}
		}
	}
	if !AllWrite.Blocks(ContributionSubmit) || !AllWrite.Blocks(PublicProfileWrite) || ContributionSubmit.Blocks(PublicProfileWrite) || PublicProfileWrite.Blocks(ContributionSubmit) {
		t.Fatal("scope mapping is wrong")
	}
	for _, tc := range []struct {
		level          string
		daily, pending int64
		interval       time.Duration
	}{{"new", 10, 5, time.Minute}, {"established", 30, 10, 30 * time.Second}, {"trusted", 60, 20, 15 * time.Second}} {
		b, e := ContributionBudget(tc.level)
		if e != nil || b.Daily != tc.daily || b.Pending != tc.pending || b.Interval != tc.interval {
			t.Fatal("fixed trust policy changed")
		}
	}
	if _, e := ContributionBudget("admin"); e == nil {
		t.Fatal("role interpreted as trust")
	}
	if ReportBudget() != (Budget{10, 5, time.Minute}) {
		t.Fatal("report budget differs")
	}
}
func TestRecommendationEligibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    RecommendationEligibility
		want bool
	}{
		{"eligible", RecommendationEligibility{true, true, true, true, true, false, true}, true},
		{"hidden", RecommendationEligibility{false, true, true, true, true, false, true}, false},
		{"corrupt", RecommendationEligibility{true, false, true, true, true, false, true}, false},
		{"adult", RecommendationEligibility{true, true, false, true, true, false, true}, false},
		{"historical", RecommendationEligibility{true, true, true, false, true, false, true}, false},
		{"retired", RecommendationEligibility{true, true, true, true, false, false, true}, false},
		{"excluded", RecommendationEligibility{true, true, true, true, true, true, true}, false},
		{"unknown rights or inactive source", RecommendationEligibility{true, true, true, true, true, false, false}, false},
	} {
		if tc.e.Eligible() != tc.want {
			t.Fatal(tc.name)
		}
	}
}
func TestUnicodeReasonLimits(t *testing.T) {
	value := strings.Repeat("🦊", 1000)
	if v, e := Text(" \n"+value+"\t", 1000); e != nil || v != value {
		t.Fatal("Unicode reason normalization")
	}
	for _, s := range []string{" ", value + "x", "bad\x00input", string([]byte{0xff})} {
		if _, e := Text(s, 1000); e == nil {
			t.Fatal("invalid reason accepted")
		}
	}
}
