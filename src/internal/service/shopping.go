package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ShoppingRow is a row from shopping_list.
type ShoppingRow struct {
	ListID   int
	PartID   string
	Quantity int
}

// SupplierItem represents an item assigned to a supplier; ListIDs tracks source rows.
type SupplierItem struct {
	PartID   string
	Quantity int
	ListIDs  []int
}

// GetUnorderedShoppingRows returns all shopping_list rows where ordered = false.
func GetUnorderedShoppingRows(ctx context.Context, dbURL string) ([]ShoppingRow, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `SELECT list_id, part_id, quantity FROM shopping_list WHERE ordered = FALSE`)
	if err != nil {
		return nil, fmt.Errorf("query shopping_list: %w", err)
	}
	defer rows.Close()

	var out []ShoppingRow
	for rows.Next() {
		var r ShoppingRow
		if err := rows.Scan(&r.ListID, &r.PartID, &r.Quantity); err != nil {
			return nil, fmt.Errorf("scan shopping row: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

// GroupShoppingItemsBySupplier assigns each shopping row to a supplier and aggregates duplicates.
// It selects the first supplier_name found for a part. If a part has no supplier -> error.
func GroupShoppingItemsBySupplier(ctx context.Context, dbURL string, rows []ShoppingRow) (map[string][]SupplierItem, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("db url missing")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	// map supplier_name -> (part -> SupplierItem)
	groupMap := map[string]map[string]*SupplierItem{}

	for _, r := range rows {
		var supplierID string
		err := pool.QueryRow(ctx, `SELECT supplier_id FROM parts_suppliers WHERE part_id = $1 LIMIT 1`, r.PartID).Scan(&supplierID)
		if err != nil {
			return nil, fmt.Errorf("no supplier found for part %s", r.PartID)
		}
		if _, ok := groupMap[supplierID]; !ok {
			groupMap[supplierID] = map[string]*SupplierItem{}
		}
		if existing, ok := groupMap[supplierID][r.PartID]; ok {
			existing.Quantity += r.Quantity
			existing.ListIDs = append(existing.ListIDs, r.ListID)
		} else {
			groupMap[supplierID][r.PartID] = &SupplierItem{
				PartID:   r.PartID,
				Quantity: r.Quantity,
				ListIDs:  []int{r.ListID},
			}
		}
	}

	// convert to desired output shape
	out := map[string][]SupplierItem{}
	for sup, m := range groupMap {
		for _, v := range m {
			out[sup] = append(out[sup], *v)
		}
	}
	return out, nil
}

// MarkShoppingListOrdered sets ordered = true for the given list IDs and updates updated_at.
func MarkShoppingListOrdered(ctx context.Context, dbURL string, ids []int) error {
	if dbURL == "" {
		return fmt.Errorf("db url missing")
	}
	if len(ids) == 0 {
		return nil
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `
UPDATE shopping_list
SET ordered = TRUE, updated_at = (extract(epoch from now()))::bigint
WHERE list_id = ANY($1)
`, ids)
	if err != nil {
		return fmt.Errorf("update shopping_list: %w", err)
	}
	return nil
}
