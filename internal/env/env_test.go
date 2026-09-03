package env

import "testing"

func TestKeyToEnvVar(t *testing.T) {
	loader := NewLoader()
	tests := map[string]string{
		"server_port":  "SERVER_PORT",
		"db.url":       "DB_URL",
		"db-host":      "DB_HOST",
		"auth.db-host": "AUTH_DB_HOST",
		"Port":         "PORT",
	}
	for key, want := range tests {
		if got := loader.KeyToEnvVar(key); got != want {
			t.Errorf("KeyToEnvVar(%q) = %q, want %q", key, got, want)
		}
	}
}
