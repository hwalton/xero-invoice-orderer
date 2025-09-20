package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// XeroConnection is the persisted record.
type XeroConnection struct {
	ID           string
	OwnerID      string
	TenantID     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
	CreatedAt    int64
	UpdatedAt    int64
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
