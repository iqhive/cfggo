package cfggo

import (
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

// maxProvenanceTrail bounds the per-key source history so a long-running
// service that reloads repeatedly cannot grow the trail without limit. Only the
// most recent entries are retained
const maxProvenanceTrail = 8

// recordSourceLocked records the provenance of key. The caller must hold
// c.configMutex for writing.
//
// In addition to the single most-recent source, it maintains an ordered
// override chain (provenanceTrail) that powers SourceChain/Explain. The trail
// is allocated lazily: a key resolved by a single source records nothing extra,
// so the overwhelmingly common case stays allocation-free. A trail entry is
// only created the first time a key's source actually changes, at which point
// the prior source is captured so the chain reads default -> file -> env -> ...
func (c *Structure) recordSourceLocked(key string, src Source) {
	if c.provenance == nil {
		c.provenance = make(map[string]Source)
	}
	prev, had := c.provenance[key]
	c.provenance[key] = src

	// No history to extend until a key is overridden by a different source
	if !had || prev == src {
		return
	}

	if c.provenanceTrail == nil {
		c.provenanceTrail = make(map[string][]Source)
	}
	trail := c.provenanceTrail[key]
	if len(trail) == 0 {
		// Seed with the prior source so the chain begins at the original value
		trail = []Source{prev, src}
	} else if trail[len(trail)-1] != src {
		if len(trail) >= maxProvenanceTrail {
			// Drop the oldest entry in place (no allocation) to stay bounded
			copy(trail, trail[1:])
			trail[len(trail)-1] = src
		} else {
			trail = append(trail, src)
		}
	}
	c.provenanceTrail[key] = trail
}

// SourceChain returns the ordered chain of sources that have contributed to
// key's current value, eg [SourceDefault, SourceFile, SourceEnv]. When a key was
// only ever resolved by a single source the chain has one element (that
// source); an unknown key returns nil. It answers "which layers set this, and
// in what order?" — the natural follow-up to Source's "where is it from now?"
func (c *Structure) SourceChain(key string) []Source {
	c.ensureInit()
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	if trail, ok := c.provenanceTrail[key]; ok && len(trail) > 0 {
		out := make([]Source, len(trail))
		copy(out, trail)
		return out
	}
	if src, ok := c.provenance[key]; ok {
		return []Source{src}
	}
	return nil
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
// When a value was set by more than one source, the full override chain is
// shown in brackets (eg "[default->file->env]"), making it the recommended
// starting point for debugging "why is this value X?"
//
// Values for fields tagged `secret:"true"` are masked,
// so the output is safe to log or paste into a bug report
func (c *Structure) Explain() string {
	return c.renderHuman(true)
}
