package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerFileLoadSaveAndEmptyFilename(t *testing.T) {
	handler := NewHandlerFile("", true)
	if !handler.IsDefault() {
		t.Fatal("IsDefault() = false, want true")
	}

	data, err := handler.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() with empty filename error = %v", err)
	}
	if data != nil {
		t.Fatalf("LoadConfig() with empty filename = %s, want nil", string(data))
	}
	if err := handler.SaveConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("SaveConfig() with empty filename: expected error, got nil")
	}

	filename := filepath.Join(t.TempDir(), "config.json")
	handler = NewHandlerFile(filename, false)
	if handler.IsDefault() {
		t.Fatal("IsDefault() = true, want false")
	}

	if err := handler.SaveConfig(json.RawMessage(`{"port":8080}`)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	got, err := handler.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if string(got) != `{"port":8080}` {
		t.Fatalf("LoadConfig() = %s, want saved JSON", string(got))
	}

	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("expected saved file to exist: %v", err)
	}

	if err := os.Chmod(filename, 0o600); err != nil {
		t.Fatalf("chmod saved file: %v", err)
	}
	if err := handler.SaveConfig(json.RawMessage(`{"port":9090}`)); err != nil {
		t.Fatalf("SaveConfig() preserving mode error = %v", err)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat saved file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("saved file mode = %v, want 0600", got)
	}
}

func TestHandlerEnvLoadConfigParsesPrefixedVariables(t *testing.T) {
	t.Setenv("CFGGO_SOURCE_APP_PORT", "8080")
	t.Setenv("CFGGO_SOURCE_FEATURE_ENABLED", "true")
	t.Setenv("CFGGO_SOURCE_NAME", "api")
	t.Setenv("CFGGO_SOURCE_JSON_OBJECT", `{"nested":1}`)
	t.Setenv("OTHER_APP_PORT", "9999")

	handler := NewHandlerEnv("CFGGO_SOURCE_", true)
	if !handler.IsDefault() {
		t.Fatal("IsDefault() = false, want true")
	}

	data, err := handler.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", string(data), err)
	}

	want := map[string]interface{}{
		"app.port":        float64(8080),
		"feature.enabled": true,
		"name":            "api",
		"json.object":     map[string]interface{}{"nested": float64(1)},
	}
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || !deepEqualJSONValue(gotValue, wantValue) {
			t.Fatalf("config[%q] = %#v, want %#v (all config: %#v)", key, gotValue, wantValue, got)
		}
	}
	if _, ok := got["other.app.port"]; ok {
		t.Fatalf("unexpected unprefixed key included: %#v", got)
	}

	if err := handler.SaveConfig(json.RawMessage(`{"ignored":true}`)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
}

func TestHandlerEnvPrefixWithoutSeparatorTrimsLeadingDot(t *testing.T) {
	t.Setenv("CFGGO_SOURCE_PORT", "8080")

	data, err := NewHandlerEnv("CFGGO_SOURCE", false).LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", string(data), err)
	}
	if _, ok := got["port"]; !ok {
		t.Fatalf("expected key %q after trimming prefix separator, got %#v", "port", got)
	}
}

func deepEqualJSONValue(got, want interface{}) bool {
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}
