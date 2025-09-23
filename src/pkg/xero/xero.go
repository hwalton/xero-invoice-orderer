package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Supplier represents a supplier row used when syncing contacts to Xero.
type Supplier struct {
	SupplierID   string `json:"supplier_id"`
	SupplierName string `json:"supplier_name"`
	ContactEmail string `json:"contact_email"`
	Phone        string `json:"phone"`
}

// newJSONRequest builds an HTTP request with standard Xero headers.
func newJSONRequest(ctx context.Context, method, u string, body []byte, accessToken, tenantID string) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if tenantID != "" {
		req.Header.Set("Xero-tenant-id", tenantID)
	}
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// doJSON executes a request and returns status + raw body for assertions.
func doJSON(client *http.Client, req *http.Request) (int, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

// buildPOPayload constructs a minimal PO payload.
func buildPOPayload(contactID string, items []POItem) ([]byte, error) {
	if contactID == "" {
		return nil, fmt.Errorf("contact id missing")
	}
	payload := map[string]interface{}{
		"PurchaseOrders": []map[string]interface{}{
			{
				"Contact":   map[string]string{"ContactID": contactID},
				"LineItems": items,
				"Status":    "AUTHORISED",
			},
		},
	}
	return json.Marshal(payload)
}

// buildItemsUpsertPayload builds the Items upsert payload.
// codeToItemID can be nil; if provided it maps Code -> existing ItemID.
func buildItemsUpsertPayload(items []Part, codeToItemID func(code string) (string, error)) ([]byte, error) {
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
		if codeToItemID != nil {
			if id, err := codeToItemID(p.PartID); err == nil && id != "" {
				ip.ItemID = id
			}
		}
		out = append(out, ip)
	}
	return json.Marshal(map[string]any{"Items": out})
}

// parse helpers

func parseConnectionsJSON(b []byte) ([]Connection, error) {
	var conns []Connection
	if err := json.Unmarshal(b, &conns); err != nil {
		return nil, err
	}
	return conns, nil
}

func parseFirstItemName(b []byte) (string, bool, error) {
	var res struct {
		Items []struct {
			Name string `json:"Name"`
		} `json:"Items"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return "", false, err
	}
	if len(res.Items) == 0 {
		return "", false, nil
	}
	return res.Items[0].Name, true, nil
}

func parseFirstItemID(b []byte) (string, error) {
	var res struct {
		Items []struct {
			ItemID string `json:"ItemID"`
		} `json:"Items"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return "", err
	}
	if len(res.Items) == 0 {
		return "", nil
	}
	return res.Items[0].ItemID, nil
}

func parseInvoiceID(b []byte) (string, error) {
	var listShape struct {
		Invoices []struct {
			InvoiceID string `json:"InvoiceID"`
		} `json:"Invoices"`
	}
	if err := json.Unmarshal(b, &listShape); err != nil {
		return "", err
	}
	if len(listShape.Invoices) == 0 {
		return "", nil
	}
	return listShape.Invoices[0].InvoiceID, nil
}

func parseInvoiceLines(b []byte) ([]InvoiceLine, error) {
	var detail struct {
		Invoices []struct {
			LineItems []struct {
				ItemCode    string  `json:"ItemCode"`
				Description string  `json:"Description"`
				Quantity    float64 `json:"Quantity"`
				Item        struct {
					Name string `json:"Name"`
				} `json:"Item"`
			} `json:"LineItems"`
		} `json:"Invoices"`
	}
	if err := json.Unmarshal(b, &detail); err != nil {
		return nil, err
	}
	if len(detail.Invoices) == 0 {
		return nil, nil
	}
	out := make([]InvoiceLine, 0, len(detail.Invoices[0].LineItems))
	for _, li := range detail.Invoices[0].LineItems {
		name := li.Item.Name
		if name == "" {
			name = li.Description
		}
		out = append(out, InvoiceLine{ItemCode: li.ItemCode, Name: name, Quantity: li.Quantity})
	}
	return out, nil
}

func parseFirstContactID(b []byte) (string, error) {
	var out struct {
		Contacts []struct {
			ContactID string `json:"ContactID"`
		} `json:"Contacts"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Contacts) == 0 {
		return "", nil
	}
	return out.Contacts[0].ContactID, nil
}

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

	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("token exchange failed: status=%d body=%s", status, string(body))
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
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

	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("refresh failed: status=%d body=%s", status, string(body))
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

// GetConnections calls GET https://api.xero.com/connections and returns parsed connections.
func GetConnections(ctx context.Context, httpClient *http.Client, accessToken string) ([]Connection, error) {
	req, err := newJSONRequest(ctx, http.MethodGet, "https://api.xero.com/connections", nil, accessToken, "")
	if err != nil {
		return nil, err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("/connections failed: status=%d body=%s", status, string(body))
	}
	return parseConnectionsJSON(body)
}

// GetItemNameByID returns item Name for a given Xero ItemID.
// found=false if not found.
func GetItemNameByID(ctx context.Context, httpClient *http.Client, accessToken, tenantID, itemID string) (name string, found bool, err error) {
	if itemID == "" {
		return "", false, nil
	}
	u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Items/%s", url.PathEscape(itemID))
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil, accessToken, tenantID)
	if err != nil {
		return "", false, err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return "", false, err
	}
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if status >= 300 {
		return "", false, fmt.Errorf("get item by id failed: status=%d body=%s", status, string(body))
	}
	return parseFirstItemName(body)
}

// GetItemNameByCode returns item Name for a given Xero Item Code.
// found=false if not found.
func GetItemNameByCode(ctx context.Context, httpClient *http.Client, accessToken, tenantID, code string) (string, bool, error) {
	if code == "" {
		return "", false, nil
	}
	where := url.QueryEscape(fmt.Sprintf(`Code=="%s"`, code))
	u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Items?where=%s", where)
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil, accessToken, tenantID)
	if err != nil {
		return "", false, err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return "", false, err
	}
	if status >= 300 {
		return "", false, fmt.Errorf("get item by code failed: status=%d body=%s", status, string(body))
	}
	return parseFirstItemName(body)
}

// SyncPartsToXero posts a minimal Items payload to Xero.
// Keeps payload minimal (Code, Name, Description, SalesDetails.UnitPrice) to avoid account/tax validation.
// Uses upsert behavior: if an item with the same Code exists it will be updated, otherwise created.
// Does not delete or touch items not present in the provided slice.
func SyncPartsToXero(ctx context.Context, httpClient *http.Client, accessToken, tenantID string, items []Part) error {
	if len(items) == 0 {
		return nil
	}
	codeToID := func(code string) (string, error) {
		return GetItemIDByCode(ctx, httpClient, accessToken, tenantID, code)
	}
	b, err := buildItemsUpsertPayload(items, codeToID)
	if err != nil {
		return err
	}
	req, err := newJSONRequest(ctx, http.MethodPost, "https://api.xero.com/api.xro/2.0/Items", b, accessToken, tenantID)
	if err != nil {
		return err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("xero items post failed: status=%d body=%s", status, string(body))
	}
	return nil
}

// InvoiceLine represents a single invoice line item (code + name + quantity).
type InvoiceLine struct {
	ItemCode string  `json:"item_code"`
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
}

// GetInvoiceItemCodes looks up an invoice by InvoiceNumber and returns the ItemCode(s)
// and Names/Quantities for the invoice's line items.
func GetInvoiceItemCodes(ctx context.Context, httpClient *http.Client, accessToken, tenantID, invoiceNumber string) ([]InvoiceLine, error) {
	if invoiceNumber == "" {
		return nil, fmt.Errorf("invoice number empty")
	}
	// find InvoiceID by InvoiceNumber
	where := url.QueryEscape(fmt.Sprintf(`InvoiceNumber=="%s"`, invoiceNumber))
	listURL := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Invoices?where=%s", where)
	req, err := newJSONRequest(ctx, http.MethodGet, listURL, nil, accessToken, tenantID)
	if err != nil {
		return nil, err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("invoices lookup failed: status=%d body=%s", status, string(body))
	}
	invoiceID, err := parseInvoiceID(body)
	if err != nil || invoiceID == "" {
		return nil, nil
	}

	// fetch invoice detail
	detailURL := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Invoices/%s", invoiceID)
	req2, err := newJSONRequest(ctx, http.MethodGet, detailURL, nil, accessToken, tenantID)
	if err != nil {
		return nil, err
	}
	status, body, err = doJSON(httpClient, req2)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("invoice detail fetch failed: status=%d body=%s", status, string(body))
	}
	return parseInvoiceLines(body)
}

// POItem is a minimal purchase order line (ItemCode + Quantity).
type POItem struct {
	ItemCode    string `json:"ItemCode"`
	Quantity    int    `json:"Quantity"`
	Description string `json:"Description,omitempty"`
}

// GetContactIDByAccountNumber looks up a Xero ContactID by AccountNumber.
// Returns empty string if not found.
func GetContactIDByAccountNumber(ctx context.Context, httpClient *http.Client, accessToken, tenantID, accountNumber string) (string, error) {
	if accountNumber == "" {
		return "", nil
	}
	where := url.QueryEscape(fmt.Sprintf(`AccountNumber=="%s"`, accountNumber))
	u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Contacts?where=%s", where)
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil, accessToken, tenantID)
	if err != nil {
		return "", err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("contacts lookup failed: status=%d body=%s", status, string(body))
	}
	return parseFirstContactID(body)
}

// CreatePurchaseOrder posts a minimal PurchaseOrder payload to Xero using ContactID.
// contactID must be the Xero Contacts.ContactID GUID.
func CreatePurchaseOrder(ctx context.Context, httpClient *http.Client, accessToken, tenantID, contactID string, items []POItem) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items")
	}
	b, err := buildPOPayload(contactID, items)
	if err != nil {
		return "", err
	}
	req, err := newJSONRequest(ctx, http.MethodPost, "https://api.xero.com/api.xro/2.0/PurchaseOrders", b, accessToken, tenantID)
	if err != nil {
		return "", err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("create purchase order failed: status=%d body=%s", status, string(body))
	}
	var res struct {
		PurchaseOrders []struct {
			PurchaseOrderID string `json:"PurchaseOrderID"`
		} `json:"PurchaseOrders"`
	}
	if err := json.Unmarshal(body, &res); err == nil && len(res.PurchaseOrders) > 0 {
		return res.PurchaseOrders[0].PurchaseOrderID, nil
	}
	return "", nil
}

// helper to find an existing item by Code. returns ItemID if found, empty string if not.
// GetItemIDByCode returns Xero ItemID for a given item Code (empty if not found).
func GetItemIDByCode(ctx context.Context, httpClient *http.Client, accessToken, tenantID, code string) (string, error) {
	where := url.QueryEscape(fmt.Sprintf(`Code=="%s"`, code))
	u := fmt.Sprintf("https://api.xero.com/api.xro/2.0/Items?where=%s", where)
	req, err := newJSONRequest(ctx, http.MethodGet, u, nil, accessToken, tenantID)
	if err != nil {
		return "", err
	}
	status, body, err := doJSON(httpClient, req)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("get item by code failed: status=%d body=%s", status, string(body))
	}
	return parseFirstItemID(body)
}
