package utils

import (
	"os"
)

// helper to read env with default
func GetEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
