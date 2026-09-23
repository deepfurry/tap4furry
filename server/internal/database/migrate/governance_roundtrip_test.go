package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/deepfurry/tap4furry/server/internal/database/governancecheck"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"testing"
	"uuid"
)

// Runs only inside the fixed, guarded disposable migration test. No shared down.
func governanceRoundTrip(t *testing.T, p *goose.Provider, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	user, cat, res := uuid.NewV7().String(), uuid.NewV7().String(), uuid.NewV7().String()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal("governance roundtrip fixture operation failed")
		}
	}
	exec("INSERT INTO app.users(id,created_at,updated_at) VALUES($1,now(),now())", user)
	exec("INSERT INTO app.categories(id,slug,default_locale,state,created_at,updated_at) VALUES($1,$2,'en','active',now(),now())", cat, "governance-"+cat)
	exec("INSERT INTO app.resources(id,slug,default_locale,category_id,publication_state,lifecycle,content_rating,version,created_at,updated_at) VALUES($1,$2,'en',$3,'draft','unknown','general',1,now(),now())", res, "governance-"+res, cat)
	kinds := []string{"create_resource", "update_resource", "add_source", "remove_broken_source", "add_tag", "add_relation", "add_translation"}
	ids := []string{}
	for _, kind := range kinds {
		key := uuid.NewV7().String()
		ids = append(ids, key)
		var target any = res
		var version any = int64(1)
		if kind == "create_resource" {
			target = nil
			version = nil
		}
		exec("INSERT INTO app.contributions(id,author_id,kind,target_resource_id,base_version,reason,request_id,request_fingerprint,submitted_fields) VALUES($1,$2,$3,$4,$5,'Preserved original',$1,decode(repeat('00',32),'hex'),0)", key, user, kind, target, version)
	}
	// Include typed snapshots as well as all seven immutable proposal envelopes.
	exec("INSERT INTO app.contribution_source_changes(contribution_id,snapshot_kind,url,source_type) VALUES($1,'proposed','https://example.invalid/original','unknown')", ids[2])
	exec("INSERT INTO app.contribution_relation_changes(contribution_id,snapshot_kind,other_resource_id,relation_type,direction,other_base_version) VALUES($1,'proposed',$2,'related_to','symmetric',1)", ids[5], res)
	exec("INSERT INTO app.contribution_localization_changes(contribution_id,snapshot_kind,locale,row_exists,name,supplied_fields) VALUES($1,'proposed','ja',true,'元の名前',1)", ids[6])
	fingerprint := func() [32]byte {
		t.Helper()
		data := []any{}
		for _, table := range []string{"contributions", "contribution_source_changes", "contribution_relation_changes", "contribution_localization_changes"} {
			column := "contribution_id"
			if table == "contributions" {
				column = "id"
			}
			rows, err := pool.Query(ctx, "SELECT * FROM app."+table+" WHERE "+column+"=ANY($1::uuid[]) ORDER BY 1,2", ids)
			if err != nil {
				t.Fatal("preserved proposal query failed")
			}
			for rows.Next() {
				v, e := rows.Values()
				if e != nil {
					t.Fatal("preserved proposal read failed")
				}
				data = append(data, v)
			}
			if rows.Err() != nil {
				t.Fatal("preserved proposal rows failed")
			}
			rows.Close()
		}
		b, _ := json.Marshal(data)
		return sha256.Sum256(b)
	}
	before := fingerprint()
	exec("INSERT INTO app.user_governance_profiles(user_id,trust_level,revision) VALUES($1,'new',1)", user)
	if _, err := p.Down(ctx); err == nil {
		t.Fatal("migration 9 discarded governance data")
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != 9 {
		t.Fatal("rejected down changed Goose version")
	}
	exec("DELETE FROM app.user_governance_profiles WHERE user_id=$1", user)
	if _, err := p.Down(ctx); err != nil {
		t.Fatal("migration 9 down failed")
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != 8 {
		t.Fatal("migration 9 down did not stop at 8")
	}
	if fingerprint() != before {
		t.Fatal("migration 9 down changed typed proposals")
	}
	if _, err := p.UpTo(ctx, 9); err != nil {
		t.Fatal("migration 9 up failed")
	}
	if fingerprint() != before {
		t.Fatal("migration 9 up changed typed proposals")
	}
	if err := governancecheck.VerifyGrants(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"contribution_source_changes", "contribution_relation_changes", "contribution_localization_changes"} {
		exec("DELETE FROM app."+table+" WHERE contribution_id=ANY($1::uuid[])", ids)
	}
	exec("DELETE FROM app.contributions WHERE id=ANY($1::uuid[])", ids)
	exec("DELETE FROM app.resources WHERE id=$1", res)
	exec("DELETE FROM app.categories WHERE id=$1", cat)
	exec("DELETE FROM app.users WHERE id=$1", user)
	if _, err := p.Down(context.WithoutCancel(ctx)); err != nil {
		t.Fatal("return to version 8 failed")
	}
}
