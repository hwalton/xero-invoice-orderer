package middleware

import (
	"context"
	"net/http"

	authpkg "github.com/hwalton/freeride-campervans/pkg/auth"
)

type contextKey string

const (
	ctxUserID contextKey = "userID"
	ctxClaims contextKey = "claims"
)

// RequireAuth returns middleware that validates JWT (via auth) and sets claims/userID in context.
func RequireAuth(auth authpkg.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
					r.Header.Set("Authorization", "Bearer "+c.Value)
				}
			}
			claims, ok := auth.Authenticate(r)
			if !ok {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
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
}

// RequireRole returns middleware that requires a role claim (exact match).
func RequireRole(role string) func(http.Handler) http.Handler {
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
