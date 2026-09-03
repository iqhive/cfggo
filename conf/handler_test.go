package conf

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileHandlerLoadAndReadOnlySave(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.conf")
	if err := os.WriteFile(filename, []byte("port=8080\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	handler, err := NewFileHandler(filename, false)
	if err != nil {
		t.Fatal(err)
	}
	if handler.IsDefault() {
		t.Fatal("required handler reports default")
	}
	loaded, err := handler.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if string(loaded) != `{"port":8080}` {
		t.Fatalf("loaded = %s", loaded)
	}
	if err := handler.SaveConfig(json.RawMessage(`{"port":9090}`)); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("SaveConfig error = %v, want ErrReadOnly", err)
	}
}

func TestFileHandlerRewritePreservesMode(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "config.conf")
	if err := os.WriteFile(filename, []byte("old=value\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	handler, err := NewFileHandler(filename, true, RewriteOnSave())
	if err != nil {
		t.Fatal(err)
	}
	if !handler.IsDefault() {
		t.Fatal("default handler reports required")
	}
	if err := handler.SaveConfig(json.RawMessage(`{"port":9090}`)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "port=9090\n" {
		t.Fatalf("saved = %q", data)
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}

	newFile := filepath.Join(dir, "new.conf")
	newHandler, err := NewFileHandler(newFile, false, RewriteOnSave())
	if err != nil {
		t.Fatal(err)
	}
	if err := newHandler.SaveConfig(json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(newFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("new mode = %o, want 600", info.Mode().Perm())
	}
}

func TestNewFileHandlerValidation(t *testing.T) {
	if _, err := NewFileHandler("", false); err == nil {
		t.Fatal("empty filename accepted")
	}
	if _, err := NewFileHandler("config.conf", false, WithCodec(nil)); err == nil {
		t.Fatal("nil codec accepted")
	}
}
