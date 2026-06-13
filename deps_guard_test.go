package cfggo

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bannedImportSubstrings lists module path fragments for third-party
// configuration libraries. cfggo's whole value proposition is being a
// self-contained, dependency-free config library, so importing any of these
// (directly or in tests/examples/benchmarks) must fail the build.
var bannedImportSubstrings = []string{
	"spf13/viper",
	"knadh/koanf",
	"kelseyhightower/envconfig",
	"ilyakaznacheev/cleanenv",
	"gookit/config",
	"cristalhq/aconfig",
	"jinzhu/configor",
	"go-ini/ini",
	"gopkg.in/ini",
	"namsral/flag",
	"peterbourgon/ff",
	"urfave/cli",
	"alecthomas/kong",
	// Generic catch-all: anything that literally calls itself "viper".
	"viper",
}

// TestNoThirdPartyConfigImports walks every Go file in the module and asserts
// that none import a known third-party configuration package. This protects the
// dependency-free guarantee documented in the README.
func TestNoThirdPartyConfigImports(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip hidden directories (e.g. .git) and any vendored tree.
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}

		for _, imp := range f.Imports {
			// imp.Path.Value includes the surrounding quotes.
			p := strings.Trim(imp.Path.Value, `"`)
			low := strings.ToLower(p)
			for _, banned := range bannedImportSubstrings {
				if strings.Contains(low, banned) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s imports banned config package %q (matched %q)", rel, p, banned)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk module: %v", walkErr)
	}
}

// TestModuleHasNoThirdPartyRequires asserts go.mod declares no third-party
// module dependencies at all, the strongest form of the dependency-free
// guarantee. Only the module's own directive and Go/toolchain lines are allowed.
func TestModuleHasNoThirdPartyRequires(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	inRequireBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "require ("):
			inRequireBlock = true
			continue
		case inRequireBlock && line == ")":
			inRequireBlock = false
			continue
		case strings.HasPrefix(line, "require "):
			// Single-line require directive.
			t.Errorf("go.mod has a third-party require: %q", line)
		case inRequireBlock:
			t.Errorf("go.mod has a third-party require: %q", line)
		}
	}
}
