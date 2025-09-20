package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Part mirrors parts used for syncing (minimal).
type Part struct {
	PartID      string  `json:"part_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	SalesPrice  float64 `json:"sales_price"`
	CostPrice   float64 `json:"cost_price"`
}

// TokenResponse represents token exchange/refresh result.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Connection represents a Xero connection entry from /connections
type Connection struct {
	ID         string `json:"id"` // id (may be absent depending on endpoint)
	TenantID   string `json:"tenantId"`
	TenantType string `json:"tenantType"`
	TenantName string `json:"tenantName"`
	CreatedUTC string `json:"createdDateUtc"`
}

// BuildAuthURL builds the Xero authorize URL.
func BuildAuthURL(clientID, redirectURI, state string) string {
	scope := "offline_access accounting.contacts accounting.transactions accounting.settings"
	// url encode redirectURI and state via QueryEscape
	return fmt.Sprintf("https://login.xero.com/identity/connect/authorize?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(scope),
		url.QueryEscape(state),
	)
}

// ExchangeCodeForToken exchanges an authorization code for tokens.
func ExchangeCodeForToken(ctx context.Context, httpClient *http.Client, clientID, clientSecret, code, redirectURI string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://identity.xero.com/connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange failed: status=%d", resp.StatusCode)
	}
	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, httpClient *http.Client, clientID, clientSecret, refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://identity.xero.com/connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refresh failed: status=%d", resp.StatusCode)
	}
	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

// GetConnections calls GET https://api.xero.com/connections and returns parsed connections.
func GetConnections(ctx context.Context, httpClient *http.Client, accessToken string) ([]Connection, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.xero.com/connections", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("/connections failed: status=%d", resp.StatusCode)
	}
	var conns []Connection
	if err := json.NewDecoder(resp.Body).Decode(&conns); err != nil {
		return nil, err
	}
	return conns, nil
}

// helper to find an existing item by Code. returns ItemID if found, empty string if not.
func getItemIDByCode(ctx context.Context, httpClient *http.Client, accessToken, tenantID, code string) (string, error) {
	// build where clause: Code == "CODE"
	where := url.QueryEscape(fmt.Sprintf(`Code=="%s"`, code))
	u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Items?where=%s", where)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Xero-tenant-id", tenantID)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("get item by code failed: status=%d", resp.StatusCode)
	}

	var res struct {
		Items []struct {
			ItemID string `json:"ItemID"`
			Code   string `json:"Code"`
		} `json:"Items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if len(res.Items) == 0 {
		return "", nil
	}
	// return first match
	return res.Items[0].ItemID, nil
}

// SyncPartsToXero posts a minimal Items payload to Xero.
// Keeps payload minimal (Code, Name, Description, SalesDetails.UnitPrice) to avoid account/tax validation.
// Uses upsert behavior: if an item with the same Code exists it will be updated, otherwise created.
// Does not delete or touch items not present in the provided slice.
func SyncPartsToXero(ctx context.Context, httpClient *http.Client, accessToken, tenantID string, items []Part) error {
	if len(items) == 0 {
		return nil
	}
	type simplePrice struct {
		UnitPrice float64 `json:"UnitPrice"`
	}
	type itemPayload struct {
		ItemID      string       `json:"ItemID,omitempty"`
		Code        string       `json:"Code"`
		Name        string       `json:"Name"`
		Description string       `json:"Description,omitempty"`
		Sales       *simplePrice `json:"SalesDetails,omitempty"`
		Purchase    *simplePrice `json:"PurchaseDetails,omitempty"`
	}

	out := make([]itemPayload, 0, len(items))
	for _, p := range items {
		ip := itemPayload{
			Code:        p.PartID,
			Name:        p.Name,
			Description: p.Description,
		}
		if p.SalesPrice > 0 {
			ip.Sales = &simplePrice{UnitPrice: p.SalesPrice}
		}
		if p.CostPrice > 0 {
			ip.Purchase = &simplePrice{UnitPrice: p.CostPrice}
		}

		// attempt to find existing item by Code so we can update instead of recreating
		if id, err := getItemIDByCode(ctx, httpClient, accessToken, tenantID, p.PartID); err == nil && id != "" {
			ip.ItemID = id
		}
		out = append(out, ip)
	}

	payload := map[string]interface{}{"Items": out}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.xero.com/api.xro/2.0/Items", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Xero-tenant-id", tenantID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return fmt.Errorf("xero items post failed: status=%d body=%s", resp.StatusCode, buf.String())
	}
	return nil
}

// GetInvoiceItemCodes looks up an invoice by InvoiceNumber and returns the ItemCode(s)
// for the invoice's line items (falls back to the line Description when ItemCode is empty).
func GetInvoiceItemCodes(ctx context.Context, httpClient *http.Client, accessToken, tenantID, invoiceNumber string) ([]string, error) {
	if invoiceNumber == "" {
		return nil, fmt.Errorf("invoice number empty")
	}

	// 1) find InvoiceID by InvoiceNumber (list endpoint)
	where := url.QueryEscape(fmt.Sprintf(`InvoiceNumber=="%s"`, invoiceNumber))
	u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Invoices?where=%s", where)
	log.Printf("[DEBUGHW] GetInvoiceItemCodes: lookup invoice=%q tenant=%q url=%s", invoiceNumber, tenantID, u)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		log.Printf("[DEBUGHW] new request error: %v", err)
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Xero-tenant-id", tenantID)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[DEBUGHW] http do error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DEBUGHW] lookup response status=%d body=%s", resp.StatusCode, string(body))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("invoices lookup failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var listShape struct {
		Invoices []struct {
			InvoiceID string `json:"InvoiceID"`
		} `json:"Invoices"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&listShape); err != nil {
		log.Printf("[DEBUGHW] json decode lookup error: %v", err)
		return nil, err
	}
	if len(listShape.Invoices) == 0 {
		log.Printf("[DEBUGHW] no invoice found for number=%q", invoiceNumber)
		return nil, nil
	}

	invoiceID := listShape.Invoices[0].InvoiceID
	if invoiceID == "" {
		log.Printf("[DEBUGHW] invoiceID empty for number=%q", invoiceNumber)
		return nil, nil
	}

	// 2) always fetch the invoice detail endpoint to get full LineItems
	detailURL := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Invoices/%s", invoiceID)
	log.Printf("[DEBUGHW] fetching invoice detail url=%s", detailURL)

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL, nil)
	if err != nil {
		log.Printf("[DEBUGHW] new detail request error: %v", err)
		return nil, err
	}
	req2.Header.Set("Authorization", "Bearer "+accessToken)
	req2.Header.Set("Xero-tenant-id", tenantID)
	req2.Header.Set("Accept", "application/json")

	resp2, err := httpClient.Do(req2)
	if err != nil {
		log.Printf("[DEBUGHW] detail http do error: %v", err)
		return nil, err
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	log.Printf("[DEBUGHW] detail response status=%d body=%s", resp2.StatusCode, string(body2))
	if resp2.StatusCode >= 300 {
		return nil, fmt.Errorf("invoice detail fetch failed: status=%d body=%s", resp2.StatusCode, string(body2))
	}

	var detailShape struct {
		Invoices []struct {
			LineItems []struct {
				ItemCode    string `json:"ItemCode"`
				Description string `json:"Description"`
			} `json:"LineItems"`
		} `json:"Invoices"`
	}
	if err := json.NewDecoder(bytes.NewReader(body2)).Decode(&detailShape); err != nil {
		log.Printf("[DEBUGHW] json decode detail error: %v", err)
		return nil, err
	}
	if len(detailShape.Invoices) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(detailShape.Invoices[0].LineItems))
	for i, li := range detailShape.Invoices[0].LineItems {
		log.Printf("[DEBUGHW] detail invoice line[%d] ItemCode=%q Description=%q", i, li.ItemCode, li.Description)
		if li.ItemCode != "" {
			out = append(out, li.ItemCode)
		} else if li.Description != "" {
			out = append(out, li.Description)
		}
	}
	log.Printf("[DEBUGHW] extracted %d items from detail for invoice %q", len(out), invoiceNumber)
	return out, nil
}
