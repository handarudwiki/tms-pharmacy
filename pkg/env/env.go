// Package env reads service configuration from environment variables.
package env

import "os"

// Get returns the value of key, or def when it is unset or empty.
func Get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// DatabaseURL is the shared PostgreSQL connection string. Each service owns its own schema.
func DatabaseURL() string {
	return Get("DATABASE_URL", "postgres://localhost:5432/pharmacy_tms?sslmode=disable")
}

// InternalToken is the shared secret services use to authenticate each other.
func InternalToken() string {
	return Get("TMS_INTERNAL_TOKEN", "dev-tms-internal-token")
}
