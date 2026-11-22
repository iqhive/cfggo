package sources

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPRedirectErrorDoesNotEchoLocationCredentials(t *testing.T) {
	const password = "redirect-location-password-8241"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header: http.Header{"Location": []string{
				"http://user:" + password + "@plaintext.example.test/config",
			}},
			Body: io.NopCloser(strings.NewReader("")),
		}, nil
	})}
	source, _ := http.NewRequest(http.MethodGet, "https://config.example.test/config", nil)
	handler := NewHandlerHTTP(source, nil, false)
	handler.Client = client

	_, err := handler.LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig accepted a plaintext redirect")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("redirect error exposes the Location password: %v", err)
	}
}

func TestSaveConfigFollowsEntireDanglingSymlinkChain(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	intermediate := filepath.Join(dir, "intermediate.json")
	final := filepath.Join(dir, "final.json")
	if err := os.Symlink("intermediate.json", config); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("final.json", intermediate); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := NewHandlerFile(config, false).SaveConfig([]byte(`{"port":4}`)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("final symlink target was not created: %v", err)
	}
	if string(got) != `{"port":4}` {
		t.Fatalf("final target = %s, want saved document", got)
	}
	info, err := os.Lstat(intermediate)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("SaveConfig replaced the intermediate symlink instead of following it")
	}
}
