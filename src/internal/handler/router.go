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

		// r.Post("/xero/sync", h.xeroSyncHandler)
		r.Post("/xero/invoice", h.getInvoiceHandler)
		r.Post("/xero/create-pos", h.createPurchaseOrdersHandler)
		r.Post("/shopping-list/add", h.addShoppingListHandler) // add invoice lines to shopping_list

		// Development helpers
		r.Get("/contacts", h.dumpContactsHandler)
		r.Get("/items", h.dumpItemsHandler)
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
