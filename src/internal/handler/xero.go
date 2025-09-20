package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	mid "github.com/hwalton/freeride-campervans/internal/middleware"
	"github.com/hwalton/freeride-campervans/internal/service"
	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/internal/web"
	"github.com/hwalton/freeride-campervans/pkg/xero"
)

// generateState returns a secure random hex string of length 2*n (n bytes).
func generateState(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// xeroConnect redirects to Xero auth URL
func (h *Handler) xeroConnectHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	clientID := utils.GetEnv("XERO_CLIENT_ID", "")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/xero/callback")

	// generate secure state and persist mapping -> ownerID (use DB-backed store with TTL)
	state, err := generateState(16)
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	ttl := 300 // seconds (5 minutes) — adjust as needed
	if err := service.CreateOAuthState(r.Context(), h.dbURL, state, ownerID, ttl); err != nil {
		http.Error(w, "failed to persist state", http.StatusInternalServerError)
		return
	}

	authURL := xero.BuildAuthURL(clientID, redirect, state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// xeroCallback exchanges code for tokens and persists connection(s)
func (h *Handler) xeroCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		http.Error(w, "code missing", http.StatusBadRequest)
		return
	}
	if state == "" {
		http.Error(w, "state missing", http.StatusBadRequest)
		return
	}

	// lookup ownerID by state (one-time use) via DB
	ownerID, found, err := service.ConsumeOAuthState(ctx, h.dbURL, state)
	if err != nil {
		http.Error(w, "state lookup failed", http.StatusInternalServerError)
		return
	}
	if !found || ownerID == "" {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("XERO_CLIENT_ID")
	clientSecret := os.Getenv("XERO_CLIENT_SECRET")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/xero/callback")

	tr, err := xero.ExchangeCodeForToken(ctx, h.client, clientID, clientSecret, code, redirect)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	conns, err := xero.GetConnections(ctx, h.client, tr.AccessToken)
	if err != nil {
		http.Error(w, "failed to get connections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// persist connections for the ownerID
	for _, c := range conns {
		expires := tr.ExpiresIn
		if expires == 0 {
			expires = 3600
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, c.TenantID, tr.AccessToken, tr.RefreshToken, expires); err != nil {
			http.Error(w, "persist connection failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// xeroConnections lists stored Xero connections for the current user.
func (h *Handler) xeroConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conns)
}

// xeroSync triggers a sync for tenant (tenant path param).
func (h *Handler) xeroSyncHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}

	// Load connections for the owner from DB and determine tenant from DB.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(conns) == 0 {
		http.Error(w, "no xero connection found for owner", http.StatusNotFound)
		return
	}

	// Always use the first DB row (owner_id is unique -> single tenant)
	found := &conns[0]

	// token refresh, load parts, sync to Xero (unchanged)
	now := time.Now().UTC()
	if found.ExpiresAt.Before(now.Add(1 * time.Minute)) {
		clientID := os.Getenv("XERO_CLIENT_ID")
		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
		tr, err := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
		if err != nil {
			http.Error(w, "refresh token failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
			http.Error(w, "failed to persist refreshed token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		found.AccessToken = tr.AccessToken
		secs := tr.ExpiresIn
		if secs == 0 {
			secs = 3600
		}
		found.ExpiresAt = time.Now().Add(time.Duration(secs) * time.Second)
	}

	parts, err := service.LoadParts(ctx, h.dbURL)
	if err != nil {
		http.Error(w, "failed to load parts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := xero.SyncPartsToXero(ctx, h.client, found.AccessToken, found.TenantID, parts); err != nil {
		http.Error(w, "sync to xero failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// set a short-lived cookie with the sync message (read by homeHandler)
	when := time.Now().UTC().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("Parts list synced to xero (%s)", when)
	utils.SetCookie(w, r, "xero_sync_msg", msg, time.Now().Add(5*time.Minute))

	// redirect back to home
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// getInvoiceHandler POSTs a form with invoice_id, queries Xero for that invoice's line item codes,
// and renders the home page with the returned items listed.
func (h *Handler) getInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil {
		r = mid.EnsureUserIDInContext(r, h.auth)
	}
	ownerID, _ := r.Context().Value(mid.CtxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	invoiceID := strings.TrimSpace(r.FormValue("invoice_id"))
	if invoiceID == "" {
		http.Error(w, "invoice id required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(conns) == 0 {
		http.Error(w, "no xero connection found for owner", http.StatusNotFound)
		return
	}
	found := &conns[0]

	// refresh token if near expiry
	now := time.Now().UTC()
	if found.ExpiresAt.Before(now.Add(1 * time.Minute)) {
		clientID := os.Getenv("XERO_CLIENT_ID")
		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
		tr, err := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
		if err != nil {
			http.Error(w, "refresh token failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
			http.Error(w, "failed to persist refreshed token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		found.AccessToken = tr.AccessToken
		secs := tr.ExpiresIn
		if secs == 0 {
			secs = 3600
		}
		found.ExpiresAt = time.Now().Add(time.Duration(secs) * time.Second)
	}

	// fetch invoice lines
	itemLines, err := xero.GetInvoiceItemCodes(ctx, h.client, found.AccessToken, found.TenantID, invoiceID)
	if err != nil {
		http.Error(w, "failed to fetch invoice: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build roots for BOM resolution
	roots := make([]service.RootItem, 0, len(itemLines))
	for _, ln := range itemLines {
		if ln.ItemCode == "" || ln.Quantity <= 0 {
			continue
		}
		roots = append(roots, service.RootItem{
			PartID:   ln.ItemCode,
			Name:     ln.Name,
			Quantity: ln.Quantity,
		})
	}
	if len(roots) == 0 {
		utils.SetCookie(w, r, "xero_sync_msg", "No valid invoice items found.", time.Now().Add(5*time.Minute))
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Resolve BOM (maxDepth 12)
	bom, errMsg, err := service.ResolveInvoiceBOM(ctx, h.dbURL, roots, 12)
	if err != nil {
		http.Error(w, "BOM resolution failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if errMsg != "" {
		utils.SetCookie(w, r, "xero_sync_msg", errMsg, time.Now().Add(5*time.Minute))
		// ensure previous invoice cookies are cleared
		utils.ClearCookie(w, r, "xero_invoice_bom")
		utils.ClearCookie(w, r, "xero_invoice_number")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// success: store BOM in cookie and redirect home
	b, _ := json.Marshal(bom)
	enc := base64.StdEncoding.EncodeToString(b)
	utils.SetCookie(w, r, "xero_invoice_bom", enc, time.Now().Add(5*time.Minute))
	utils.SetCookie(w, r, "xero_invoice_number", invoiceID, time.Now().Add(5*time.Minute))

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return
	// Render home template with invoice items included (reusing homeHandler rendering logic).
	// Load connections for template as in homeHandler
	var connsForUI []service.XeroConnection
	if c, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID); err == nil {
		connsForUI = c
	}

	var hasXeroConn bool
	var tenantID string
	var createdAt interface{}
	if len(connsForUI) > 0 {
		hasXeroConn = true
		tenantID = connsForUI[0].TenantID
		createdAt = connsForUI[0].CreatedAt
	}

	// Build sync message from short-lived cookie (set after successful sync)
	var xeroSyncMsg string
	if c, err := r.Cookie("xero_sync_msg"); err == nil && c.Value != "" {
		xeroSyncMsg = c.Value
		utils.ClearCookie(w, r, "xero_sync_msg")
	}

	data := map[string]interface{}{
		"Title":             "Home",
		"UserID":            ownerID,
		"HasXeroConnection": hasXeroConn,
		"XeroTenantID":      tenantID,
		"XeroCreatedAt":     createdAt,
		"XeroSyncMessage":   xeroSyncMsg,
		"InvoiceItems":      itemLines,
		"InvoiceNumber":     invoiceID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "home.html", data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
		return
	}
	if b, err := web.TemplatesFS.ReadFile("templates/home.html"); err == nil {
		t, err := template.New("home").Parse(string(b))
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		if err := t.Execute(w, data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		return
	}
	http.Error(w, "template error", http.StatusInternalServerError)
}
