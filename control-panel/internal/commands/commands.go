package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/hwalton/psqltoolbox"
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
