package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Part mirrors parts used for syncing (minimal).
type Part struct {
	PartID        string  `json:"part_id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	SalesPrice    float64 `json:"sales_price"`
	PurchasePrice float64 `json:"purchase_price"`
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

// SyncPartsToXero posts a minimal Items payload to Xero.
// Keeps payload minimal (Code, Name, Description, SalesDetails.UnitPrice) to avoid account/tax validation.
func SyncPartsToXero(ctx context.Context, httpClient *http.Client, accessToken, tenantID string, items []Part) error {
	if len(items) == 0 {
		return nil
	}
	type simplePrice struct {
		UnitPrice float64 `json:"UnitPrice"`
	}
	type itemPayload struct {
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
		if p.PurchasePrice > 0 {
			ip.Purchase = &simplePrice{UnitPrice: p.PurchasePrice}
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
