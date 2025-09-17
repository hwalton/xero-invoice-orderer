package auth

import (
	"errors"
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

// NewJWT returns an Authenticator that validates HMAC-signed JWTs.
// Keep this package independent of your internal packages so it can be moved later.
func NewJWT(secret string) Authenticator {
	return &jwtAuth{secret: []byte(secret)}
}

type jwtAuth struct {
	secret []byte
}

func (j *jwtAuth) Authenticate(r *http.Request) (map[string]interface{}, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, false
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, false
	}
	tokenStr := parts[1]

	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		// Only accept HMAC here; adjust if you use another alg
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, false
	}

	if claims, ok := tok.Claims.(jwt.MapClaims); ok {
		// convert to plain map[string]interface{}
		out := make(map[string]interface{}, len(claims))
		for k, v := range claims {
			out[k] = v
		}
		return out, true
	}
	return nil, false
}
