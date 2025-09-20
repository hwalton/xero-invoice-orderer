package service

import (
	"context"
	"fmt"

	"github.com/hwalton/freeride-campervans/pkg/xero"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoadParts loads parts from the primary DB and returns them as pkg/xero.Part.
// This mirrors the query used by the control-panel commands but lives in service for reuse.
func LoadParts(ctx context.Context, dbURL string) ([]xero.Part, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	// Return only the columns required by pkg/xero.Part to match Scan below.
	rows, err := pool.Query(ctx, `
SELECT
  part_id,
  COALESCE(name, '') AS name,
  COALESCE(description, '') AS description,
  COALESCE(cost_price, 0)::float8 AS cost_price,
  COALESCE(sales_price, 0)::float8 AS sales_price
FROM parts
`)
	if err != nil {
		return nil, fmt.Errorf("query parts: %w", err)
	}
	defer rows.Close()

	var parts []xero.Part
	for rows.Next() {
		var p xero.Part
		if err := rows.Scan(
			&p.PartID,
			&p.Name,
			&p.Description,
			&p.CostPrice,
			&p.SalesPrice,
		); err != nil {
			return nil, fmt.Errorf("scan part: %w", err)
		}
		parts = append(parts, p)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows error: %w", rows.Err())
	}
	return parts, nil
}
