package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateOAuthState stores a state->ownerID mapping with TTL (seconds).
func CreateOAuthState(ctx context.Context, dbURL, state, ownerID string, ttlSeconds int) error {
	if dbURL == "" {
		return fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
INSERT INTO oauth_states (state, owner_id, expires_at, created_at)
VALUES ($1, $2, now() + ($3 || ' seconds')::interval, now())
ON CONFLICT (state) DO UPDATE
SET owner_id = EXCLUDED.owner_id, expires_at = EXCLUDED.expires_at, created_at = EXCLUDED.created_at
`, state, ownerID, fmt.Sprintf("%d", ttlSeconds))
	if err != nil {
		return fmt.Errorf("create oauth state: %w", err)
	}
	return nil
}

// ConsumeOAuthState atomically returns the ownerID for state and deletes the row.
// returns (ownerID, found, error)
func ConsumeOAuthState(ctx context.Context, dbURL, state string) (string, bool, error) {
	if dbURL == "" {
		return "", false, fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return "", false, fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	var ownerID string
	// atomic delete + return owner_id if not expired
	err = pool.QueryRow(ctx, `
DELETE FROM oauth_states
WHERE state = $1 AND expires_at > now()
RETURNING owner_id
`, state).Scan(&ownerID)
	if err != nil {
		// no rows found (expired or missing) will return pgx.ErrNoRows
		return "", false, nil
	}
	return ownerID, true, nil
}
