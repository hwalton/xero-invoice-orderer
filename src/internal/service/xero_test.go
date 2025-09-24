//go:build integration
// +build integration

package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
)

func setupTestPostgresXero(t *testing.T) (dbURL string, cleanup func()) {
	t.Helper()

	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("docker socket not found; skipping docker-dependent tests")
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("could not create dockertest pool: %v", err)
	}

	opts := &dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "15-alpine",
		Env: []string{
			"POSTGRES_USER=postgres",
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_DB=postgres",
		},
	}
	resource, err := pool.RunWithOptions(opts)
	if err != nil {
		t.Fatalf("could not start postgres container: %v", err)
	}

	cleanupFunc := func() {
		_ = pool.Purge(resource)
	}

	hostPort := resource.GetPort("5432/tcp")
	connStr := fmt.Sprintf("postgres://postgres:secret@localhost:%s/postgres?sslmode=disable", hostPort)

	var db *pgxpool.Pool
	if err := pool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var cerr error
		db, cerr = pgxpool.New(ctx, connStr)
		if cerr != nil {
			return cerr
		}
		var one int
		if err := db.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			db.Close()
			return err
		}
		return nil
	}); err != nil {
		_ = pool.Purge(resource)
		t.Fatalf("could not connect to postgres in container: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS xero_connections (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  access_token TEXT NOT NULL,
  refresh_token TEXT NOT NULL,
  expires_at BIGINT,
  created_at BIGINT,
  updated_at BIGINT
);

CREATE TABLE IF NOT EXISTS shopping_list (
  list_id SERIAL PRIMARY KEY,
  item_id TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  ordered BOOLEAN DEFAULT FALSE,
  created_at BIGINT,
  updated_at BIGINT
);
`)
	if err != nil {
		db.Close()
		_ = pool.Purge(resource)
		t.Fatalf("failed to create tables: %v", err)
	}

	db.Close()
	return connStr, cleanupFunc
}

func TestUpsertConnection_InsertAndUpdate(t *testing.T) {
	// do not run in parallel due to docker container usage
	dbURL, cleanup := setupTestPostgresXero(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// empty db url -> error
	if err := UpsertConnection(ctx, "", "o", "t", "a", "b", 60); err == nil {
		t.Fatal("expected error for empty db url")
	}

	ownerID := "owner1"
	tenantID := "tenant1"
	access := "tok-1"
	refresh := "ref-1"
	expires := int64(3600)

	if err := UpsertConnection(ctx, dbURL, ownerID, tenantID, access, refresh, expires); err != nil {
		t.Fatalf("UpsertConnection failed: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	var id, gotOwner, gotTenant, gotAccess, gotRefresh string
	var gotExpires int64
	if err := pool.QueryRow(ctx, `SELECT id, owner_id, tenant_id, access_token, refresh_token, expires_at FROM xero_connections WHERE id = $1`, ownerID+":"+tenantID).
		Scan(&id, &gotOwner, &gotTenant, &gotAccess, &gotRefresh, &gotExpires); err != nil {
		t.Fatalf("query connection row failed: %v", err)
	}
	if gotOwner != ownerID || gotTenant != tenantID || gotAccess != access || gotRefresh != refresh {
		t.Fatalf("unexpected connection row values: gotOwner=%s gotTenant=%s gotAccess=%s gotRefresh=%s", gotOwner, gotTenant, gotAccess, gotRefresh)
	}
	now := time.Now().Unix()
	if gotExpires < now || gotExpires > now+expires+5 {
		t.Fatalf("expires_at unexpected: got %d expected around %d", gotExpires, now+expires)
	}

	// update existing row
	newAccess := "tok-2"
	newRefresh := "ref-2"
	newExpires := int64(7200)
	if err := UpsertConnection(ctx, dbURL, ownerID, tenantID, newAccess, newRefresh, newExpires); err != nil {
		t.Fatalf("UpsertConnection (update) failed: %v", err)
	}
	var updAccess, updRefresh string
	var updExpires int64
	if err := pool.QueryRow(ctx, `SELECT access_token, refresh_token, expires_at FROM xero_connections WHERE id = $1`, ownerID+":"+tenantID).
		Scan(&updAccess, &updRefresh, &updExpires); err != nil {
		t.Fatalf("query updated connection failed: %v", err)
	}
	if updAccess != newAccess || updRefresh != newRefresh {
		t.Fatalf("expected updated tokens, got access=%s refresh=%s", updAccess, updRefresh)
	}
	if updExpires < now || updExpires > now+newExpires+5 {
		t.Fatalf("updated expires_at unexpected: got %d expected around %d", updExpires, now+newExpires)
	}
}

func TestAddShoppingListEntry_InsertRows(t *testing.T) {
	// do not run in parallel due to docker container usage
	dbURL, cleanup := setupTestPostgresXero(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// empty db url -> error
	if err := AddShoppingListEntry(ctx, "", "P-1", 1, false); err == nil {
		t.Fatal("expected error for empty db url")
	}

	if err := AddShoppingListEntry(ctx, dbURL, "P-1", 2, false); err != nil {
		t.Fatalf("AddShoppingListEntry failed: %v", err)
	}
	if err := AddShoppingListEntry(ctx, dbURL, "P-2", 3, true); err != nil {
		t.Fatalf("AddShoppingListEntry failed: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `SELECT item_id, quantity, ordered FROM shopping_list ORDER BY list_id`)
	if err != nil {
		t.Fatalf("query shopping_list: %v", err)
	}
	defer rows.Close()

	var got []struct {
		ItemID   string
		Quantity int
		Ordered  bool
	}
	for rows.Next() {
		var r struct {
			ItemID   string
			Quantity int
			Ordered  bool
		}
		if err := rows.Scan(&r.ItemID, &r.Quantity, &r.Ordered); err != nil {
			t.Fatalf("scan shopping_list row: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 shopping_list rows, got %d", len(got))
	}
	if got[0].ItemID != "P-1" || got[0].Quantity != 2 || got[0].Ordered {
		t.Fatalf("unexpected first shopping row: %#v", got[0])
	}
	if got[1].ItemID != "P-2" || got[1].Quantity != 3 || !got[1].Ordered {
		t.Fatalf("unexpected second shopping row: %#v", got[1])
	}
}
