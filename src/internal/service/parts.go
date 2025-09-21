package service

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hwalton/freeride-campervans/pkg/xero"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BOMNode represents a part/assembly for the UI (assemblies have children and no qty input).
type BOMNode struct {
	PartID     string    `json:"part_id"`
	Name       string    `json:"name"`
	Quantity   float64   `json:"quantity"`    // effective qty (multiplied up the tree)
	IsAssembly bool      `json:"is_assembly"` // true when node expands into children
	Children   []BOMNode `json:"children,omitempty"`
}

// RootItem is an invoice line root for resolution.
type RootItem struct {
	PartID   string  `json:"part_id"`
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
}

// ResolveInvoiceBOM expands invoice roots into a tree of purchasable leaves.
// Uses Xero for item metadata; Supabase for relationships and item->contact mapping.
// item IDs must be Xero Item Code; contacts are Xero AccountNumber in items_contacts.
func ResolveInvoiceBOM(ctx context.Context, dbURL string, roots []RootItem, maxDepth int, httpClient *http.Client, accessToken, tenantID string) ([]BOMNode, string, error) {
	if dbURL == "" {
		return nil, "", fmt.Errorf("db url missing")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, "", fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	// helpers
	getItem := func(ctx context.Context, code string) (name string, exists bool, err error) {
		name, ok, err := xero.GetItemNameByCode(ctx, httpClient, accessToken, tenantID, code)
		if err != nil {
			return "", false, err
		}
		return name, ok, nil
	}
	hasContact := func(ctx context.Context, id string) (bool, error) {
		var c int
		if err := pool.QueryRow(ctx, `SELECT COUNT(1) FROM items_contacts WHERE item_id = $1`, id).Scan(&c); err != nil {
			return false, err
		}
		return c > 0, nil
	}
	getChildren := func(ctx context.Context, id string) (pairs [][2]interface{}, err error) {
		rows, err := pool.Query(ctx, `SELECT child_id, quantity FROM parent_child WHERE parent_id = $1`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out [][2]interface{}
		for rows.Next() {
			var child string
			var qty int
			if err := rows.Scan(&child, &qty); err != nil {
				return nil, err
			}
			out = append(out, [2]interface{}{child, qty})
		}
		return out, rows.Err()
	}

	type stackKey struct{ id string }
	visiting := map[stackKey]bool{}

	var dfs func(ctx context.Context, id, displayName string, qty float64, depth int) (BOMNode, string, bool, error)
	dfs = func(ctx context.Context, id, displayName string, qty float64, depth int) (BOMNode, string, bool, error) {
		if depth > maxDepth {
			return BOMNode{}, fmt.Sprintf("max depth exceeded while resolving item %s (possible circular reference)", id), false, nil
		}
		k := stackKey{id: id}
		if visiting[k] {
			return BOMNode{}, fmt.Sprintf("circular parent/child relationship detected at %s", id), false, nil
		}
		visiting[k] = true
		defer func() { delete(visiting, k) }()

		// ensure item exists (via Xero)
		name, exists, err := getItem(ctx, id)
		if err != nil {
			return BOMNode{}, "", false, err
		}
		if !exists {
			return BOMNode{}, fmt.Sprintf("item %s not found in Xero", id), false, nil
		}
		if displayName == "" {
			displayName = name
		}

		// purchasable leaf if it has a contact mapping
		okContact, err := hasContact(ctx, id)
		if err != nil {
			return BOMNode{}, "", false, err
		}
		if okContact {
			return BOMNode{
				PartID:     id,
				Name:       displayName,
				Quantity:   qty,
				IsAssembly: false,
				Children:   nil,
			}, "", true, nil
		}

		// expand children (from Supabase)
		children, err := getChildren(ctx, id)
		if err != nil {
			return BOMNode{}, "", false, err
		}
		if len(children) == 0 {
			return BOMNode{}, fmt.Sprintf("item %s has no supplier contact and no subcomponents", id), false, nil
		}

		node := BOMNode{
			PartID:     id,
			Name:       displayName,
			Quantity:   qty,
			IsAssembly: true,
		}
		for _, pair := range children {
			childID := pair[0].(string)
			childQty := float64(pair[1].(int))
			childNode, errMsg, ok, err := dfs(ctx, childID, "", qty*childQty, depth+1)
			if err != nil {
				return BOMNode{}, "", false, err
			}
			if errMsg != "" {
				return BOMNode{}, errMsg, false, nil
			}
			if !ok {
				return BOMNode{}, "unexpected resolve failure", false, nil
			}
			node.Children = append(node.Children, childNode)
		}
		return node, "", true, nil
	}

	var rootsOut []BOMNode
	for _, r := range roots {
		node, errMsg, ok, err := dfs(ctx, r.PartID, r.Name, r.Quantity, 1)
		if err != nil {
			return nil, "", err
		}
		if errMsg != "" || !ok {
			// any error aborts and return message
			return nil, errMsg, nil
		}
		rootsOut = append(rootsOut, node)
	}
	return rootsOut, "", nil
}

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
