package auth

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
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
func NewJWT(secret string) Authenticator {
	return &jwtAuth{secret: []byte(secret)}
}

type jwtAuth struct {
	secret []byte
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

	// debug: small token info (do NOT log full token in production)
	prefix := tokenString
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}

	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		// Ensure expected signing method (Supabase typically uses HS256)
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.secret, nil
	})
	if err != nil {
		// more debug: try to show header alg (decode first segment)
		parts := strings.Split(tokenString, ".")
		if len(parts) > 0 {
			if h, derr := base64.RawURLEncoding.DecodeString(parts[0]); derr == nil {
				log.Printf("jwt auth: parse error: %v; token header: %s", err, string(h))
			} else {
				log.Printf("jwt auth: parse error: %v; failed to decode header: %v", err, derr)
			}
		} else {
			log.Printf("jwt auth: parse error: %v; token parts invalid", err)
		}
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

	out := make(map[string]interface{}, len(claims))
	for k, v := range claims {
		out[k] = v
	}
	return out, true
}
