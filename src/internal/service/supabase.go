package service

import (
	"context"
	"fmt"
	"time"

	"github.com/hwalton/freeride-campervans/pkg/xero"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

	nowEpoch := time.Now().Unix()
	expiresAt := nowEpoch + expiresInSeconds
	createdAt := nowEpoch
	updatedAt := nowEpoch

	_, err = pool.Exec(ctx, `
INSERT INTO xero_connections (id, owner_id, tenant_id, access_token, refresh_token, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE
SET access_token = EXCLUDED.access_token,
    refresh_token = EXCLUDED.refresh_token,
    expires_at = EXCLUDED.expires_at,
    updated_at = EXCLUDED.updated_at
`, id, ownerID, tenantID, accessToken, refreshToken, expiresAt, createdAt, updatedAt)
	if err != nil {
		return fmt.Errorf("upsert connection: %w", err)
	}
	return nil
}

// AddShoppingListEntry inserts a row into shopping_list for the given item and quantity.
func AddShoppingListEntry(ctx context.Context, dbURL, itemID string, quantity int, ordered bool) error {
	if dbURL == "" {
		return fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
INSERT INTO shopping_list (item_id, quantity, ordered, created_at)
VALUES ($1, $2, $3, (extract(epoch from now()))::bigint)
`, itemID, quantity, ordered)
	if err != nil {
		return fmt.Errorf("insert shopping_list: %w", err)
	}
	return nil
}

// LoadSuppliers reads suppliers table and returns as pkg/xero.Supplier slice.
func LoadSuppliers(ctx context.Context, dbURL string) ([]xero.Supplier, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
SELECT supplier_id, COALESCE(supplier_name, '') AS supplier_name, COALESCE(contact_email, '') AS contact_email, COALESCE(phone, '') AS phone
FROM suppliers
`)
	if err != nil {
		return nil, fmt.Errorf("query suppliers: %w", err)
	}
	defer rows.Close()

	var out []xero.Supplier
	for rows.Next() {
		var s xero.Supplier
		if err := rows.Scan(&s.SupplierID, &s.SupplierName, &s.ContactEmail, &s.Phone); err != nil {
			return nil, fmt.Errorf("scan supplier: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return out, nil
}
