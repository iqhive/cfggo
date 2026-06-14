package env

import (
	"os"
	"reflect"
	"strings"
)

// Loader handles loading environment variables
type Loader struct{}

// NewLoader creates a new environment variable loader
func NewLoader() *Loader {
	return &Loader{}
}

// LoadEnvironmentVariables loads environment variables for the given keys
func (l *Loader) LoadEnvironmentVariables(keys []string) map[string]string {
	envVars := make(map[string]string)

	for _, key := range keys {
		envVar := l.KeyToEnvVar(key)
		if value, exists := os.LookupEnv(envVar); exists {
			envVars[key] = value
		}
	}

	return envVars
}

// KeyToEnvVar converts a configuration key to an environment variable name
func (l *Loader) KeyToEnvVar(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// EnvVarToKey converts an environment variable name to a configuration key
func (l *Loader) EnvVarToKey(envVar string) string {
	return strings.ToLower(strings.ReplaceAll(envVar, "_", "."))
}

// GetPrefixedEnvironmentVariables gets all environment variables with a specific prefix
func (l *Loader) GetPrefixedEnvironmentVariables(prefix string) map[string]string {
	envVars := make(map[string]string)
	upperPrefix := strings.ToUpper(prefix)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]
		if strings.HasPrefix(key, upperPrefix) {
			configKey := l.EnvVarToKey(strings.TrimPrefix(key, upperPrefix))
			envVars[configKey] = value
		}
	}

	return envVars
}

// ValidateEnvironmentType checks if an environment value can be converted to the target type
func (l *Loader) ValidateEnvironmentType(value string, targetType reflect.Type) error {
	// This would include validation logic similar to the converter
	// For now, we'll keep it simple and assume validation happens during conversion
	return nil
}

// GetEnvironmentValue safely gets an environment variable value
func (l *Loader) GetEnvironmentValue(key string) (string, bool) {
	envVar := l.KeyToEnvVar(key)
	return os.LookupEnv(envVar)
}

// SetEnvironmentValue sets an environment variable (mainly for testing)
func (l *Loader) SetEnvironmentValue(key, value string) error {
	envVar := l.KeyToEnvVar(key)
	return os.Setenv(envVar, value)
}

// UnsetEnvironmentValue unsets an environment variable (mainly for testing)
func (l *Loader) UnsetEnvironmentValue(key string) error {
	envVar := l.KeyToEnvVar(key)
	return os.Unsetenv(envVar)
}
