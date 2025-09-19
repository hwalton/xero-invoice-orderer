package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	mid "github.com/hwalton/freeride-campervans/internal/middleware"
	"github.com/hwalton/freeride-campervans/internal/service"
	"github.com/hwalton/freeride-campervans/internal/utils"
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
	// ensure user is authenticated and we have ownerID
	ownerID, ok := mid.GetUserIDFromRequest(r, h.auth)
	if !ok || ownerID == "" {
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
	ownerID, _ := r.Context().Value(ctxUserID).(string)
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
	json.NewEncoder(w).Encode(conns)
}

// xeroSync triggers a sync for tenant (tenant path param).
func (h *Handler) xeroSyncHandler(w http.ResponseWriter, r *http.Request) {
	tenant := chi.URLParam(r, "tenant")
	if tenant == "" {
		http.Error(w, "tenant missing", http.StatusBadRequest)
		return
	}
	ownerID, _ := r.Context().Value(ctxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}

	// load connections and find matching tenant
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	conns, err := service.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var found *service.XeroConnection
	for _, c := range conns {
		if c.TenantID == tenant {
			found = &c
			break
		}
	}
	if found == nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	// ensure token not expired — refresh if needed
	now := time.Now().UTC()
	if found.ExpiresAt.Before(now.Add(1 * time.Minute)) {
		clientID := os.Getenv("XERO_CLIENT_ID")
		clientSecret := os.Getenv("XERO_CLIENT_SECRET")
		tr, err := xero.RefreshToken(ctx, h.client, clientID, clientSecret, found.RefreshToken)
		if err != nil {
			http.Error(w, "refresh token failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// persist refreshed tokens
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
			http.Error(w, "failed to persist refreshed token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// replace found access token for sync
		found.AccessToken = tr.AccessToken
		// recompute ExpiresAt for local copy
		secs := tr.ExpiresIn
		if secs == 0 {
			secs = 3600
		}
		found.ExpiresAt = time.Now().Add(time.Duration(secs) * time.Second)
	}

	// load parts from DB
	parts, err := service.LoadParts(ctx, h.dbURL)
	if err != nil {
		http.Error(w, "failed to load parts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// perform sync to Xero
	if err := xero.SyncPartsToXero(ctx, h.client, found.AccessToken, found.TenantID, parts); err != nil {
		http.Error(w, "sync to xero failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sync completed", "tenant": tenant})
}
