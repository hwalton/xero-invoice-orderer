package handler

import (
	"context"
	"net/http"
	"strings"
)

// RequireAuth validates token, stores claims and userID in context.
// If the request accepts HTML and is not authenticated, redirect to /login.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := h.auth.Authenticate(r)
		if !ok {
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml") {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
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
