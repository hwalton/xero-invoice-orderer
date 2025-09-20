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
			// If no Authorization header, try cookie
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

// EnsureUserIDInContext ensures ctxUserID (and ctxClaims) are set on the request context when possible.
// Returns a request with an updated context (original request returned if nothing to add).
func EnsureUserIDInContext(r *http.Request, auth authpkg.Authenticator) *http.Request {
	// already present
	if v, ok := r.Context().Value(ctxUserID).(string); ok && v != "" {
		return r
	}

	// clone request for auth checks so we don't mutate headers on the original
	req := r
	if req.Header.Get("Authorization") == "" {
		if c, err := req.Cookie("access_token"); err == nil && c.Value != "" {
			req = req.Clone(req.Context())
			req.Header.Set("Authorization", "Bearer "+c.Value)
		}
	}

	if auth == nil {
		return r
	}

	claims, ok := auth.Authenticate(req)
	if !ok || claims == nil {
		return r
	}

	var uid string
	if v, ok := claims["sub"].(string); ok && v != "" {
		uid = v
	} else if v, ok := claims["user_id"].(string); ok && v != "" {
		uid = v
	}
	if uid == "" {
		return r
	}

	ctx := context.WithValue(r.Context(), ctxClaims, claims)
	ctx = context.WithValue(ctx, ctxUserID, uid)
	return r.WithContext(ctx)
}

// SetUserIDInContext stores the provided userID into the request context and returns a cloned request.
// Exported so handlers can set the value for the current request after e.g. login.
func SetUserIDInContext(r *http.Request, userID string) *http.Request {
	if userID == "" {
		return r
	}
	ctx := context.WithValue(r.Context(), ctxUserID, userID)
	return r.WithContext(ctx)
}

// GetUserID returns the user id stored in ctx, or empty string if not present.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxUserID).(string); ok {
		return v
	}
	return ""
}
