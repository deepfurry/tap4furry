package resourcecheck

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/deepfurry/tap4furry/server/internal/database"
	"github.com/deepfurry/tap4furry/server/internal/database/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testFixture struct {
	Fixture
	pools map[string]*pgxpool.Pool
}

func setup(t *testing.T) testFixture {
	t.Helper()
	if os.Getenv("GFP_RESOURCE_INTEGRATION") != "1" {
		t.Skip("explicit disposable resource integration not enabled")
	}
	if os.Getenv("CI") != "true" || os.Getenv("GFP_DISPOSABLE_INFRA") != "1" {
		t.Fatal("disposable resource guards required")
	}
	f := testFixture{Fixture: NewFixture(), pools: map[string]*pgxpool.Pool{}}
	for _, role := range []string{"api", "admin", "worker", "readonly", "migrator"} {
		p, err := database.Open(t.Context(), "postgres://gfp_"+role+":gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable")
		if err != nil {
			t.Fatal("disposable connection failed")
		}
		t.Cleanup(p.Close)
		if _, err = database.Inspect(t.Context(), p, "gfp_"+role, "gfp_ci"); err != nil {
			t.Fatal("disposable identity mismatch")
		}
		f.pools[role] = p
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := f.Cleanup(ctx, f.pools["migrator"]); err != nil {
			t.Error(err)
		}
	})
	if err := f.Create(t.Context(), f.pools["admin"]); err != nil {
		t.Fatal(err)
	}
	return f
}

func assertSQL(t *testing.T, pool *pgxpool.Pool, state, statement string, args ...any) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal("test transaction unavailable")
	}
	defer tx.Rollback(t.Context())
	_, err = tx.Exec(t.Context(), statement, args...)
	if state == "" {
		if err != nil {
			t.Fatal(database.SafeError("valid fixture SQL", err))
		}
		return
	}
	var p *pgconn.PgError
	if !errors.As(err, &p) || p.Code != state {
		t.Fatalf("expected SQLSTATE %s for fixture statement; details withheld", state)
	}
}

func TestIntegrationSchemaShape(t *testing.T) {
	f := setup(t)
	p := f.pools["migrator"]
	columns := map[string][]string{
		"categories":             {"id", "slug", "default_locale", "state", "created_at", "updated_at", "deleted_at"},
		"category_localizations": {"category_id", "locale", "name", "description", "created_at", "updated_at"},
		"tags":                   {"id", "slug", "default_locale", "state", "created_at", "updated_at", "deleted_at"},
		"tag_localizations":      {"tag_id", "locale", "name", "description", "created_at", "updated_at"},
		"resources":              {"id", "slug", "default_locale", "category_id", "publication_state", "lifecycle", "content_rating", "version", "published_at", "created_at", "updated_at", "deleted_at"},
		"resource_localizations": {"resource_id", "locale", "name", "summary", "description", "created_at", "updated_at"},
		"resource_tags":          {"resource_id", "tag_id"},
		"resource_sources":       {"id", "resource_id", "url", "label", "source_type", "availability_state", "rights_status", "is_primary", "created_at", "updated_at"},
		"resource_relations":     {"id", "source_resource_id", "target_resource_id", "relation_type", "created_at"},
		"resource_external_ids":  {"resource_id", "namespace", "external_id", "created_at"},
	}
	for table, want := range columns {
		rows, err := p.Query(t.Context(), "SELECT column_name,data_type,column_default FROM information_schema.columns WHERE table_schema='app' AND table_name=$1 ORDER BY ordinal_position", table)
		if err != nil {
			t.Fatal("schema inspection failed")
		}
		got := []string{}
		for rows.Next() {
			var name, kind string
			var def *string
			if err := rows.Scan(&name, &kind, &def); err != nil {
				t.Fatal("schema scan failed")
			}
			got = append(got, name)
			if kind == "jsonb" || kind == "USER-DEFINED" {
				t.Fatal("canonical opaque/enum column")
			}
			if (name == "id" || name == "version" || name == "content_rating") && def != nil {
				t.Fatal("explicit value acquired DB default")
			}
		}
		if rows.Err() != nil {
			t.Fatal("schema iteration failed")
		}
		rows.Close()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("unexpected column shape: %s", table)
		}
		var rls bool
		var triggers, badFK int
		if err := p.QueryRow(t.Context(), `SELECT c.relrowsecurity,(SELECT count(*) FROM pg_trigger WHERE tgrelid=c.oid AND NOT tgisinternal),(SELECT count(*) FROM pg_constraint WHERE conrelid=c.oid AND contype='f' AND confdeltype<>'r') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='app' AND c.relname=$1`, table).Scan(&rls, &triggers, &badFK); err != nil || rls || triggers != 0 || badFK != 0 {
			t.Fatal("RLS, application trigger or non-RESTRICT FK introduced")
		}
	}
	var publication, lifecycle string
	if err := p.QueryRow(t.Context(), "SELECT publication_state,lifecycle FROM app.resources WHERE id=$1", f.A.String()).Scan(&publication, &lifecycle); err != nil || publication != "draft" || lifecycle != "unknown" {
		t.Fatal("resource defaults differ")
	}
}

func TestIntegrationResourceConstraints(t *testing.T) {
	f := setup(t)
	p := f.pools["migrator"]
	for _, item := range []struct {
		table string
		id    uuid.UUID
		limit int
	}{{"categories", f.Category, 64}, {"tags", f.Tag, 64}, {"resources", f.A, 80}} {
		for _, slug := range []string{"", "UPPER", "a--b", "-a", "a_1", "中文", strings.Repeat("a", item.limit+1)} {
			assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET slug=$2 WHERE id=$1", item.id.String(), slug)
		}
		for _, locale := range []string{"", "a", "en_US", "en--US", strings.Repeat("a", 65)} {
			assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET default_locale=$2 WHERE id=$1", item.id.String(), locale)
		}
		assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET updated_at=created_at-interval '1 second' WHERE id=$1", item.id.String())
		assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET deleted_at=created_at-interval '1 second' WHERE id=$1", item.id.String())
		if item.table != "resources" {
			assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET state='removed' WHERE id=$1", item.id.String())
			assertSQL(t, p, "23505", "INSERT INTO app."+item.table+" (id,slug,default_locale,created_at,updated_at) SELECT $2,slug,default_locale,created_at,updated_at FROM app."+item.table+" WHERE id=$1", item.id.String(), uuid.NewV7().String())
		}
	}
	assertSQL(t, p, "23505", "INSERT INTO app.resources (id,slug,default_locale,category_id,content_rating,version,created_at,updated_at) SELECT $2,slug,default_locale,category_id,content_rating,version,created_at,updated_at FROM app.resources WHERE id=$1", f.A.String(), uuid.NewV7().String())
	for _, field := range []string{"publication_state", "lifecycle", "content_rating"} {
		assertSQL(t, p, "23514", "UPDATE app.resources SET "+field+"='invalid' WHERE id=$1", f.A.String())
	}
	assertSQL(t, p, "23514", "UPDATE app.resources SET version=0 WHERE id=$1", f.A.String())
	assertSQL(t, p, "23502", "UPDATE app.resources SET content_rating=NULL WHERE id=$1", f.A.String())
	assertSQL(t, p, "23502", "UPDATE app.resources SET version=NULL WHERE id=$1", f.A.String())
	assertSQL(t, p, "23514", "UPDATE app.resources SET publication_state='published' WHERE id=$1", f.A.String())
	assertSQL(t, p, "23514", "UPDATE app.resources SET published_at=created_at-interval '1 second' WHERE id=$1", f.A.String())
	assertSQL(t, p, "", "UPDATE app.resources SET publication_state='published',published_at=now(),lifecycle='discontinued',content_rating='explicit' WHERE id=$1", f.A.String())
	for _, item := range []struct {
		table, key string
		id         uuid.UUID
		limit      int
	}{{"category_localizations", "category_id", f.Category, 80}, {"tag_localizations", "tag_id", f.Tag, 80}, {"resource_localizations", "resource_id", f.A, 160}} {
		for _, name := range []string{"", " ", "\t", " leading", "trailing\n", strings.Repeat("字", item.limit+1)} {
			assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET name=$2 WHERE "+item.key+"=$1", item.id.String(), name)
		}
		assertSQL(t, p, "23505", "INSERT INTO app."+item.table+" ("+item.key+",locale,name,created_at,updated_at) SELECT "+item.key+",'EN',name,created_at,updated_at FROM app."+item.table+" WHERE "+item.key+"=$1 AND locale='en'", item.id.String())
		assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET locale='en_US' WHERE "+item.key+"=$1", item.id.String())
		assertSQL(t, p, "23514", "UPDATE app."+item.table+" SET description='   ' WHERE "+item.key+"=$1", item.id.String())
	}
	assertSQL(t, p, "", "UPDATE app.resource_localizations SET description=$2 WHERE resource_id=$1", f.A.String(), strings.Repeat("字", 10000))
	assertSQL(t, p, "23514", "UPDATE app.resource_localizations SET summary=' ' WHERE resource_id=$1", f.A.String())
	assertSQL(t, p, "23505", "INSERT INTO app.resource_tags(resource_id,tag_id) VALUES($1,$2)", f.A.String(), f.Tag.String())
	assertSQL(t, p, "23503", "INSERT INTO app.resource_tags(resource_id,tag_id) VALUES($1,$2)", f.A.String(), uuid.NewV7().String())
	assertSQL(t, p, "23001", "DELETE FROM app.categories WHERE id=$1", f.Category.String())
	assertSQL(t, p, "23001", "DELETE FROM app.resources WHERE id=$1", f.A.String())
	for _, field := range []string{"source_type", "availability_state", "rights_status"} {
		assertSQL(t, p, "23514", "UPDATE app.resource_sources SET "+field+"='invalid' WHERE id=$1", f.Source.String())
	}
	for _, value := range []string{"ftp://example.invalid", "https://example.invalid/#fragment", "https://", " https://example.invalid", strings.Repeat("x", 2049)} {
		assertSQL(t, p, "23514", "UPDATE app.resource_sources SET url=$2 WHERE id=$1", f.Source.String(), value)
	}
	assertSQL(t, p, "23505", "INSERT INTO app.resource_sources(id,resource_id,url,source_type,created_at,updated_at) SELECT $2,resource_id,url,source_type,created_at,updated_at FROM app.resource_sources WHERE id=$1", f.Source.String(), uuid.NewV7().String())
	assertSQL(t, p, "23505", "INSERT INTO app.resource_sources(id,resource_id,url,source_type,is_primary,created_at,updated_at) VALUES($1,$2,'https://example.invalid/second','official',true,now(),now())", uuid.NewV7().String(), f.A.String())
	assertSQL(t, p, "", "INSERT INTO app.resource_sources(id,resource_id,url,source_type,created_at,updated_at) SELECT $2,$3,url,'mirror',created_at,updated_at FROM app.resource_sources WHERE id=$1", f.Source.String(), uuid.NewV7().String(), f.B.String())
	assertSQL(t, p, "", "UPDATE app.resource_sources SET availability_state='unavailable',rights_status='unknown',source_type='mirror',is_primary=true WHERE id=$1", f.Source.String())
	assertSQL(t, p, "23514", "UPDATE app.resource_relations SET target_resource_id=source_resource_id WHERE id=$1", f.Relation.String())
	assertSQL(t, p, "23514", "UPDATE app.resource_relations SET source_resource_id=target_resource_id,target_resource_id=source_resource_id WHERE id=$1", f.Relation.String())
	assertSQL(t, p, "23514", "UPDATE app.resource_relations SET relation_type='contains' WHERE id=$1", f.Relation.String())
	assertSQL(t, p, "23505", "INSERT INTO app.resource_relations(id,source_resource_id,target_resource_id,relation_type,created_at) SELECT $2,source_resource_id,target_resource_id,relation_type,created_at FROM app.resource_relations WHERE id=$1", f.Relation.String(), uuid.NewV7().String())
	assertSQL(t, p, "23505", "INSERT INTO app.resource_external_ids(resource_id,namespace,external_id,created_at) VALUES($1,'smoke',$2,now())", f.B.String(), f.A.String())
	assertSQL(t, p, "", "INSERT INTO app.resource_external_ids(resource_id,namespace,external_id,created_at) VALUES($1,'smoke',$2,now()),($1,'smoke',$3,now())", f.B.String(), uuid.NewV7().String(), uuid.NewV7().String())
	for _, ns := range []string{"UPPER", "double..dot", "", strings.Repeat("a", 65)} {
		assertSQL(t, p, "23514", "UPDATE app.resource_external_ids SET namespace=$2 WHERE resource_id=$1", f.A.String(), ns)
	}
}

func TestIntegrationResourcePrivileges(t *testing.T) {
	f := setup(t)
	for _, role := range []string{"api", "admin", "worker", "readonly"} {
		t.Run(role, func(t *testing.T) {
			if err := VerifyRole(t.Context(), f.pools[role], role, f.Fixture); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := f.ExerciseMutation(t.Context(), f.pools["admin"]); err != nil {
		t.Fatal(err)
	}
}

func bump(ctx context.Context, q *sqlc.Queries, id uuid.UUID, expected int64) (int64, error) {
	return q.CompareAndBumpResourceVersion(ctx, sqlc.CompareAndBumpResourceVersionParams{ID: ID(id), ExpectedVersion: expected, Now: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
}

func TestIntegrationResourceRevisionTransactions(t *testing.T) {
	f := setup(t)
	p := f.pools["admin"]
	if err := f.ExerciseMutation(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{f.A, f.B} {
		r, err := sqlc.New(f.pools["api"]).GetResourceRevision(t.Context(), ID(id))
		if err != nil || r.Version != 2 {
			t.Fatal("logical graph mutation did not increment each endpoint once")
		}
	}
	tx, err := p.Begin(t.Context())
	if err != nil {
		t.Fatal("CAS transaction failed")
	}
	defer tx.Rollback(t.Context())
	if _, err = tx.Exec(t.Context(), "UPDATE app.resource_localizations SET name='stale edit' WHERE resource_id=$1", f.A.String()); err != nil {
		t.Fatal("child edit fixture failed")
	}
	if _, err = bump(t.Context(), sqlc.New(tx), f.A, 1); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("stale expected version did not conflict")
	}
	if err = tx.Rollback(t.Context()); err != nil {
		t.Fatal("stale transaction rollback failed")
	}
	var name string
	if err = p.QueryRow(t.Context(), "SELECT name FROM app.resource_localizations WHERE resource_id=$1 AND locale='en'", f.A.String()).Scan(&name); err != nil || name == "stale edit" {
		t.Fatal("stale transaction leaked child edit")
	}
	for _, item := range []struct {
		table, key string
		id         uuid.UUID
	}{{"category_localizations", "category_id", f.Category}, {"tag_localizations", "tag_id", f.Tag}} {
		if _, err = p.Exec(t.Context(), "UPDATE app."+item.table+" SET name='Taxonomy edit',updated_at=now() WHERE "+item.key+"=$1", item.id.String()); err != nil {
			t.Fatal("taxonomy edit failed")
		}
	}
	r, err := sqlc.New(p).GetResourceRevision(t.Context(), ID(f.A))
	if err != nil || r.Version != 2 {
		t.Fatal("independent taxonomy edit changed resource version")
	}
	tx, err = p.Begin(t.Context())
	if err != nil {
		t.Fatal("soft-delete transaction failed")
	}
	defer tx.Rollback(t.Context())
	if _, err = bump(t.Context(), sqlc.New(tx), f.A, 2); err != nil {
		t.Fatal("soft-delete CAS failed")
	}
	if _, err = tx.Exec(t.Context(), "UPDATE app.resources SET deleted_at=now() WHERE id=$1", f.A.String()); err != nil {
		t.Fatal("soft-delete fixture failed")
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal("soft-delete commit failed")
	}
	if _, err = sqlc.New(p).GetResourceRevision(t.Context(), ID(f.A)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("normal resource view exposes soft-deleted row")
	}
	if _, err = bump(t.Context(), sqlc.New(p), f.A, 3); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("CAS mutated soft-deleted resource")
	}
}

func TestIntegrationResourceCASConcurrency(t *testing.T) {
	f := setup(t)
	errs := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := bump(t.Context(), sqlc.New(f.pools["admin"]), f.A, 1)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, pgx.ErrNoRows) {
			conflict++
		} else {
			t.Fatal(database.SafeError("concurrent CAS", err))
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("CAS did not choose exactly one winner")
	}
	r, err := sqlc.New(f.pools["api"]).GetResourceRevision(t.Context(), ID(f.A))
	if err != nil || r.Version != 2 {
		t.Fatal("CAS revision differs")
	}
}
