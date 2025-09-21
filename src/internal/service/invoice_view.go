package service

import "math"

// LeafTotal is a flat total per purchasable part.
type LeafTotal struct {
	PartID   string  `json:"part_id"`
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
}

// BuildPerAssemblyBOM converts an "effective totals" BOM into a per-assembly tree:
//   - Root nodes show the invoice quantity (as-is).
//   - For every non-root node, Quantity = child.EffectiveQty / parent.EffectiveQty.
//     This yields "Qty required (for each Assy)" at every level.
func BuildPerAssemblyBOM(bom []BOMNode, roots []RootItem) []BOMNode {
	min := len(bom)
	if len(roots) < min {
		min = len(roots)
	}
	out := make([]BOMNode, 0, min)

	// norm builds a per-assembly subtree under a given parent effective quantity.
	var norm func(node BOMNode, parentEffective float64) BOMNode
	norm = func(node BOMNode, parentEffective float64) BOMNode {
		// Per-assembly qty = node.Effective / parent.Effective
		perAssyQty := node.Quantity
		if parentEffective > 0 {
			perAssyQty = node.Quantity / parentEffective
		}
		res := BOMNode{
			PartID:     node.PartID,
			Name:       node.Name,
			Quantity:   perAssyQty,
			IsAssembly: node.IsAssembly,
		}
		for _, ch := range node.Children {
			// Pass this node's effective quantity down as the parent effective for children.
			res.Children = append(res.Children, norm(ch, node.Quantity))
		}
		return res
	}

	for i := 0; i < min; i++ {
		rootEff := bom[i].Quantity
		// Root shows the invoice quantity exactly (not normalized).
		root := BOMNode{
			PartID:     bom[i].PartID,
			Name:       bom[i].Name,
			Quantity:   roots[i].Quantity,
			IsAssembly: bom[i].IsAssembly,
		}
		for _, ch := range bom[i].Children {
			root.Children = append(root.Children, norm(ch, rootEff))
		}
		out = append(out, root)
	}
	return out
}

// AggregateLeafTotals multiplies quantities down the per-assembly tree to produce
// total quantities for each purchasable leaf across all roots (handles multi-tier).
// Returned quantities are rounded to the nearest integer for form defaults.
func AggregateLeafTotals(perAssy []BOMNode) []LeafTotal {
	agg := map[string]*LeafTotal{}

	var multWalk func(node BOMNode, mul float64)
	multWalk = func(node BOMNode, mul float64) {
		if node.IsAssembly {
			nextMul := mul
			if node.Quantity > 0 {
				nextMul = mul * node.Quantity
			}
			for _, ch := range node.Children {
				multWalk(ch, nextMul)
			}
			return
		}
		total := mul * node.Quantity
		if lt, ok := agg[node.PartID]; ok {
			lt.Quantity += total
		} else {
			agg[node.PartID] = &LeafTotal{PartID: node.PartID, Name: node.Name, Quantity: total}
		}
	}

	for _, root := range perAssy {
		multWalk(root, 1)
	}

	out := make([]LeafTotal, 0, len(agg))
	for _, v := range agg {
		v.Quantity = math.Round(v.Quantity)
		out = append(out, *v)
	}
	return out
}
