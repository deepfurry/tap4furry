package migrate

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"testing"
	"uuid"
)

// Called only after the existing fixed loopback/database/CI/version guards.
func completeContributionRoundTrip(t *testing.T, p *goose.Provider, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	user, category, res, old, newID := uuid.NewV7().String(), uuid.NewV7().String(), uuid.NewV7().String(), uuid.NewV7().String(), uuid.NewV7().String()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal("migration round-trip fixture operation failed")
		}
	}
	exec("INSERT INTO app.users(id,created_at,updated_at) VALUES($1,now(),now())", user)
	exec("INSERT INTO app.categories(id,slug,default_locale,state,created_at,updated_at) VALUES($1,$2,'en','active',now(),now())", category, "roundtrip-"+category)
	exec("INSERT INTO app.category_localizations(category_id,locale,name,created_at,updated_at) VALUES($1,'en','Roundtrip',now(),now())", category)
	exec("INSERT INTO app.resources(id,slug,default_locale,category_id,publication_state,lifecycle,content_rating,version,created_at,updated_at) VALUES($1,$2,'en',$3,'draft','unknown','general',1,now(),now())", res, "roundtrip-"+res, category)
	exec("INSERT INTO app.resource_localizations(resource_id,locale,name,created_at,updated_at) VALUES($1,'en','Roundtrip',now(),now())", res)
	exec("INSERT INTO app.contributions(id,author_id,kind,reason,request_id,request_fingerprint,submitted_fields) VALUES($1,$2,'create_resource','Old proposal',$1,decode(repeat('00',32),'hex'),127)", old, user)
	exec("INSERT INTO app.contribution_contents(contribution_id,content_kind,default_locale,category_id,name,summary,lifecycle,content_rating) VALUES($1,'proposed','en',$2,'Old original','Original summary','unknown','general')", old, category)
	exec("INSERT INTO app.contributions(id,author_id,kind,target_resource_id,base_version,reason,request_id,request_fingerprint,submitted_fields) VALUES($1,$2,'add_source',$3,1,'New proposal',$1,decode(repeat('00',32),'hex'),0)", newID, user, res)
	exec("INSERT INTO app.contribution_source_changes(contribution_id,snapshot_kind,url,source_type) VALUES($1,'proposed','https://example.invalid/source','unknown')", newID)
	if _, err := p.Down(ctx); err == nil {
		t.Fatal("migration 8 down discarded new proposals")
	}
	version, err := p.GetDBVersion(ctx)
	if err != nil || version != 8 {
		t.Fatal("rejected downgrade changed Goose version")
	}
	exec("DELETE FROM app.contribution_source_changes WHERE contribution_id=$1", newID)
	exec("DELETE FROM app.contributions WHERE id=$1", newID)
	if _, err = p.Down(ctx); err != nil {
		t.Fatal("migration 8 down failed after fixture cleanup")
	}
	version, err = p.GetDBVersion(ctx)
	if err != nil || version != 7 {
		t.Fatal("migration 8 down did not preserve version 7")
	}
	var name string
	if err = pool.QueryRow(ctx, "SELECT name FROM app.contribution_contents WHERE contribution_id=$1", old).Scan(&name); err != nil || name != "Old original" {
		t.Fatal("migration 8 down changed old proposal")
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal("migration 8 up failed")
	}
	if err = pool.QueryRow(ctx, "SELECT name FROM app.contribution_contents WHERE contribution_id=$1", old).Scan(&name); err != nil || name != "Old original" {
		t.Fatal("migration 8 up changed old proposal")
	}
	exec("DELETE FROM app.contribution_contents WHERE contribution_id=$1", old)
	exec("DELETE FROM app.contributions WHERE id=$1", old)
	exec("DELETE FROM app.resource_localizations WHERE resource_id=$1", res)
	exec("DELETE FROM app.resources WHERE id=$1", res)
	exec("DELETE FROM app.category_localizations WHERE category_id=$1", category)
	exec("DELETE FROM app.categories WHERE id=$1", category)
	exec("DELETE FROM app.users WHERE id=$1", user)
	if _, err = p.Down(ctx); err != nil {
		t.Fatal("return to version 7 before legacy round-trip failed")
	}
}
