package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/iqhive/banner"
	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/cfglogger"
)

type demoConfig struct {
	cfggo.Structure
	Port  func() int  `cfggo:"port" default:"8080"`
	Debug func() bool `cfggo:"debug" default:"false"`
}
type result struct {
	port   int
	debug  bool
	source string
}

// fixtures isolate flags and environment from the generator's own CLI. Files
// are real JSON files, read again by Reload; no result is simulated.
func fixtures() (layers []result, reload []result) {
	dir, err := os.MkdirTemp("", "cfggo-hero-")
	must(err)
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.json")
	write := func(port int, debug bool) {
		must(os.WriteFile(path, []byte(fmt.Sprintf(`{"port":%d,"debug":%t}`, port, debug)), 0600))
	}
	const envKey = "CFGGO_HERO_PORT"
	old, exists := os.LookupEnv(envKey)
	must(os.Unsetenv(envKey))
	defer func() {
		if exists {
			_ = os.Setenv(envKey, old)
		} else {
			_ = os.Unsetenv(envKey)
		}
	}()
	// A separate key is disabled so ambient variables cannot alter Debug.
	const debugKey = "CFGGO_HERO_DEBUG"
	oldDebug, debugExists := os.LookupEnv(debugKey)
	must(os.Unsetenv(debugKey))
	defer func() {
		if debugExists {
			_ = os.Setenv(debugKey, oldDebug)
		} else {
			_ = os.Unsetenv(debugKey)
		}
	}()
	snapshot := func(c *demoConfig) result {
		src, ok := c.Source("port")
		if !ok {
			panic("hero fixture: port has no source")
		}
		return result{c.Port(), c.Debug(), src.String()}
	}
	for stage := 0; stage < 4; stage++ {
		c := &demoConfig{}
		fs := flag.NewFlagSet("fixture", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		opts := []cfggo.Option{cfggo.WithFlagSet(fs), cfggo.WithEnvPrefix("CFGGO_HERO_"), cfggo.WithLogger(&cfglogger.NoopLogger{})}
		if stage >= 1 {
			write(9090, false)
			opts = append(opts, cfggo.WithFileConfig(path))
		}
		if stage >= 2 {
			must(os.Setenv(envKey, "7000"))
		}
		must(cfggo.Init(c, opts...))
		if stage == 3 {
			must(fs.Parse([]string{"--port=6000"}))
		}
		layers = append(layers, snapshot(c))
	}
	write(9090, false)
	c := &demoConfig{}
	must(cfggo.Init(c, cfggo.WithFileConfig(path), cfggo.WithoutEnv(), cfggo.WithoutFlags(), cfggo.WithLogger(&cfglogger.NoopLogger{})))
	reload = append(reload, snapshot(c))
	write(9191, true)
	must(c.Reload())
	reload = append(reload, snapshot(c))
	return
}
func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("hero fixture: %v", err))
	}
}

// typeLine keeps a visible cursor on each typed input frame.
func (m *film) typeLine(x, y int, s string, ms int) {
	for i := 0; i <= len(s); i++ {
		m.cur.Clear(y, y+1)
		m.cur.Text(x, y, s[:i], lit)
		m.cur.Text(x+i, y, " ", caret)
		m.cur.Cells[y][x+i].Rune = '▏'
		m.cut(ms)
	}
	m.cur.Clear(y, y+1)
	m.cur.Text(x, y, s, lit)
}
func (m *film) layers() {
	values, _ := fixtures()
	m.cur = frame{}
	m.cur.Text(3, 1, "01 / typed access, with a source you can explain", muted)
	m.cur.Text(3, 3, "type Config struct {", plain)
	m.cur.Text(5, 4, "cfggo.Structure", plain)
	m.cur.Text(5, 5, "Port func() int  `cfggo:\"port\" default:\"8080\"`", lit)
	m.cur.Text(3, 6, "}", plain)
	m.cur.Text(3, 9, "LATER SOURCES WIN", muted)
	for i, r := range values {
		labels := []string{"default: 8080", "JSON: 9090", "env: 7000", "flag: 6000"}
		x := 3 + i*19
		m.cur.Text(x, 11, labels[i], hot)
		m.cur.Text(x, 12, strings.Repeat(string(full), 16), bar)
		if i > 0 {
			m.cur.Text(x-2, 11, "›", muted)
		}
		m.cur.Clear(15, 19)
		m.cur.Text(3, 15, fmt.Sprintf("cfg.Port()          → %d  (int)", r.port), bright)
		m.cur.Text(3, 17, fmt.Sprintf("cfg.Source(\"port\")  → %s, true", r.source), muted)
		m.cut(430)
	}
	m.typeLine(3, 8, `cfg.Port()`, 65)
	m.cut(1400)
}
func (m *film) reload() {
	_, values := fixtures()
	m.cur = frame{}
	m.cur.Text(3, 1, "02 / reload the file; keep the same typed accessors", muted)
	m.cur.Text(3, 4, "config.json", muted)
	m.cur.Text(43, 4, "RUNNING APPLICATION", muted)
	show := func(r result, st style) {
		m.cur.Clear(6, 13)
		m.cur.Text(3, 6, fmt.Sprintf(`{ "port": %d,`, r.port), st)
		m.cur.Text(3, 7, fmt.Sprintf(`  "debug": %t }`, r.debug), st)
		m.cur.Text(34, 8, "───›", bar)
		m.cur.Text(43, 7, fmt.Sprintf("cfg.Port()  → %d", r.port), st)
		m.cur.Text(43, 9, fmt.Sprintf("cfg.Debug() → %t", r.debug), st)
		m.cur.Text(3, 12, strings.Repeat(string(full), 28), bar)
	}
	show(values[0], plain)
	m.cut(1100)
	for y := 6; y < 8; y++ {
		for x := 3; x < 32; x++ {
			m.cur.Cells[y][x] = banner.Cell{}
		}
	}
	m.cur.Text(3, 6, fmt.Sprintf(`{ "port": %d,`, values[1].port), hot)
	m.cur.Text(3, 7, fmt.Sprintf(`  "debug": %t }`, values[1].debug), hot)
	m.cur.Text(3, 14, "JSON file changed; accessors still read the current snapshot", muted)
	m.cut(650)
	m.typeLine(3, 16, "cfg.Reload()", 70)
	show(values[1], hot)
	m.cur.Clear(14, 15)
	m.cur.Text(3, 14, "Reload complete. Same functions, new values. No restart.", muted)
	m.cur.Text(3, 18, "Reload() → nil", bright)
	m.cut(1910)
}
