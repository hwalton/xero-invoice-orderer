package service

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/hwalton/freeride-campervans/pkg/xero"
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

    rows, err := pool.Query(ctx, `
SELECT
  part_id,
  COALESCE(name, '') AS name,
  COALESCE(description, '') AS description,
  COALESCE(barcode, '') AS barcode,
  COALESCE(sales_price, 0)::float8 AS sales_price,
  COALESCE(purchase_price, 0)::float8 AS purchase_price,
  COALESCE(sales_account_code, '') AS sales_account_code,
  COALESCE(purchase_account_code, '') AS purchase_account_code,
  COALESCE(tax_type, '') AS tax_type,
  COALESCE(is_tracked, false) AS is_tracked,
  COALESCE(inventory_asset_account_code, '') AS inventory_asset_account_code
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
            &p.BarCode,
            &p.SalesPrice,
            &p.PurchasePrice,
            &p.SalesAccountCode,
            &p.PurchaseAccountCode,
            &p.TaxType,
            &p.IsTrackedAsInventory,
            &p.InventoryAssetAccountCode,
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