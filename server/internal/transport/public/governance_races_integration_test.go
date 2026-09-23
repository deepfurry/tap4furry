package public

import (
	"context"
	"errors"
	"github.com/deepfurry/tap4furry/server/internal/auth"
	"github.com/deepfurry/tap4furry/server/internal/governance"
	"github.com/deepfurry/tap4furry/server/internal/identity"
	"github.com/deepfurry/tap4furry/server/internal/moderation"
	"sync"
	"testing"
	"time"
	"uuid"
)

func TestIntegrationGovernanceIndependentReportBudgets(t *testing.T) {
	f := newGovernanceFixture(t)
	ctx := t.Context()
	r, _ := f.published()
	for i, reason := range []string{"other", "spam", "content_rating", "broken_link", "privacy"} {
		in := reportInput(r.ID)
		in.Reason = reason
		f.report(in)
		f.exec("UPDATE app.reports SET created_at=clock_timestamp()-interval '2 minutes' WHERE reporter_id=$1", f.author.UserID.String())
		if i == 4 {
			_, e := f.pubGov.SubmitReport(ctx, f.author, moderation.ReportInput{RequestID: uuid.NewV7(), ResourceID: r.ID, TargetKind: "resource", Reason: "malicious_link", Body: "Another"})
			var limit *moderation.LimitError
			if !errors.As(e, &limit) || limit.Reason != "pending_limit" {
				t.Fatal("pending report budget not enforced", e)
			}
		}
	}
	own, err := f.pubGov.OwnReports(ctx, f.author, 1, 2, "")
	if err != nil || !own.HasNext || len(own.Items) != 2 {
		t.Fatal("report pagination failed", err)
	}
	for _, key := range f.records.Reports {
		if err = f.pubGov.WithdrawReport(ctx, f.author, uuid.MustParse(key), uuid.NewV7()); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		key := f.report(reportInput(r.ID))
		if err = f.pubGov.WithdrawReport(ctx, f.author, key, uuid.NewV7()); err != nil {
			t.Fatal(err)
		}
		f.exec("UPDATE app.reports SET created_at=clock_timestamp()-interval '2 minutes' WHERE reporter_id=$1", f.author.UserID.String())
	}
	f.change(moderation.UserChange{RequestID: uuid.NewV7(), Kind: "trust", Trust: "trusted", Reason: "Fixture trust"})
	_, err = f.pubGov.SubmitReport(ctx, f.author, reportInput(r.ID))
	var limit *moderation.LimitError
	if !errors.As(err, &limit) || limit.Reason != "daily_limit" || limit.RetryAfter < 1 {
		t.Fatal("withdrawal or trust refunded report daily budget", err)
	}
	ownState, err := f.pubGov.OwnGovernance(ctx, f.author)
	if err != nil || ownState.ReportQuota.Remaining24h != 0 || ownState.ContributionQuota.DailyLimit != 60 {
		t.Fatal("independent budgets not shown", err)
	}
}

func TestIntegrationGovernanceRestrictionSerializesBusinessWrites(t *testing.T) {
	for _, operation := range []string{"establish", "revoke", "expire"} {
		t.Run(operation, func(t *testing.T) {
			f := newGovernanceFixture(t)
			ctx := t.Context()
			profile := identity.ProfileUpdate{DisplayName: identity.Field[string]{Set: true, Value: cp("Concurrent profile")}}
			if operation == "establish" {
				pause := f.gate.armQuery(t, "-- name: GovernanceInsertRestriction")
				done := make(chan error, 1)
				go func() { _, e := f.adminGov.ChangeUser(ctx, f.actor, f.author.UserID, restriction(0)); done <- e }()
				select {
				case <-pause.entered:
				case <-time.After(4 * time.Second):
					t.Fatal("restriction did not reach locked write")
				}
				business := make(chan error, 1)
				go func() { _, e := f.pubGov.UpdateProfile(ctx, f.author, profile); business <- e }()
				blocked := false
				until := time.Now().Add(4 * time.Second)
				for time.Now().Before(until) {
					if f.scalar("SELECT count(*) FROM pg_stat_activity WHERE usename='gfp_api' AND cardinality(pg_blocking_pids(pid))>0") > 0 {
						blocked = true
						break
					}
					select {
					case <-business:
						t.Fatal("business write bypassed restriction lock")
					case <-time.After(10 * time.Millisecond):
					}
				}
				if !blocked {
					t.Fatal("business lock wait unobserved")
				}
				close(pause.resume)
				if e := <-done; e != nil {
					t.Fatal(e)
				}
				var limited *governance.RestrictedError
				if e := <-business; !errors.As(e, &limited) {
					t.Fatal("restriction commit did not block waiting profile", e)
				}
			} else {
				f.change(restriction(0))
				tx, e := f.owner.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback(ctx)
				if _, e = tx.Exec(ctx, "SELECT id FROM app.users WHERE id=$1 FOR UPDATE", f.author.UserID.String()); e != nil {
					t.Fatal(e)
				}
				done := make(chan error, 1)
				go func() { _, err := f.pubGov.UpdateProfile(ctx, f.author, profile); done <- err }()
				waitForGovernanceBlock(t, f, tx, "gfp_api", done)
				if operation == "revoke" {
					_, e = tx.Exec(ctx, "UPDATE app.user_restrictions SET revoked_at=clock_timestamp(),revoked_by=$2,revoke_reason='Concluded' WHERE user_id=$1", f.author.UserID.String(), f.actor.UserID.String())
				} else {
					_, e = tx.Exec(ctx, "UPDATE app.user_restrictions SET starts_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '1 second' WHERE user_id=$1", f.author.UserID.String())
				}
				if e != nil {
					t.Fatal("fixture policy transition failed")
				}
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				if e = <-done; e != nil {
					t.Fatal("waiting profile used stale policy time/state", e)
				}
			}
		})
	}
}

func TestIntegrationGovernanceTwoUserLockOrderAndSelfReview(t *testing.T) {
	f := newGovernanceFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	me, err := f.identity.Me(ctx, f.author.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.operator.Grant(ctx, me.Email, auth.Administrator); err != nil {
		t.Fatal(err)
	}
	_, cookie := f.adminLogin(me.Email)
	other, err := f.adminAuth.ResolveAdmin(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.records.Users = append(f.records.Users, f.actor.UserID.String())
	start := make(chan struct{})
	var results [2]error
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			actor, target := f.actor, other.UserID
			if i == 1 {
				actor, target = other, f.actor.UserID
			}
			<-start
			_, results[i] = f.adminGov.ChangeUser(ctx, actor, target, moderation.UserChange{RequestID: uuid.NewV7(), Kind: "trust", Trust: "established", Reason: "Independent authorized review"})
		})
	}
	close(start)
	wg.Wait()
	for _, e := range results {
		if e != nil {
			t.Fatal("opposite user governance deadlocked or failed", e)
		}
	}
	r, _ := f.published()
	report := f.report(reportInput(r.ID))
	if e := f.adminGov.DecideReport(ctx, other, report, moderation.ReportDecision{RequestID: uuid.NewV7(), ExpectedVersion: 1, Kind: "dismiss", SafeMessage: "Self decision"}); !errors.Is(e, moderation.ErrForbidden) {
		t.Fatal("self report decision accepted", e)
	}
}
