// language: go
package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
)

func setupTestPostgres(t *testing.T) (dbURL string, cleanup func()) {
	t.Helper()

	// allow explicit opt-out
	if os.Getenv("DOCKER_DISABLED") == "1" {
		t.Skip("docker disabled via DOCKER_DISABLED")
	}

	// quick check for docker socket to avoid noisy errors when docker isn't present
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("docker socket not found; skipping docker-dependent tests")
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("could not create dockertest pool (docker may be unavailable): %v", err)
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

	// Ensure cleanup
	cleanupFunc := func() {
		_ = pool.Purge(resource)
	}

	// Build connection string to host port
	hostPort := resource.GetPort("5432/tcp")
	connStr := fmt.Sprintf("postgres://postgres:secret@localhost:%s/postgres?sslmode=disable", hostPort)

	// Wait for postgres to be ready
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

	// create required table
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS items_contacts (
  item_id TEXT NOT NULL,
  contact_id TEXT NOT NULL,
  created_at BIGINT DEFAULT (extract(epoch from now()))::bigint,
  updated_at BIGINT DEFAULT (extract(epoch from now()))::bigint,
  PRIMARY KEY (item_id, contact_id)
);
`)
	if err != nil {
		db.Close()
		_ = pool.Purge(resource)
		t.Fatalf("failed to create items_contacts table: %v", err)
	}

	db.Close()

	return connStr, func() {
		cleanupFunc()
	}
}

func TestGroupShoppingItemsByContact_BasicAggregation(t *testing.T) {
	t.Parallel()

	dbURL, cleanup := setupTestPostgres(t)
	defer cleanup()

	// populate items_contacts mappings
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
INSERT INTO items_contacts (item_id, contact_id) VALUES
  ('P-001', 'C-AAA'),
  ('P-002', 'C-AAA'),
  ('P-003', 'C-BBB')
ON CONFLICT DO NOTHING;
`)
	if err != nil {
		t.Fatalf("failed to insert items_contacts: %v", err)
	}

	// prepare shopping rows (multiple rows for same item to test aggregation)
	rows := []ShoppingRow{
		{ListID: 1, ItemID: "P-001", Quantity: 2},
		{ListID: 2, ItemID: "P-001", Quantity: 3},
		{ListID: 3, ItemID: "P-002", Quantity: 4},
		{ListID: 4, ItemID: "P-003", Quantity: 1},
	}

	grouped, err := GroupShoppingItemsByContact(context.Background(), dbURL, rows)
	if err != nil {
		t.Fatalf("GroupShoppingItemsByContact returned error: %v", err)
	}

	// Expect two contact groups: C-AAA and C-BBB
	if len(grouped) != 2 {
		t.Fatalf("expected 2 contact groups, got %d: %#v", len(grouped), grouped)
	}

	// Validate C-AAA group
	aaa, ok := grouped["C-AAA"]
	if !ok {
		t.Fatalf("expected contact C-AAA present")
	}
	// Convert to map for easier assertions
	mAAA := map[string]ContactItem{}
	for _, ci := range aaa {
		// normalize ListIDs order for deterministic assertion
		sort.Ints(ci.ListIDs)
		mAAA[ci.ItemID] = ci
	}

	if ci, ok := mAAA["P-001"]; !ok {
		t.Fatalf("expected P-001 in C-AAA group")
	} else {
		if ci.Quantity != 5 {
			t.Fatalf("expected P-001 aggregated qty 5, got %d", ci.Quantity)
		}
		if len(ci.ListIDs) != 2 || ci.ListIDs[0] != 1 || ci.ListIDs[1] != 2 {
			t.Fatalf("unexpected P-001 ListIDs: %v", ci.ListIDs)
		}
	}
	if ci, ok := mAAA["P-002"]; !ok {
		t.Fatalf("expected P-002 in C-AAA group")
	} else {
		if ci.Quantity != 4 {
			t.Fatalf("expected P-002 qty 4, got %d", ci.Quantity)
		}
		if len(ci.ListIDs) != 1 || ci.ListIDs[0] != 3 {
			t.Fatalf("unexpected P-002 ListIDs: %v", ci.ListIDs)
		}
	}

	// Validate C-BBB group
	bbb, ok := grouped["C-BBB"]
	if !ok {
		t.Fatalf("expected contact C-BBB present")
	}
	if len(bbb) != 1 {
		t.Fatalf("expected 1 item for C-BBB, got %d", len(bbb))
	}
	if bbb[0].ItemID != "P-003" || bbb[0].Quantity != 1 || len(bbb[0].ListIDs) != 1 || bbb[0].ListIDs[0] != 4 {
		t.Fatalf("unexpected C-BBB item: %#v", bbb[0])
	}
}

func TestGroupShoppingItemsByContact_MissingMappingError(t *testing.T) {
	t.Parallel()

	dbURL, cleanup := setupTestPostgres(t)
	defer cleanup()

	// populate items_contacts with only one mapping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
INSERT INTO items_contacts (item_id, contact_id) VALUES
  ('P-001', 'C-AAA')
ON CONFLICT DO NOTHING;
`)
	if err != nil {
		t.Fatalf("failed to insert items_contacts: %v", err)
	}

	// include a row with an unmapped item P-999 to trigger error
	rows := []ShoppingRow{
		{ListID: 10, ItemID: "P-001", Quantity: 1},
		{ListID: 11, ItemID: "P-999", Quantity: 2}, // no mapping
	}

	_, err = GroupShoppingItemsByContact(context.Background(), dbURL, rows)
	if err == nil {
		t.Fatalf("expected error due to missing mapping for P-999, got nil")
	}
	// basic check that error message references the missing item
	if !contains(err.Error(), "P-999") && !contains(err.Error(), "no contact mapping") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// simple helpers
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// allow running tests locally without docker in some environments by skipping
// when DOCKER_DISABLED env var is set (optional convenience).
func TestMain(m *testing.M) {
	if os.Getenv("DOCKER_DISABLED") == "1" {
		// skip tests that require docker; run as passing to avoid false failures
		os.Exit(0)
	}
	os.Exit(m.Run())
}
