package auth

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Authenticator is the minimal interface your handlers depend on.
type Authenticator interface {
	// Authenticate returns the token claims and true when the request is authenticated.
	// Claims is a plain map so handlers don't need jwt dependency.
	Authenticate(r *http.Request) (claims map[string]interface{}, ok bool)
}

// NewJWT returns an Authenticator that validates HMAC-signed JWTs using the provided secret.
// It will also validate issuer/audience if SUPABASE_JWT_ISSUER / SUPABASE_JWT_AUDIENCE env vars are set.
func NewJWT(secret string) Authenticator {
	return &jwtAuth{
		secret:   []byte(secret),
		issuer:   os.Getenv("SUPABASE_JWT_ISSUER"),
		audience: os.Getenv("SUPABASE_JWT_AUDIENCE"),
	}
}

type jwtAuth struct {
	secret   []byte
	issuer   string // optional expected iss
	audience string // optional expected aud
}

func (a *jwtAuth) Authenticate(r *http.Request) (map[string]interface{}, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, false
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, false
	}
	tokenString := parts[1]

	// debug: ensure secret present
	if len(a.secret) == 0 {
		log.Printf("jwt auth: no secret configured (SUPABASE_JWT_SECRET missing?)")
	}

	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Ensure expected signing method (Supabase typically uses HS256)
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.secret, nil
	})
	if err != nil {
		// parse error - do not log token contents
		log.Printf("jwt auth: parse error: %v", err)
		return nil, false
	}
	if !token.Valid {
		log.Printf("jwt auth: token invalid")
		return nil, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, false
	}

	// Optional: validate issuer
	if a.issuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != a.issuer {
			log.Printf("jwt auth: issuer mismatch; expected=%s got=%v", a.issuer, claims["iss"])
			return nil, false
		}
	}

	// Optional: validate audience (aud can be string or array)
	if a.audience != "" {
		if audVal, ok := claims["aud"]; ok {
			switch v := audVal.(type) {
			case string:
				if v != a.audience {
					log.Printf("jwt auth: audience mismatch; expected=%s got=%s", a.audience, v)
					return nil, false
				}
			case []interface{}:
				found := false
				for _, it := range v {
					if s, ok := it.(string); ok && s == a.audience {
						found = true
						break
					}
				}
				if !found {
					log.Printf("jwt auth: audience not found; expected=%s", a.audience)
					return nil, false
				}
			default:
				log.Printf("jwt auth: unexpected aud claim type: %T", audVal)
				return nil, false
			}
		} else {
			log.Printf("jwt auth: aud claim missing but expected=%s", a.audience)
			return nil, false
		}
	}

	out := make(map[string]interface{}, len(claims))
	for k, v := range claims {
		out[k] = v
	}
	return out, true
}
