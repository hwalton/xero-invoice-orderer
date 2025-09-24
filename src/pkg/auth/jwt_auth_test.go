package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func signedToken(t *testing.T, method jwt.SigningMethod, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	s, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("signed token: %v", err)
	}
	return s
}

func TestAuthenticate_NoHeader(t *testing.T) {
	a := NewJWT("secret", "", "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, ok := a.Authenticate(req)
	if ok {
		t.Fatalf("expected not authenticated with no header")
	}
}

func TestAuthenticate_MalformedHeader(t *testing.T) {
	a := NewJWT("secret", "", "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer") // malformed
	_, ok := a.Authenticate(req)
	if ok {
		t.Fatalf("expected not authenticated for malformed header")
	}
}

func TestAuthenticate_WrongSigningMethod(t *testing.T) {
	secret := "s3cr3t"
	// create HS384 token while code expects HS256
	token := signedToken(t, jwt.SigningMethodHS384, secret, jwt.MapClaims{"sub": "1"})
	a := NewJWT(secret, "", "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	_, ok := a.Authenticate(req)
	if ok {
		t.Fatalf("expected not authenticated for wrong signing method")
	}
}

func TestAuthenticate_InvalidSignature(t *testing.T) {
	secret := "correct"
	bad := "wrong"
	token := signedToken(t, jwt.SigningMethodHS256, bad, jwt.MapClaims{"sub": "1"})
	a := NewJWT(secret, "", "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	_, ok := a.Authenticate(req)
	if ok {
		t.Fatalf("expected not authenticated for invalid signature")
	}
}

func TestAuthenticate_ValidToken_IssuerAudience(t *testing.T) {
	secret := "topsecret"

	claims := jwt.MapClaims{
		"sub": "user-1",
		"iss": "test-iss",
		"aud": "test-aud",
	}
	token := signedToken(t, jwt.SigningMethodHS256, secret, claims)

	// pass expected issuer/audience into constructor
	a := NewJWT(secret, "test-iss", "test-aud")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	out, ok := a.Authenticate(req)
	if !ok {
		t.Fatalf("expected authenticated for valid token")
	}
	if out["sub"] != "user-1" {
		t.Fatalf("unexpected sub claim: %v", out["sub"])
	}
}

func TestAuthenticate_AudienceArray(t *testing.T) {
	secret := "secret-array"

	// aud as array (use []interface{} so type matches how jwt package decodes)
	claims := jwt.MapClaims{
		"sub": "user-2",
		"aud": []interface{}{"other", "aud-target"},
	}
	token := signedToken(t, jwt.SigningMethodHS256, secret, claims)

	// specify expected audience only
	a := NewJWT(secret, "", "aud-target")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, ok := a.Authenticate(req)
	if !ok {
		t.Fatalf("expected authenticated when audience present in array")
	}
}
