package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ShoppingRow is a row from shopping_list.
type ShoppingRow struct {
	ListID   int
	ItemID   string
	Quantity int
}

// ContactItem represents an item assigned to a contact; ListIDs tracks source rows.
type ContactItem struct {
	ItemID   string
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

	rows, err := pool.Query(ctx, `SELECT list_id, item_id, quantity FROM shopping_list WHERE ordered = FALSE`)
	if err != nil {
		return nil, fmt.Errorf("query shopping_list: %w", err)
	}
	defer rows.Close()

	var out []ShoppingRow
	for rows.Next() {
		var r ShoppingRow
		if err := rows.Scan(&r.ListID, &r.ItemID, &r.Quantity); err != nil {
			return nil, fmt.Errorf("scan shopping row: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

// GroupShoppingItemsByContact assigns each shopping row to a contact (AccountNumber) and aggregates duplicates.
// If an item has no contact mapping -> error.
func GroupShoppingItemsByContact(ctx context.Context, dbURL string, rows []ShoppingRow) (map[string][]ContactItem, error) {
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

	// map contact_account_number -> (item -> ContactItem)
	groupMap := map[string]map[string]*ContactItem{}

	for _, r := range rows {
		var contactID string // Xero Contacts.AccountNumber
		err := pool.QueryRow(ctx, `SELECT contact_id FROM items_contacts WHERE item_id = $1 LIMIT 1`, r.ItemID).Scan(&contactID)
		if err != nil {
			return nil, fmt.Errorf("no contact mapping found for item %s", r.ItemID)
		}
		if _, ok := groupMap[contactID]; !ok {
			groupMap[contactID] = map[string]*ContactItem{}
		}
		if existing, ok := groupMap[contactID][r.ItemID]; ok {
			existing.Quantity += r.Quantity
			existing.ListIDs = append(existing.ListIDs, r.ListID)
		} else {
			groupMap[contactID][r.ItemID] = &ContactItem{
				ItemID:   r.ItemID,
				Quantity: r.Quantity,
				ListIDs:  []int{r.ListID},
			}
		}
	}

	// convert to desired output shape
	out := map[string][]ContactItem{}
	for contact, m := range groupMap {
		for _, v := range m {
			out[contact] = append(out[contact], *v)
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
