package handler

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

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
	// add perform-login POST route
	r.Post("/perform-login", h.performLogin)
	// logout - prefer POST in production, GET works for quick testing
	r.Post("/logout", h.logoutHandler)

	// protected routes
	r.Group(func(r chi.Router) {
		// use RequireAuth which now redirects HTML requests to /login
		r.Use(h.RequireAuth)

		// make "/" protected so unauthenticated requests are redirected to /login
		r.Get("/", h.home)

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
			// inner RequireAuth is redundant but harmless; you can remove it if desired
			r.Use(h.RequireAuth)
			r.Get("/xero/connections", h.xeroConnections)
			r.Post("/xero/{tenant}/sync", h.xeroSync)
		})
	})

	return r
}

// RequireAuth validates token, stores claims and userID in context.
// If the request accepts HTML and is not authenticated, redirect to /login.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If Authorization header missing, try access_token cookie (browser session flow)
		if r.Header.Get("Authorization") == "" {
			if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
				// mask token length for logs
				tok := c.Value
				m := 8
				if len(tok) < m {
					m = len(tok)
				}
				log.Printf("RequireAuth: found access_token cookie (len=%d) prefix=%q", len(tok), tok[:m])
				r.Header.Set("Authorization", "Bearer "+c.Value)
			} else {
				log.Printf("RequireAuth: no access_token cookie")
			}
		} else {
			hdr := r.Header.Get("Authorization")
			log.Printf("RequireAuth: Authorization header present (len=%d)", len(hdr))
		}

		claims, ok := h.auth.Authenticate(r)
		if !ok {
			log.Printf("RequireAuth: Authenticate returned false")
			http.Redirect(w, r, "/login", http.StatusFound)
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

// dummy home page shown after successful login
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(ctxUserID).(string)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><html><head><meta charset="utf-8"><title>Home</title></head><body>
    <h1>Welcome</h1><p>User ID: `+uid+`</p>
    <form method="POST" action="/logout"><button type="submit">Logout</button></form>
    </body></html>`)
}
