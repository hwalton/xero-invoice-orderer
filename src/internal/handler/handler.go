package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	authpkg "github.com/hwalton/freeride-campervans/pkg/auth"
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
}

func NewRouter(a authpkg.Authenticator, c *http.Client) http.Handler {
	h := &Handler{auth: a, client: c}
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
