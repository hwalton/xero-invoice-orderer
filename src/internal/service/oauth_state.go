package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateOAuthState stores a one-time state with TTL (seconds).
func CreateOAuthState(ctx context.Context, dbURL, state, ownerID string, ttlSeconds int) error {
	if dbURL == "" {
		return fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
	createdAt := time.Now().Unix()

	_, err = pool.Exec(ctx, `
INSERT INTO oauth_states (state, owner_id, expires_at, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (state) DO UPDATE
  SET owner_id = EXCLUDED.owner_id,
      expires_at = EXCLUDED.expires_at,
      created_at = EXCLUDED.created_at
`, state, ownerID, expiresAt, createdAt)
	if err != nil {
		return fmt.Errorf("insert oauth_state: %w", err)
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
	nowEpoch := time.Now().Unix()
	// atomic delete + return owner_id if not expired (compare against unix epoch seconds)
	err = pool.QueryRow(ctx, `
DELETE FROM oauth_states
WHERE state = $1 AND expires_at > $2
RETURNING owner_id
`, state, nowEpoch).Scan(&ownerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			// expired or missing
			return "", false, nil
		}
		return "", false, fmt.Errorf("consume oauth_state: %w", err)
	}
	return ownerID, true, nil
}
