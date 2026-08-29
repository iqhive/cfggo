//go:build vhs

package cfggo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type tapeTranscriptTest struct {
	name     string
	script   string
	want     []string
	forbid   []string
	timeout  time.Duration
	optional bool
}

func TestEveryRootTapeHasTranscriptTest(t *testing.T) {
	tapes, err := filepath.Glob("*.tape")
	if err != nil {
		t.Fatal(err)
	}

	covered := make(map[string]bool)
	for _, tt := range tapeTranscriptTests() {
		covered[tt.name] = true
	}

	for _, tape := range tapes {
		if !covered[tape] {
			t.Fatalf("%s does not have a transcript test", tape)
		}
	}
}

func TestVHSTapeTranscripts(t *testing.T) {
	restoreDemoConfig := restoreFileAfterTest(t, "examples/demo/config.json")
	defer restoreDemoConfig()

	for _, tt := range tapeTranscriptTests() {
		t.Run(tt.name, func(t *testing.T) {
			timeout := tt.timeout
			if timeout == 0 {
				timeout = 45 * time.Second
			}

			output := runTapeTranscript(t, timeout, tt.script)
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Fatalf("transcript missing %q\n\n%s", want, output)
				}
			}
			for _, forbidden := range append(commonForbiddenTranscriptText(), tt.forbid...) {
				if strings.Contains(strings.ToLower(output), strings.ToLower(forbidden)) {
					t.Fatalf("transcript contains forbidden text %q\n\n%s", forbidden, output)
				}
			}
		})
	}
}

func TestVHSTapeSyntax(t *testing.T) {
	if _, err := exec.LookPath("vhs"); err != nil {
		t.Skip("vhs is not installed")
	}

	tapes, err := filepath.Glob("*.tape")
	if err != nil {
		t.Fatal(err)
	}
	if len(tapes) == 0 {
		t.Fatal("no VHS tapes found")
	}

	args := append([]string{"validate"}, tapes...)
	cmd := exec.Command("vhs", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("vhs validate failed: %v\n%s", err, out)
	}
}

func tapeTranscriptTests() []tapeTranscriptTest {
	return []tapeTranscriptTest{
		{
			name: "advanced.tape",
			script: `set -euo pipefail
go run ./examples/advanced 2>&1 | sed -E 's|^[0-9]{4}/[^ ]+ [0-9:]+ |LOG |' | sed -n '1,22p'`,
			want: []string{
				"=== Advanced Error Handling and Logging Example ===",
				"LOG [SERVICE-A ERROR]",
				"Service B error: [CODE:500] Internal error in service B",
				"Service A: analytics-service, Retries: 5",
			},
		},
		{
			name: "banner.tape",
			script: `set -euo pipefail
cd examples/demo
sed -n '12,27p' main.go`,
			want: []string{
				"type Config struct",
				"Port     func() int",
				"cfggo.WithLogger(&cfglogger.NoopLogger{})",
			},
		},
		{
			name: "debugging.tape",
			script: `set -euo pipefail
go run ./examples/debugging 2>&1 | sed -n '1,40p'`,
			want: []string{
				"=== Blessed Startup Pattern ===",
				"redacted report:",
				"Unrecognized keys (no matching struct field): stale_key",
				"bad JSON",
				"wrong type",
			},
		},
		{
			name: "demo.tape",
			script: `set -euo pipefail
cd examples/demo
cp config.initial.json config.json
sed -n '12,35p' main.go
go build -o .demo-app .
./.demo-app > demo-transcript.out 2>&1 &
pid=$!
sleep 3
cat > config.json <<'EOF'
{
  "port": 9090,
  "log_level": "debug"
}
EOF
sleep 2
kill "$pid"
wait "$pid" 2>/dev/null || true
cat demo-transcript.out
rm -f demo-transcript.out .demo-app
cp config.initial.json config.json`,
			want: []string{
				"type Config struct",
				"app running: port=8080 log_level=info",
				"config reloaded",
				"port: 8080 -> 9090",
				"typed accessors: port=9090 log_level=debug",
			},
			timeout: 60 * time.Second,
		},
		{
			name: "hotreload.tape",
			script: `set -euo pipefail
cd examples/demo
cp config.initial.json config.json
go build -o .demo-app .
./.demo-app > hotreload-transcript.out 2>&1 &
pid=$!
sleep 3
cat > config.json <<'EOF'
{
  "port": 3000,
  "log_level": "debug"
}
EOF
sleep 2
cat > config.json <<'EOF'
{
  "port": 8443,
  "log_level": "warn"
}
EOF
sleep 2
kill "$pid"
wait "$pid" 2>/dev/null || true
cat hotreload-transcript.out
rm -f hotreload-transcript.out .demo-app
cp config.initial.json config.json`,
			want: []string{
				"app running: port=8080 log_level=info",
				"port: 8080 -> 3000",
				"log_level: info -> debug",
				"port: 3000 -> 8443",
				"log_level: debug -> warn",
				"typed accessors: port=8443 log_level=warn",
			},
			timeout: 60 * time.Second,
		},
		{
			name: "validation.tape",
			script: `set -euo pipefail
cd examples/validation
go run main.go`,
			want: []string{
				"Configuration is valid!",
				"server_port 8080 is valid",
				"Expected error: validation failed for 'server_port'",
				"max_connections 100 is valid",
			},
		},
	}
}

func runTapeTranscript(t *testing.T, timeout time.Duration, script string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", script)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("transcript timed out after %s\n%s", timeout, out)
	}
	if err != nil {
		t.Fatalf("transcript command failed: %v\n%s", err, out)
	}
	return string(out)
}

func commonForbiddenTranscriptText() []string {
	return []string{
		"bad flag in substitute command",
		"command not found",
		"no such file or directory",
		"sed:",
		"syntax error",
		"exit status",
		"panic:",
	}
}

func restoreFileAfterTest(t *testing.T, path string) func() {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return func() {
		if err := os.WriteFile(path, contents, 0644); err != nil {
			t.Fatalf("restore %s: %v", path, err)
		}
	}
}
