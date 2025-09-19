package commands

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/hwalton/psqltoolbox"

	"github.com/hwalton/freeride-campervans/control-panel/internal/xero"
)

func RunMigrationsUp(isProd bool) error {
	var dbURL string
	var ok bool

	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
	}

	migrationsPath := "../../../migrations"

	fmt.Printf("[%s] Running DB migrations from %s...\n", time.Now().Format(time.RFC3339), migrationsPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "migrate", "-database", dbURL, "-path", migrationsPath, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("migrate up failed: %w", err)
	}

	fmt.Printf("[%s] Migrations applied.\n", time.Now().Format(time.RFC3339))
	return nil
}

func ResetDBDev() error {
	// load env vars
	var dbURL string
	var ok bool

	dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
	if !ok || dbURL == "" {
		return fmt.Errorf("DEV_SUPABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	migrationsPath := "../../../migrations"

	// drop all tables and optionally run migrations
	if err := psqltoolbox.DropTablesAndMigrate(ctx, conn, dbURL, migrationsPath); err != nil {
		return err
	}

	return nil
}

func SyncPartsToXero(isProd bool) error {
	// pick DB URL
	var dbURL string
	var ok bool
	var accessToken string
	var tenantID string
	if isProd {
		dbURL, ok = os.LookupEnv("PROD_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("PROD_SUPABASE_URL not set")
		}
		accessToken, ok := os.LookupEnv("PROD_XERO_ACCESS_TOKEN")
		if !ok || accessToken == "" {
			return fmt.Errorf("PROD_XERO_ACCESS_TOKEN not set")
		}
		tenantID, ok := os.LookupEnv("PROD_XERO_TENANT_ID")
		if !ok || tenantID == "" {
			return fmt.Errorf("PROD_XERO_TENANT_ID not set")
		}
	} else {
		dbURL, ok = os.LookupEnv("DEV_SUPABASE_URL")
		if !ok || dbURL == "" {
			return fmt.Errorf("DEV_SUPABASE_URL not set")
		}
		accessToken, ok = os.LookupEnv("DEV_XERO_ACCESS_TOKEN")
		if !ok || accessToken == "" {
			return fmt.Errorf("DEV_XERO_ACCESS_TOKEN not set")
		}
		tenantID, ok = os.LookupEnv("DEV_XERO_TENANT_ID")
		if !ok || tenantID == "" {
			return fmt.Errorf("DEV_XERO_TENANT_ID not set")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := connectDB(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := conn.Close(ctx); cerr != nil {
			log.Printf("warning: failed to close db connection: %v", cerr)
		}
	}()

	// load parts from DB (only current columns: part_id, name, description, cost_price, sales_price)
	rows, err := conn.Query(ctx, `
SELECT
  part_id,
  COALESCE(name, '') AS name,
  COALESCE(cost_price, 0)::float8 AS cost_price,
  COALESCE(sales_price, 0)::float8 AS sales_price
FROM parts
`)
	if err != nil {
		return fmt.Errorf("query parts: %w", err)
	}
	defer rows.Close()

	var parts []xero.Part
	for rows.Next() {
		var p xero.Part
		if err := rows.Scan(
			&p.PartID,
			&p.Name,
			&p.CostPrice,
			&p.SalesPrice,
		); err != nil {
			return fmt.Errorf("scan part: %w", err)
		}
		parts = append(parts, p)
	}
	if rows.Err() != nil {
		return fmt.Errorf("rows error: %w", rows.Err())
	}

	if len(parts) == 0 {
		log.Printf("no parts found to sync")
		return nil
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	log.Printf("syncing %d parts to Xero (prod=%v)", len(parts), isProd)
	if err := xero.SyncPartsToXero(ctx, httpClient, accessToken, tenantID, parts); err != nil {
		return fmt.Errorf("sync to xero failed: %w", err)
	}

	log.Printf("sync to xero completed")
	return nil
}
