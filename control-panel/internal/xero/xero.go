package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// Part is the local representation of a part you want to push to Xero.
// Trimmed to match current parts table columns.
type Part struct {
	PartID     string  `json:"part_id"`
	Name       string  `json:"name"`
	CostPrice  float64 `json:"cost_price"`
	SalesPrice float64 `json:"sales_price"`
}

// SyncPartsToXero sends parts to the Xero Items endpoint. Access token must be valid.
// tenantID is the Xero tenant (organization) id obtained from /connections.
func SyncPartsToXero(ctx context.Context, httpClient *http.Client, accessToken, tenantID string, items []Part) error {
	if len(items) == 0 {
		return nil
	}
	type simplePrice struct {
		UnitPrice float64 `json:"UnitPrice"`
	}
	type itemPayload struct {
		Code     string       `json:"Code"`
		Name     string       `json:"Name"`
		Purchase *simplePrice `json:"PurchaseDetails,omitempty"` // Xero expects PurchaseDetails; we map CostPrice -> PurchaseDetails.UnitPrice
		Sales    *simplePrice `json:"SalesDetails,omitempty"`
	}

	out := make([]itemPayload, 0, len(items))
	for _, p := range items {
		ip := itemPayload{
			Code: p.PartID,
			Name: p.Name,
		}
		if p.CostPrice > 0 {
			ip.Purchase = &simplePrice{UnitPrice: p.CostPrice}
		}
		if p.SalesPrice > 0 {
			ip.Sales = &simplePrice{UnitPrice: p.SalesPrice}
		}
		out = append(out, ip)
	}

	payload := map[string]interface{}{
		"Items": out,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal items: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.xero.com/api.xro/2.0/Items", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Xero-tenant-id", tenantID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// debug: confirm what we're sending (do not log token in public)
	log.Printf("xero: tenant=%s items=%d", tenantID, len(items))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("xero import failed: status=%d body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("xero import failed: status=%d body=%q", resp.StatusCode, string(body))
	}
	// success; Xero returns created items in response body (can parse if needed)
	return nil
}
