package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "config.json")
	if err := os.WriteFile(real, []byte(`{"port":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	handler := NewHandlerFile(link, false)
	if err := handler.SaveConfig(json.RawMessage(`{"port":2}`)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("config path is no longer a symlink: Save replaced the link instead of writing through it")
	}
	got, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"port":2}` {
		t.Fatalf("symlink target contains %s, want the saved document", got)
	}
	loaded, err := handler.LoadConfig()
	if err != nil || string(loaded) != `{"port":2}` {
		t.Fatalf("LoadConfig through link = %s, %v", loaded, err)
	}
}

func TestSaveConfigThroughDanglingSymlinkCreatesTarget(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink("missing.json", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := NewHandlerFile(link, false).SaveConfig(json.RawMessage(`{"port":3}`)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("dangling symlink was replaced by a regular file")
	}
	target := filepath.Join(dir, "missing.json")
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected the link target to be created: %v", err)
	}
	if string(got) != `{"port":3}` {
		t.Fatalf("target contains %s, want the saved document", got)
	}
	targetInfo, _ := os.Stat(target)
	if perm := targetInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("new target mode = %v, want 0600", perm)
	}
}

func TestHandlerEnvPreservesLargeIntegerLiterals(t *testing.T) {
	t.Setenv("CFGGO_BIGINT_ID", "9007199254740993")
	data, err := NewHandlerEnv("CFGGO_BIGINT_", false).LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if got := string(doc["id"]); got != "9007199254740993" {
		t.Fatalf("id literal = %s, want it re-emitted exactly", got)
	}
}

func TestHandlerFileRejectsOversizedFiles(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(filename, []byte(`{"pad":"`+string(make([]byte, 64))+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandlerFile(filename, false)
	h.MaxBytes = 32
	if _, err := h.LoadConfig(); err == nil {
		t.Fatal("LoadConfig accepted a file larger than MaxBytes")
	}
	h.MaxBytes = 0
	if _, err := h.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig with the default cap: %v", err)
	}
	if DefaultMaxFileBytes != 10<<20 {
		t.Fatalf("DefaultMaxFileBytes = %d, want 10 MiB to match the HTTP and conf caps", DefaultMaxFileBytes)
	}
}
