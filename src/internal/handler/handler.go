package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hwalton/freeride-campervans/internal/store"
	authpkg "github.com/hwalton/freeride-campervans/pkg/auth"
	"github.com/hwalton/freeride-campervans/pkg/xero"
)

type contextKey string

const (
	ctxUserID contextKey = "userID"
	ctxClaims contextKey = "claims"
)

// Handler groups dependencies for route handlers.
type Handler struct {
	auth   authpkg.Authenticator
	client *http.Client
	dbURL  string
}

// NewRouter now accepts dbURL so handlers can persist connections.
func NewRouter(a authpkg.Authenticator, c *http.Client, dbURL string) http.Handler {
	h := &Handler{auth: a, client: c, dbURL: dbURL}
	r := chi.NewRouter()

	r.Get("/health", h.health)

	// protected routes
	r.Group(func(r chi.Router) {
		r.Use(h.RequireAuth)
		r.Get("/protected", h.protected)
		// example: admin-only
		r.Route("/admin", func(rr chi.Router) {
			rr.Use(h.RequireRole("admin"))
			rr.Get("/", h.adminOnly)
		})
		// Xero connect + callback
		r.Get("/xero/connect", h.xeroConnect)
		r.Get("/xero/callback", h.xeroCallback)
		// list connections + trigger sync (protected)
		r.Group(func(r chi.Router) {
			r.Use(h.RequireAuth)
			r.Get("/xero/connections", h.xeroConnections)
			r.Post("/xero/{tenant}/sync", h.xeroSync)
		})
	})

	return r
}

// RequireAuth validates token, stores claims and userID in context.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := h.auth.Authenticate(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// common claim keys: "sub" or "user_id"
		var uid string
		if v, ok := claims["sub"].(string); ok && v != "" {
			uid = v
		} else if v, ok := claims["user_id"].(string); ok && v != "" {
			uid = v
		}
		ctx := context.WithValue(r.Context(), ctxClaims, claims)
		ctx = context.WithValue(ctx, ctxUserID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole returns middleware that requires a role claim (string match).
func (h *Handler) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, _ := r.Context().Value(ctxClaims).(map[string]interface{})
			if claims == nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if rRole, ok := claims["role"].(string); !ok || rRole != role {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) protected(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(ctxUserID).(string)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "protected", "user": uid})
}

func (h *Handler) adminOnly(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "admin area"})
}

// helper to read env with default
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// xeroConnect redirects to Xero auth URL
func (h *Handler) xeroConnect(w http.ResponseWriter, r *http.Request) {
	clientID := getEnv("XERO_CLIENT_ID", "")
	redirect := getEnv("REDIRECT", "https://localhost:8080/callback")
	state := "random" // better: per-user CSRF state stored server-side
	authURL := xero.BuildAuthURL(clientID, redirect, state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// xeroCallback exchanges code for tokens and persists connection(s)
func (h *Handler) xeroCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code missing", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("XERO_CLIENT_ID")
	clientSecret := os.Getenv("XERO_CLIENT_SECRET")
	redirect := getEnv("REDIRECT", "https://localhost:8080/callback")

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
		if err := store.UpsertConnection(ctx, h.dbURL, ownerID, c.TenantID, tr.AccessToken, tr.RefreshToken, expires); err != nil {
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
func (h *Handler) xeroConnections(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := r.Context().Value(ctxUserID).(string)
	if ownerID == "" {
		http.Error(w, "owner id missing", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	conns, err := store.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conns)
}

// xeroSync triggers a sync for tenant (tenant path param).
func (h *Handler) xeroSync(w http.ResponseWriter, r *http.Request) {
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
	conns, err := store.GetConnectionsForOwner(ctx, h.dbURL, ownerID)
	if err != nil {
		http.Error(w, "failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var found *store.XeroConnection
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
		if err := store.UpsertConnection(ctx, h.dbURL, ownerID, found.TenantID, tr.AccessToken, tr.RefreshToken, tr.ExpiresIn); err != nil {
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
