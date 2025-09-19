package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hwalton/freeride-campervans/internal/service"
	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/pkg/xero"
)

type contextKey string

const (
	ctxUserID contextKey = "userID"
	ctxClaims contextKey = "claims"
)

// xeroConnect redirects to Xero auth URL
func (h *Handler) xeroConnectHandler(w http.ResponseWriter, r *http.Request) {
	clientID := utils.GetEnv("XERO_CLIENT_ID", "")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/callback")
	state := "random" // better: per-user CSRF state stored server-side
	authURL := xero.BuildAuthURL(clientID, redirect, state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// xeroCallback exchanges code for tokens and persists connection(s)
func (h *Handler) xeroCallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code missing", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("XERO_CLIENT_ID")
	clientSecret := os.Getenv("XERO_CLIENT_SECRET")
	redirect := utils.GetEnv("REDIRECT", "https://localhost:8080/callback")

	tr, err := xero.ExchangeCodeForToken(ctx, h.client, clientID, clientSecret, code, redirect)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// discover tenant(s)
	conns, err := xero.GetConnections(ctx, h.client, tr.AccessToken)
	if err != nil {
		http.Error(w, "failed to get connections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// owner id from authenticated user
	ownerID, _ := r.Context().Value(ctxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing in context", http.StatusInternalServerError)
		return
	}

	// persist each tenant
	for _, c := range conns {
		// expires seconds fallback
		expires := tr.ExpiresIn
		if expires == 0 {
			expires = 3600
		}
		if err := service.UpsertConnection(ctx, h.dbURL, ownerID, c.TenantID, tr.AccessToken, tr.RefreshToken, expires); err != nil {
			http.Error(w, "persist connection failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// redirect back to app UI (or show JSON in dev)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "connected",
		"tenants":    conns,
		"expires_in": tr.ExpiresIn,
		"refresh":    "tokens persisted",
	})
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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
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

	// ensure token not expired — simple check; in production call RefreshToken if near expiry
	// Here, we assume stored ExpiresAt was set; try best-effort refresh if expired.
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

	// For demo: we don't load parts from DB here. In production, load parts and call xero.SyncPartsToXero.
	// Example minimal call showing success (no-op).
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sync invoked", "tenant": tenant})
}
