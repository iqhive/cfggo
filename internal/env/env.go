package env

import "strings"

// Loader derives environment variable names from configuration keys
type Loader struct{}

// NewLoader creates a new environment variable loader
func NewLoader() *Loader {
	return &Loader{}
}

// KeyToEnvVar converts a configuration key to an environment variable name:
// upper-cased, with "." and "-" mapped to "_", so every key yields a name a
// shell can export (db-host -> DB_HOST, db.url -> DB_URL). The mapping is
// lossy, so two keys may share a variable; Group reports such collisions
func (l *Loader) KeyToEnvVar(key string) string {
	return strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
}
