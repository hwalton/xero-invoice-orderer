package utils

import (
	"net/http"
	"time"
)

// SetCookie sets a cookie with consistent defaults (HttpOnly, SameSite=Lax, Secure per IsSecureRequest).
func SetCookie(w http.ResponseWriter, r *http.Request, name, value string, expires time.Time) {
	secure := IsSecureRequest(r)
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}

// ClearCookie removes a cookie using the same security flags.
func ClearCookie(w http.ResponseWriter, r *http.Request, name string) {
	c := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}
