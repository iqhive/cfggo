package sources

import (
	"net/http"
	"strings"
	"testing"
)

func TestPlaintextRefusalDoesNotEchoURLCredentials(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://user:hunter2-PASSWORD@config.example.com/app.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewHandlerHTTP(req, nil, false).LoadConfig()
	if err == nil {
		t.Fatal("plaintext non-loopback URL was accepted")
	}
	if strings.Contains(err.Error(), "hunter2-PASSWORD") {
		t.Fatalf("refusal error echoes the URL password: %v", err)
	}
	if !strings.Contains(err.Error(), "config.example.com") {
		t.Fatalf("refusal error = %v, want the host named", err)
	}

	saver, _ := http.NewRequest(http.MethodPut, "http://user:hunter2-PASSWORD@config.example.com/app.json", nil)
	err = NewHandlerHTTP(nil, saver, false).SaveConfig([]byte(`{}`))
	if err == nil || strings.Contains(err.Error(), "hunter2-PASSWORD") {
		t.Fatalf("SaveConfig refusal = %v, want an error without the password", err)
	}
}
