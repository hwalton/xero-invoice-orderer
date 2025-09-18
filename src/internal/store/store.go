package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// XeroConnection is the persisted record.
type XeroConnection struct {
	ID           string
	OwnerID      string
	TenantID     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UpsertConnection upserts a connection. id will be ownerID:tenantID to ensure uniqueness.
func UpsertConnection(ctx context.Context, dbURL, ownerID, tenantID, accessToken, refreshToken string, expiresInSeconds int64) error {
	if dbURL == "" {
		return fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	id := ownerID + ":" + tenantID
	// compute expires_at using seconds
	_, err = pool.Exec(ctx, `
INSERT INTO xero_connections (id, owner_id, tenant_id, access_token, refresh_token, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now() + ($6 || ' seconds')::interval, now(), now())
ON CONFLICT (id) DO UPDATE
SET access_token = EXCLUDED.access_token,
    refresh_token = EXCLUDED.refresh_token,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
`, id, ownerID, tenantID, accessToken, refreshToken, fmt.Sprintf("%d", expiresInSeconds))
	if err != nil {
		return fmt.Errorf("upsert connection: %w", err)
	}
	return nil
}

// GetConnectionsForOwner returns stored connections for an owner.
func GetConnectionsForOwner(ctx context.Context, dbURL, ownerID string) ([]XeroConnection, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `SELECT id, owner_id, tenant_id, access_token, refresh_token, expires_at, created_at, updated_at FROM xero_connections WHERE owner_id = $1`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("query connections: %w", err)
	}
	defer rows.Close()

	var out []XeroConnection
	for rows.Next() {
		var xc XeroConnection
		if err := rows.Scan(&xc.ID, &xc.OwnerID, &xc.TenantID, &xc.AccessToken, &xc.RefreshToken, &xc.ExpiresAt, &xc.CreatedAt, &xc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conn: %w", err)
		}
		out = append(out, xc)
	}
	return out, nil
}
