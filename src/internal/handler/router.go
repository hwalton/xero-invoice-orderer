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

	// in-memory OAuth state store: state -> ownerID
	stateMu    sync.Mutex
	stateStore map[string]string
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
		auth:       a,
		client:     c,
		dbURL:      dbURL,
		templates:  templates,
		stateStore: make(map[string]string),
	}
	r := chi.NewRouter()

	r.Get("/health", h.health)

	// public login route
	r.Get("/login", h.loginHandler)
	// add perform-login POST route
	r.Post("/perform-login", h.supabaseConnectHandler)
	// logout - prefer POST in production, GET works for quick testing
	r.Post("/logout", h.logoutHandler)

	// protected routes (require authentication)
	r.Group(func(r chi.Router) {
		r.Use(mid.RequireAuth(h.auth))
		r.Get("/", h.homeHandler)

		// Xero connect + callback
		r.Get("/xero/connect", h.xeroConnectHandler)
		r.Get("/xero/callback", h.xeroCallbackHandler)

		// list connections + trigger sync (protected)
		r.Get("/xero/connections", h.xeroConnectionsHandler)
		r.Post("/xero/{tenant}/sync", h.xeroSyncHandler)
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
