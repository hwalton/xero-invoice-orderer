package service

import (
	"reflect"
	"testing"
)

func TestBuildPerAssemblyBOM_SimpleHierarchy(t *testing.T) {
	t.Parallel()

	// Simulate ResolveInvoiceBOM output (effective totals)
	// Invoice: 1 x KIT-001
	// KIT-001 effective total = 1
	// KIT-002 effective total = 2  (means invoice needs 2 KIT-002)
	//   P-0001 effective total (under KIT-002) = 6 (2 * 3 per KIT-002)
	// P-0001 direct under KIT-001 effective total = 4
	bom := []BOMNode{
		{
			PartID:     "KIT-001",
			Name:       "Kitchen Unit",
			Quantity:   1, // effective total for root
			IsAssembly: true,
			Children: []BOMNode{
				{
					PartID:     "KIT-002",
					Name:       "Sub Kit",
					Quantity:   2, // effective total across invoice
					IsAssembly: true,
					Children: []BOMNode{
						{
							PartID:     "P-0001",
							Name:       "Plywood",
							Quantity:   6, // effective total (2 * 3)
							IsAssembly: false,
						},
					},
				},
				{
					PartID:     "P-0001",
					Name:       "Plywood",
					Quantity:   4, // direct effective total under root
					IsAssembly: false,
				},
			},
		},
	}

	roots := []RootItem{
		{PartID: "KIT-001", Name: "Kitchen Unit", Quantity: 1}, // invoice qty
	}

	per := BuildPerAssemblyBOM(bom, roots)

	// Expect:
	// root.Quantity == roots[0].Quantity (invoice qty)
	// KIT-002 perAssy = KIT-002.eff / root.eff = 2 / 1 = 2
	// P-0001 under KIT-002 perAssy = 6 / 2 = 3
	// P-0001 direct under root perAssy = 4 / 1 = 4

	if len(per) != 1 {
		t.Fatalf("expected 1 root in per-assembly BOM, got %d", len(per))
	}
	root := per[0]
	if root.Quantity != 1 {
		t.Fatalf("expected root quantity 1 (invoice qty), got %v", root.Quantity)
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children under root, got %d", len(root.Children))
	}

	// child 0 is KIT-002
	k2 := root.Children[0]
	if k2.PartID != "KIT-002" {
		t.Fatalf("expected first child KIT-002, got %s", k2.PartID)
	}
	if got := k2.Quantity; got != 2 {
		t.Fatalf("expected KIT-002 per-assembly qty 2, got %v", got)
	}
	// its child P-0001 per-assembly should be 3
	if len(k2.Children) != 1 {
		t.Fatalf("expected KIT-002 to have 1 child, got %d", len(k2.Children))
	}
	if p := k2.Children[0]; p.PartID != "P-0001" || p.Quantity != 3 {
		t.Fatalf("expected KIT-002 child P-0001 qty 3, got %v qty=%v", p.PartID, p.Quantity)
	}

	// direct P-0001 under root
	pDirect := root.Children[1]
	if pDirect.PartID != "P-0001" || pDirect.Quantity != 4 {
		t.Fatalf("expected direct P-0001 qty 4, got %v qty=%v", pDirect.PartID, pDirect.Quantity)
	}
}

func TestAggregateLeafTotals_MultiTierTotals(t *testing.T) {
	t.Parallel()

	// Build per-assembly tree (as BuildPerAssemblyBOM would produce)
	// Root: KIT-001 (invoice qty 1)
	//  - KIT-002 (perAssy qty 2)
	//     - P-0001 (perAssy qty 3) -> contributes 2 * 3 = 6
	//  - P-0001 (perAssy qty 4) -> contributes 1 * 4 = 4
	perAssy := []BOMNode{
		{
			PartID:     "KIT-001",
			Name:       "Kitchen Unit",
			Quantity:   1,
			IsAssembly: true,
			Children: []BOMNode{
				{
					PartID:     "KIT-002",
					Name:       "Sub Kit",
					Quantity:   2,
					IsAssembly: true,
					Children: []BOMNode{
						{
							PartID:     "P-0001",
							Name:       "Plywood",
							Quantity:   3,
							IsAssembly: false,
						},
					},
				},
				{
					PartID:     "P-0001",
					Name:       "Plywood",
					Quantity:   4,
					IsAssembly: false,
				},
			},
		},
	}

	leafTotals := AggregateLeafTotals(perAssy)

	// Aggregate into map for assertions
	m := map[string]float64{}
	for _, lt := range leafTotals {
		m[lt.PartID] = lt.Quantity
	}

	if got, ok := m["P-0001"]; !ok {
		t.Fatalf("expected P-0001 in leaf totals")
	} else if got != 10 {
		t.Fatalf("expected P-0001 total 10, got %v", got)
	}

	// Also ensure no unexpected parts
	if len(m) != 1 {
		t.Fatalf("expected only 1 leaf part, got %d map=%v", len(m), m)
	}
}

func TestBuildAndAggregate_RoundTrip(t *testing.T) {
	t.Parallel()

	// Create effective-bom for two roots to ensure aggregation combines across roots.
	// Root A: invoice qty 2 -> effective root qty 2
	//   child P-1 effective 6 (so perAssy = 6/2 = 3)
	// Root B: invoice qty 3 -> effective root qty 3
	//   child P-1 effective 9 (so perAssy = 9/3 = 3)
	bom := []BOMNode{
		{
			PartID:     "ROOT-A",
			Name:       "Root A",
			Quantity:   2, // effective total
			IsAssembly: true,
			Children: []BOMNode{
				{PartID: "P-1", Name: "Part1", Quantity: 6, IsAssembly: false},
			},
		},
		{
			PartID:     "ROOT-B",
			Name:       "Root B",
			Quantity:   3,
			IsAssembly: true,
			Children: []BOMNode{
				{PartID: "P-1", Name: "Part1", Quantity: 9, IsAssembly: false},
			},
		},
	}
	roots := []RootItem{
		{PartID: "ROOT-A", Name: "Root A", Quantity: 2},
		{PartID: "ROOT-B", Name: "Root B", Quantity: 3},
	}

	per := BuildPerAssemblyBOM(bom, roots)
	if len(per) != 2 {
		t.Fatalf("expected 2 roots in per-assembly result, got %d", len(per))
	}
	lt := AggregateLeafTotals(per)

	// Both roots require 3 of P-1 per assy, and roots invoice qty are 2 and 3, so totals = (2*3)+(3*3)=6+9=15
	found := false
	for _, v := range lt {
		if v.PartID == "P-1" {
			found = true
			if v.Quantity != 15 {
				t.Fatalf("expected P-1 total 15, got %v", v.Quantity)
			}
		}
	}
	if !found {
		t.Fatalf("P-1 not found in leaf totals: %v", lt)
	}
}

// ensure BuildPerAssemblyBOM preserves children order and structure shape (basic equality helper)
func equalBOM(a, b []BOMNode) bool {
	return reflect.DeepEqual(a, b)
}
