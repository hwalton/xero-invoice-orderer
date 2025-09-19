package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	mid "github.com/hwalton/freeride-campervans/internal/middleware"
	authpkg "github.com/hwalton/freeride-campervans/pkg/auth"
)

// Handler groups dependencies for route handlers.
type Handler struct {
	auth      authpkg.Authenticator
	client    *http.Client
	dbURL     string
	templates *template.Template // added: parsed templates

	// removed in-memory stateStore -> using DB-backed state with TTL
	_ sync.Mutex
}

// contextKey is a private type to avoid collisions when storing values in context.
type contextKey string

const (
	// ctxClaims stores JWT/identity claims (map[string]interface{})
	ctxClaims contextKey = "claims"
	// ctxUserID stores the authenticated user's id (string)
	ctxUserID contextKey = "userID"
)

// NewRouter now accepts dbURL so handlers can persist connections.
func NewRouter(a authpkg.Authenticator, c *http.Client, dbURL string, templates *template.Template) http.Handler {
	h := &Handler{
		auth:      a,
		client:    c,
		dbURL:     dbURL,
		templates: templates,
	}
	r := chi.NewRouter()

	r.Get("/health", h.health)

	// public login route
	r.Get("/login", h.loginHandler)
	r.Post("/perform-login", h.supabaseConnectHandler)
	r.Post("/logout", h.logoutHandler)

	// Protect routes with RequireAuth
	r.Group(func(r chi.Router) {
		r.Use(mid.RequireAuth(h.auth))
		r.Get("/", h.homeHandler) // <-- protected now
		r.Get("/xero/connect", h.xeroConnectHandler)
		r.Get("/xero/callback", h.xeroCallbackHandler)
		r.Get("/xero/connections", h.xeroConnectionsHandler)

		r.Post("/xero/sync", h.xeroSyncHandler)
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
