package xero

// ...existing code...
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Part mirrors your parts for Xero export (port from control-panel/internal/xero)
type Part struct {
	PartID        string  `json:"part_id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	SalesPrice    float64 `json:"sales_price"`
	PurchasePrice float64 `json:"purchase_price"`
	// ...other fields...
}

// BuildAuthURL returns the Xero OAuth authorize URL.
func BuildAuthURL(clientID, redirectURI, state string) string {
	scope := "offline_access accounting.contacts accounting.transactions accounting.settings"
	return fmt.Sprintf("https://login.xero.com/identity/connect/authorize?response_type=code&client_id=%s&redirect_uri=%s&scope=%s&state=%s", clientID, redirectURI, scope, state)
}

// ExchangeCodeForToken exchanges authorization code for tokens.
func ExchangeCodeForToken(ctx context.Context, httpClient *http.Client, clientID, clientSecret, code, redirectURI string) (accessToken, refreshToken string, expiresIn time.Duration, err error) {
	// implement token POST similar to your curl script in utils_dev/exchange-code-for-token.sh
	// ...existing code...
	return
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, httpClient *http.Client, clientID, clientSecret, refreshToken string) (newAccess, newRefresh string, expiresIn time.Duration, err error) {
	// implement token refresh POST to identity.xero.com/connect/token
	// ...existing code...
	return
}

// SyncPartsToXero posts items; ported from control-panel/internal/xero.SyncPartsToXero
func SyncPartsToXero(ctx context.Context, httpClient *http.Client, accessToken, tenantID string, items []Part) error {
	// copy logic from [control-panel/internal/xero/xero.go](control-panel/internal/xero/xero.go)
	return nil
}
