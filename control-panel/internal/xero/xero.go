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
type Part struct {
	PartID                    string  `json:"part_id"`
	Name                      string  `json:"name"`
	Description               string  `json:"description"`
	SalesPrice                float64 `json:"sales_price"`
	PurchasePrice             float64 `json:"purchase_price"`
	SalesAccountCode          string  `json:"sales_account_code,omitempty"`
	PurchaseAccountCode       string  `json:"purchase_account_code,omitempty"`
	TaxType                   string  `json:"tax_type,omitempty"`
	IsTrackedAsInventory      bool    `json:"is_tracked_as_inventory,omitempty"`
	InventoryAssetAccountCode string  `json:"inventory_asset_account_code,omitempty"`
	BarCode                   string  `json:"barcode,omitempty"`
}

// ImportItemsToXero sends parts to the Xero Items endpoint. Access token must be valid.
// tenantID is the Xero tenant (organization) id obtained from /connections.
func SyncPartsToXero(ctx context.Context, httpClient *http.Client, accessToken, tenantID string, items []Part) error {
	if len(items) == 0 {
		return nil
	}
	type priceDetails struct {
		UnitPrice   float64 `json:"UnitPrice"`
		AccountCode string  `json:"AccountCode,omitempty"`
		TaxType     string  `json:"TaxType,omitempty"`
	}
	type itemPayload struct {
		Code                      string       `json:"Code"`
		Name                      string       `json:"Name"`
		Description               string       `json:"Description,omitempty"`
		PurchaseDetails           priceDetails `json:"PurchaseDetails,omitempty"`
		SalesDetails              priceDetails `json:"SalesDetails,omitempty"`
		IsTrackedAsInventory      bool         `json:"IsTrackedAsInventory,omitempty"`
		InventoryAssetAccountCode string       `json:"InventoryAssetAccountCode,omitempty"`
		BarCode                   string       `json:"BarCode,omitempty"`
	}

	// transform local Part -> xero item payload
	out := make([]itemPayload, 0, len(items))
	for _, p := range items {
		ip := itemPayload{
			Code:        p.PartID,
			Name:        p.Name,
			Description: p.Description,
			PurchaseDetails: priceDetails{
				UnitPrice:   p.PurchasePrice,
				AccountCode: p.PurchaseAccountCode,
				TaxType:     p.TaxType,
			},
			SalesDetails: priceDetails{
				UnitPrice:   p.SalesPrice,
				AccountCode: p.SalesAccountCode,
				TaxType:     p.TaxType,
			},
			IsTrackedAsInventory:      p.IsTrackedAsInventory,
			InventoryAssetAccountCode: p.InventoryAssetAccountCode,
			BarCode:                   p.BarCode,
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
	log.Printf("xero: tenant=%s auth_len=%d items=%d", tenantID, len(accessToken), len(items))
	// log.Printf("xero request body: %s", string(b))

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
