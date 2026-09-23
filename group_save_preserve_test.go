package cfggo_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iqhive/cfggo"
)

type preserveRootConfig struct {
	cfggo.Structure
	Host func() string `cfggo:"db.host" default:"localhost"`
	Port func() int    `cfggo:"db.port" default:"5432"`
}

type preserveNsConfig struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func readDocument(t *testing.T, file string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v (%s)", file, err, data)
	}
	return doc
}

// Group.Save must not discard top-level sections the members do not own: a
// section ignored with GroupWithIgnoredSections belongs to another program
// sharing the file, and an unclaimed section is the user's data.
func TestGroupSavePreservesForeignSections(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	initial := `{"auth":{"port":1},"other-binary":{"token":"keep-me"},"unclaimed":{"x":1}}`
	if err := os.WriteFile(file, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	auth := &preserveNsConfig{}
	g := cfggo.NewGroup(cfggo.GroupWithFileConfig(file), cfggo.GroupWithoutFlags(),
		cfggo.GroupWithIgnoredSections("other-binary"))
	g.Register("auth", auth)
	if err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := auth.Set("port", 2); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := g.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	doc := readDocument(t, file)
	if string(doc["other-binary"]) != `{"token":"keep-me"}` {
		t.Fatalf("ignored section was not preserved verbatim: %s", doc["other-binary"])
	}
	if string(doc["unclaimed"]) != `{"x":1}` {
		t.Fatalf("unclaimed section was not preserved: %s", doc["unclaimed"])
	}
	var authDoc map[string]int
	if err := json.Unmarshal(doc["auth"], &authDoc); err != nil || authDoc["port"] != 2 {
		t.Fatalf("auth section = %s, want port 2", doc["auth"])
	}

	// A member's own Save is forwarded to the group and must preserve too
	if err := auth.Set("port", 3); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := auth.Save(); err != nil {
		t.Fatalf("member Save: %v", err)
	}
	doc = readDocument(t, file)
	if string(doc["other-binary"]) != `{"token":"keep-me"}` {
		t.Fatalf("ignored section lost by member Save: %s", doc["other-binary"])
	}
	if err := json.Unmarshal(doc["auth"], &authDoc); err != nil || authDoc["port"] != 3 {
		t.Fatalf("auth section = %s, want port 3", doc["auth"])
	}
}

// A root member's keys are written flat and dotted, exactly as a standalone
// Structure writes them ({"db.host": ...}). A later Init or Reload of the
// group must recognise those keys as the root member's, not report them as
// sections no member claims (which fails Init under GroupWithStrictKeys).
func TestGroupRootMemberDottedKeysRoundTripStrict(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	root := &preserveRootConfig{}
	auth := &preserveNsConfig{}
	g := cfggo.NewGroup(cfggo.GroupWithFileConfig(file), cfggo.GroupWithoutFlags(), cfggo.GroupWithStrictKeys())
	g.Register("", root)
	g.Register("auth", auth)
	if err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := root.Set("db.host", "example.org"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := g.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := g.Reload(); err != nil {
		t.Fatalf("Reload after Save: %v", err)
	}

	// Simulate a restart from the saved file
	root2 := &preserveRootConfig{}
	auth2 := &preserveNsConfig{}
	g2 := cfggo.NewGroup(cfggo.GroupWithFileConfig(file), cfggo.GroupWithoutFlags(), cfggo.GroupWithStrictKeys())
	g2.Register("", root2)
	g2.Register("auth", auth2)
	if err := g2.Init(); err != nil {
		t.Fatalf("strict Init from a document the group itself saved failed: %v", err)
	}
	if got := root2.Host(); got != "example.org" {
		t.Fatalf("Host() = %q, want example.org", got)
	}
	if got := auth2.Port(); got != 1 {
		t.Fatalf("auth Port() = %d, want 1", got)
	}
}

// When the combined document cannot be read at reload time, every member must
// keep its previous values, as a standalone Structure does, instead of being
// silently reset to its defaults because the (optional) source is absent.
func TestGroupReloadKeepsValuesWhenOptionalSourceDisappears(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":4242}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := &preserveNsConfig{}
	g := cfggo.NewGroup(cfggo.GroupWithDefaultFileConfig(file), cfggo.GroupWithoutFlags())
	g.Register("auth", auth)
	if err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := auth.Port(); got != 4242 {
		t.Fatalf("Port() = %d, want 4242", got)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := g.Reload(); err == nil {
		t.Fatal("Reload succeeded although the combined document is gone")
	}
	if got := auth.Port(); got != 4242 {
		t.Fatalf("Port() after failed reload = %d, want 4242 (member was reset to its default)", got)
	}

	// Save must not clobber the file from an empty base either
	if err := g.Save(); err == nil {
		t.Fatal("Save succeeded although the combined document could not be read")
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("Save wrote a file although it could not read the current document")
	}
}
