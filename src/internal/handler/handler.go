package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hwalton/freeride-campervans/internal/utils"
	"github.com/hwalton/freeride-campervans/internal/web"
	authpkg "github.com/hwalton/freeride-campervans/pkg/auth"
)

type contextKey string

const (
	ctxUserID contextKey = "userID"
	ctxClaims contextKey = "claims"
)

// Handler groups dependencies for route handlers.
type Handler struct {
	auth      authpkg.Authenticator
	client    *http.Client
	dbURL     string
	templates *template.Template // added: parsed templates
}

// NewRouter now accepts dbURL so handlers can persist connections.
func NewRouter(a authpkg.Authenticator, c *http.Client, dbURL string, templates *template.Template) http.Handler {
	h := &Handler{auth: a, client: c, dbURL: dbURL, templates: templates}
	r := chi.NewRouter()

	r.Get("/health", h.health)

	// public login route
	r.Get("/login", h.login)
	// supabase redirect for OAuth
	r.Get("/auth/supabase", h.authSupabase)

	// protected routes
	r.Group(func(r chi.Router) {
		// use RequireAuth which now redirects HTML requests to /login
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

// login serves the login page via templates
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// render using parsed templates; pass any dynamic data here
	data := map[string]interface{}{
		"Title": "Login — Freeride",
	}
	if h.templates != nil {
		_ = h.templates.ExecuteTemplate(w, "login.html", data)
		return
	}
	// fallback: embedded raw file (if templates not provided)
	if b, err := web.TemplatesFS.ReadFile("templates/login.html"); err == nil {
		_, _ = io.WriteString(w, string(b))
		return
	}
	http.Error(w, "template error", http.StatusInternalServerError)
}
