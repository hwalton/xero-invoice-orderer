package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

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

func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}

// xeroConnect redirects to Xero auth URL
func (h *Handler) xeroConnectHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("[DEBUGhw] 1 xeroConnectHandler\n")

	// prefer ownerID from user_id cookie to avoid re-parsing JWT
	var ownerID string
	if c, err := r.Cookie("user_id"); err == nil && c.Value != "" {
		ownerID = c.Value
		log.Printf("[DEBUGhw] xeroConnect: owner from cookie=%s", ownerID)
	} else if h.auth != nil {
		// fallback to middleware helper if cookie missing
		if got, ok := mid.GetUserIDFromRequest(r, h.auth); ok && got != "" {
			ownerID = got
			log.Printf("[DEBUGhw] xeroConnect: owner recovered via auth=%s", ownerID)
		}
	}

	if ownerID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	log.Printf("[DEBUGhw] 2 xeroConnect: owner=%s", ownerID)

	clientID := utils.GetEnv("XERO_CLIENT_ID", "")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/xero/callback")

	log.Printf("[DEBUGhw] 3 xeroConnect: owner=%s client_id=%s redirect=%s", ownerID, short(clientID), redirect)

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

	log.Printf("[DEBUGhw] 4 xeroConnect: owner=%s state=%s ttl=%d", ownerID, short(state), ttl)

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

	log.Printf("[DEBUGhw] xeroCallback: state=%s owner=%s code_len=%d", short(state), ownerID, len(code))

	clientID := os.Getenv("XERO_CLIENT_ID")
	clientSecret := os.Getenv("XERO_CLIENT_SECRET")
	redirect := utils.GetEnv("REDIRECT", "http://localhost:8080/xero/callback")

	tr, err := xero.ExchangeCodeForToken(ctx, h.client, clientID, clientSecret, code, redirect)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUGhw] xeroCallback: token exchange succeeded access_len=%d refresh_len=%d", len(tr.AccessToken), len(tr.RefreshToken))

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

	log.Printf("[DEBUGhw] xeroCallback: persisted %d connections for owner=%s", len(conns), ownerID)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// xeroConnections lists stored Xero connections for the current user.
func (h *Handler) xeroConnectionsHandler(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := r.Context().Value(ctxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}
	log.Printf("[DEBUGhw] xeroConnections: owner=%s", ownerID)

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
	// prefer owner id from context/cookie/auth helper (same as before)
	ownerID, _ := r.Context().Value(ctxUserID).(string)
	if ownerID == "" {
		if c, err := r.Cookie("user_id"); err == nil && c.Value != "" {
			ownerID = c.Value
			log.Printf("[DEBUGhw] xeroSync: owner from cookie=%s", ownerID)
			r = r.WithContext(context.WithValue(r.Context(), ctxUserID, ownerID))
		} else if h.auth != nil {
			if got, ok := mid.GetUserIDFromRequest(r, h.auth); ok && got != "" {
				ownerID = got
				r = r.WithContext(context.WithValue(r.Context(), ctxUserID, ownerID))
				log.Printf("[DEBUGhw] xeroSync: owner recovered via auth=%s", ownerID)
			}
		}
	}

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
	tenant := found.TenantID
	log.Printf("[DEBUGhw] xeroSync: tenant selected from db=%s", short(tenant))
	log.Printf("[DEBUGhw] xeroSync: owner=%s tenant=%s", ownerID, tenant)

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
	log.Printf("[DEBUGhw] xeroSync: loaded %d parts to sync for tenant=%s", len(parts), tenant)

	if err := xero.SyncPartsToXero(ctx, h.client, found.AccessToken, found.TenantID, parts); err != nil {
		http.Error(w, "sync to xero failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUGhw] xeroSync: sync completed for tenant=%s", tenant)

	// set a short-lived cookie with the sync message (read by homeHandler)
	when := time.Now().UTC().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("[%s] Parts list synced to xero", when)
	utils.SetCookie(w, r, "xero_sync_msg", msg, time.Now().Add(5*time.Minute))

	// redirect back to home
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
