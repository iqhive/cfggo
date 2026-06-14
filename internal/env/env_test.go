package env

import (
	"reflect"
	"testing"
)

func TestLoaderKeyConversionsAndLookup(t *testing.T) {
	loader := NewLoader()

	if got, want := loader.KeyToEnvVar("database.host.name"), "DATABASE_HOST_NAME"; got != want {
		t.Fatalf("KeyToEnvVar() = %q, want %q", got, want)
	}
	if got, want := loader.EnvVarToKey("DATABASE_HOST_NAME"), "database.host.name"; got != want {
		t.Fatalf("EnvVarToKey() = %q, want %q", got, want)
	}

	if err := loader.SetEnvironmentValue("database.host", "localhost"); err != nil {
		t.Fatalf("SetEnvironmentValue() error = %v", err)
	}
	t.Cleanup(func() {
		if err := loader.UnsetEnvironmentValue("database.host"); err != nil {
			t.Fatalf("UnsetEnvironmentValue() cleanup error = %v", err)
		}
	})

	value, ok := loader.GetEnvironmentValue("database.host")
	if !ok || value != "localhost" {
		t.Fatalf("GetEnvironmentValue() = %q, %v; want localhost, true", value, ok)
	}

	loaded := loader.LoadEnvironmentVariables([]string{"database.host", "database.port"})
	if got, want := loaded["database.host"], "localhost"; got != want {
		t.Fatalf("LoadEnvironmentVariables()[database.host] = %q, want %q", got, want)
	}
	if _, ok := loaded["database.port"]; ok {
		t.Fatalf("LoadEnvironmentVariables() included unset key: %#v", loaded)
	}
}

func TestLoaderGetPrefixedEnvironmentVariables(t *testing.T) {
	loader := NewLoader()
	t.Setenv("CFGGO_INTERNAL_HOST", "localhost")
	t.Setenv("CFGGO_INTERNAL_PORT", "8080")
	t.Setenv("OTHER_HOST", "ignored")

	got := loader.GetPrefixedEnvironmentVariables("cfggo_internal_")
	want := map[string]string{
		"host": "localhost",
		"port": "8080",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetPrefixedEnvironmentVariables() = %#v, want %#v", got, want)
	}
}

func TestLoaderValidateEnvironmentType(t *testing.T) {
	loader := NewLoader()
	if err := loader.ValidateEnvironmentType("8080", reflect.TypeOf(0)); err != nil {
		t.Fatalf("ValidateEnvironmentType() error = %v", err)
	}
}
