package cfggo

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/iqhive/cfggo/sources"
)

// Source identifies where a configuration value originated. It powers the
// provenance tracking surfaced by Source, Sources, and Explain, which answer
// the most common config support question: "why is this value what it is?".
type Source int

const (
	// SourceUnknown means no provenance was recorded for the key.
	SourceUnknown Source = iota
	// SourceDefault means the value came from a struct default (zero value or a
	// `default:"..."` tag).
	SourceDefault
	// SourceFile means the value came from a file-backed ConfigHandler.
	SourceFile
	// SourceHTTP means the value came from an HTTP-backed ConfigHandler.
	SourceHTTP
	// SourceEnv means the value came from an environment variable.
	SourceEnv
	// SourceFlag means the value came from a command-line flag.
	SourceFlag
	// SourceSet means the value was set programmatically via Set.
	SourceSet
)

// String returns a short, human-readable name for the source.
func (s Source) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceFile:
		return "file"
	case SourceHTTP:
		return "http"
	case SourceEnv:
		return "env"
	case SourceFlag:
		return "flag"
	case SourceSet:
		return "set"
	default:
		return "unknown"
	}
}

// recordSourceLocked records the provenance of key. The caller must hold
// c.configMutex for writing.
func (c *Structure) recordSourceLocked(key string, src Source) {
	if c.provenance == nil {
		c.provenance = make(map[string]Source)
	}
	c.provenance[key] = src
}

// handlerSource maps the configured ConfigHandler to the Source it represents.
func (c *Structure) handlerSource() Source {
	switch c.configHandler.(type) {
	case *sources.HandlerEnv:
		return SourceEnv
	case *sources.HandlerHTTP:
		return SourceHTTP
	default:
		return SourceFile
	}
}

// Source returns where the current value of key came from, and whether any
// provenance was recorded.
func (c *Structure) Source(key string) (Source, bool) {
	c.ensureInit()
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	src, ok := c.provenance[key]
	return src, ok
}

// Sources returns a copy of the provenance map: config key -> originating
// Source. The returned map is safe for the caller to retain and mutate.
func (c *Structure) Sources() map[string]Source {
	c.ensureInit()
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	out := make(map[string]Source, len(c.provenance))
	for k, v := range c.provenance {
		out[k] = v
	}
	return out
}

// Explain returns a human-readable, key-sorted dump of every configuration
// value annotated with where it came from (and the field's help text, if any).
// It is the recommended starting point for debugging "why is this value X?".
//
// Values for fields tagged `secret:"true"` are masked,
// so the output is safe to log or paste into a bug report
func (c *Structure) Explain() string {
	c.ensureInit()

	c.configMutex.RLock()
	keys := make([]string, 0, len(c.configData))
	values := make(map[string]string, len(c.configData))
	srcs := make(map[string]Source, len(c.configData))
	for key, value := range c.configData {
		keys = append(keys, key)
		if c.isSecretKey(key) {
			values[key] = maskedValue
		} else {
			values[key] = fmt.Sprintf("%v", value)
		}
		srcs[key] = c.provenance[key]
	}
	c.configMutex.RUnlock()

	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.name + ":\n")
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	for _, key := range keys {
		help := c.GetHelpTag(key)
		if help != "" {
			fmt.Fprintf(tw, "%s\t%s\t(from %s)\t// %s\n", key, values[key], srcs[key], help)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t(from %s)\t\n", key, values[key], srcs[key])
		}
	}
	tw.Flush()
	return sb.String()
}
