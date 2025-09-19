package utils

import (
	"net/http"
	"os"
	"strings"
)

// helper to read env with default
func GetEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func IsSecureRequest(r *http.Request) bool {
	if dev := GetEnv("DEVELOPMENT_MODE", ""); dev != "" {
		if strings.EqualFold(dev, "1") || strings.EqualFold(dev, "true") || strings.EqualFold(dev, "yes") {
			return false
		}
	}
	return true
}
