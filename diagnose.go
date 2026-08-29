package cfggo

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/iqhive/cfggo/validcfg"
)

// KeyDiagnostic is the resolved state of a single configuration key
// as reported by DiagnoseData
type KeyDiagnostic struct {
	// Key is the dotted configuration key
	Key string
	// Value is the currently resolved value
	Value interface{}
	// Source is where the current value came from
	Source Source
	// SourceChain is the ordered chain of sources that contributed to the
	// current value (eg default -> file -> env). It has a single element when
	// only one source ever set the key. It answers "which layers set this?"
	SourceChain []Source
	// Help is the field's `help` tag, if any.
	Help string
	// EnvVar is the environment variable name cfggo reads for this key
	EnvVar string
	// Type is the accessor's return type (eg "int"), or "" when unknown
	Type string
	// Default is the field's `default` tag value (masked when the field is
	// secret), or "" when none was declared
	Default string
	// HasDefault reports whether a `default` tag was declared
	HasDefault bool
	// IsAccessor reports whether the key is backed by a func() T accessor field
	// (as opposed to an unrecognized key present only in the config data)
	IsAccessor bool
	// Recognized reports whether the key is backed by a struct field. A false
	// value usually indicates a typo in a config file or environment variable
	Recognized bool
	// Suggestion is the closest known configuration key for an unrecognized key,
	// when cfggo can make a confident match.
	Suggestion string
	// Secret reports whether the field is tagged `secret:"true"`. When true,
	// Value (and Default) are masked (set to "****") so the diagnostic never
	// carries the sensitive value; validation is still performed against the
	// real value
	Secret bool
	// Err is the validation error for this key
	// or nil when it passes or has no validator
	Err error
}

// Diagnostics is a structured, machine-readable snapshot of a configuration's
// resolved state: every key with its value, provenance, type, help text, and
// validation result, plus any unrecognized keys. It is the one-stop building
// block for a "--config-check" / dry-run command, and the single data model
// every human-readable report (Diagnose, ConfigReference, Report) is rendered
// from. Obtain it via DiagnoseData and format it however you like
type Diagnostics struct {
	// Name is the configuration's name.
	Name string
	// Keys holds one entry per resolved configuration key, sorted by key
	Keys []KeyDiagnostic
	// Unrecognized lists configuration keys with no matching struct field
	Unrecognized []string
	// Valid reports whether every key passed its validators
	Valid bool
}

// DiagnoseData returns a structured snapshot of the configuration's resolved
// state. It is the canonical data model: callers can inspect it programmatically
// (eg to implement a `--config-check` flag that prints a report and exits
// non-zero when Valid is false) or render their own formatting. The built-in
// Diagnose, ConfigReference, and Report helpers are all thin formatters over
// this data, so a custom renderer never diverges from them
func (c *Structure) DiagnoseData() Diagnostics {
	c.ensureInit()

	c.configMutex.RLock()
	data := make(map[string]interface{}, len(c.configData))
	for k, v := range c.configData {
		data[k] = v
	}
	prov := make(map[string]Source, len(c.provenance))
	for k, v := range c.provenance {
		prov[k] = v
	}
	var chains map[string][]Source
	if len(c.provenanceTrail) > 0 {
		chains = make(map[string][]Source, len(c.provenanceTrail))
		for k, t := range c.provenanceTrail {
			cp := make([]Source, len(t))
			copy(cp, t)
			chains[k] = cp
		}
	}
	c.configMutex.RUnlock()

	c.validationMutex.RLock()
	validators := make(map[string]validcfg.Validator, len(c.validationMap))
	for k, v := range c.validationMap {
		validators[k] = v
	}
	c.validationMutex.RUnlock()

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	valid := true
	diags := make([]KeyDiagnostic, 0, len(keys))
	for _, key := range keys {
		var info fieldInfo
		var recognized bool
		if c.plan != nil {
			if leaf, ok := c.plan.byKey[key]; ok {
				info = leaf.info
				recognized = leaf.info.IsAccessor
			}
		}
		kd := KeyDiagnostic{
			Key:         key,
			Value:       data[key],
			Source:      prov[key],
			SourceChain: sourceChainFor(chains, prov, key),
			Help:        info.Help,
			EnvVar:      c.envVarName(key),
			Default:     info.DefaultTag,
			HasDefault:  info.HasDefault,
			IsAccessor:  info.IsAccessor,
			Recognized:  recognized,
			Suggestion:  c.suggestKey(key),
			Secret:      info.IsSecret,
		}
		if info.Type != nil {
			kd.Type = info.Type.String()
		}
		// Validate against the real value before masking,
		// so a bad secret is still reported as invalid
		if v, ok := validators[key]; ok {
			if err := v(data[key]); err != nil {
				kd.Err = validcfg.ValidationError{Key: key, Err: err}.WithProvenance(c.provenanceValue(key, data[key]), prov[key].String())
				valid = false
			}
		}
		// Never let a secret value escape via the diagnostic struct itself.
		// A secret's default tag may itself be a credential, so mask it too
		if kd.Secret {
			kd.Value = maskedValue
			if kd.Default != "" {
				kd.Default = maskedValue
			}
		}
		diags = append(diags, kd)
	}

	return Diagnostics{
		Name:         c.name,
		Keys:         diags,
		Unrecognized: c.unrecognizedKeys(),
		Valid:        valid,
	}
}

// Diagnose returns a structured snapshot of the configuration's resolved state.
// It is an alias for DiagnoseData retained for readability at call sites that
// read the snapshot rather than render it
func (c *Structure) Diagnose() Diagnostics {
	return c.DiagnoseData()
}

// sourceChainFor returns a defensive copy of the recorded override chain for
// key, falling back to the single recorded source when no multi-source chain
// exists. The returned slice is owned by the caller
func sourceChainFor(chains map[string][]Source, prov map[string]Source, key string) []Source {
	if chain, ok := chains[key]; ok && len(chain) > 0 {
		return chain
	}
	if src, ok := prov[key]; ok {
		return []Source{src}
	}
	return nil
}

// String renders the diagnostics as an aligned, human-readable
// table suitable for printing from a `--config-check` command
func (d Diagnostics) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s configuration diagnostics (valid=%t):\n", d.Name, d.Valid)

	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE\tTYPE\tSOURCE\tSTATUS")
	for _, k := range d.Keys {
		status := "ok"
		if k.Err != nil {
			status = "INVALID: " + k.Err.Error()
		} else if !k.Recognized {
			status = "unrecognized"
			if k.Suggestion != "" {
				status += " (did you mean " + k.Suggestion + "?)"
			}
		}
		typ := k.Type
		if typ == "" {
			typ = "-"
		}
		fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%s\n", k.Key, k.Value, typ, k.Source, status)
	}
	tw.Flush()

	if len(d.Unrecognized) > 0 {
		fmt.Fprintf(&sb, "\nUnrecognized keys (no matching struct field): %s\n", strings.Join(d.Unrecognized, ", "))
	}
	return sb.String()
}

// Reference renders the static field reference (key, type, env var, default,
// help) for every accessor-backed key in the snapshot, sorted by key. It is the
// table ConfigReference prints
func (d Diagnostics) Reference() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s configuration reference:\n", d.Name)
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tTYPE\tENV\tDEFAULT\tHELP")
	for _, k := range d.Keys {
		if !k.IsAccessor {
			continue
		}
		typ := k.Type
		if typ == "" {
			typ = "-"
		}
		def := "-"
		if k.HasDefault {
			def = k.Default
		}
		help := k.Help
		if help == "" {
			help = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", k.Key, typ, k.EnvVar, def, help)
	}
	tw.Flush()
	return sb.String()
}

// ConfigReference returns a human-readable reference table of every
// configuration field: its key, type, environment variable name, default, and
// help text. It is generated from the struct definition, making it ideal for
// documentation or a `--help` style listing. Secret defaults are masked
func (c *Structure) ConfigReference() string {
	return c.DiagnoseData().Reference()
}

// Report returns a single, one-stop human-readable dump intended for bug
// reports and "--config-check" style commands. It combines the static field
// reference with the resolved runtime state and validation status. Values for
// fields tagged `secret:"true"` are masked throughout, so the output is safe to
// attach to an issue or paste into a log. It is built from a single
// DiagnoseData snapshot, so the two tables are guaranteed consistent
func (c *Structure) Report() string {
	d := c.DiagnoseData()
	var sb strings.Builder
	sb.WriteString(d.Reference())
	sb.WriteString("\n")
	sb.WriteString(d.String())
	return sb.String()
}

// ExplainKey returns a compact, human-readable explanation for one
// configuration key: its current value, type, source, override chain, env var,
// default/help metadata, and validation status. Secret values are masked.
func (c *Structure) ExplainKey(key string) string {
	d := c.DiagnoseData()
	for _, kd := range d.Keys {
		if kd.Key == key {
			return kd.Explain()
		}
	}
	return fmt.Sprintf("%s: <unknown key>%s\n", key, c.didYouMeanSuffix(key))
}

// Explain renders a compact, human-readable explanation for this key diagnostic.
func (k KeyDiagnostic) Explain() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s:\n", k.Key)
	fmt.Fprintf(&sb, "  value: %v\n", k.Value)
	if k.Type != "" {
		fmt.Fprintf(&sb, "  type: %s\n", k.Type)
	} else {
		fmt.Fprintf(&sb, "  type: -\n")
	}
	fmt.Fprintf(&sb, "  source: %s\n", k.Source)
	if len(k.SourceChain) > 1 {
		fmt.Fprintf(&sb, "  source_chain: %s\n", formatSourceChain(k.SourceChain))
	}
	if k.EnvVar != "" {
		fmt.Fprintf(&sb, "  env: %s\n", k.EnvVar)
	}
	if k.HasDefault {
		fmt.Fprintf(&sb, "  default: %s\n", k.Default)
	}
	if k.Help != "" {
		fmt.Fprintf(&sb, "  help: %s\n", k.Help)
	}
	status := "ok"
	if k.Err != nil {
		status = "INVALID: " + k.Err.Error()
	} else if !k.Recognized {
		status = "unrecognized"
		if k.Suggestion != "" {
			status += " (did you mean " + k.Suggestion + "?)"
		}
	}
	fmt.Fprintf(&sb, "  status: %s\n", status)
	return sb.String()
}

// renderHuman builds the key-sorted, secret-masked dump shared by String and
// Explain. When withSource is true each value is annotated with its final
// source and (when more than one source contributed) the full override chain.
// It is kept deliberately lean — it does not run validators or compute env-var
// names — so the common debug-dump path stays cheap
func (c *Structure) renderHuman(withSource bool) string {
	c.ensureInit()

	c.configMutex.RLock()
	keys := make([]string, 0, len(c.configData))
	values := make(map[string]string, len(c.configData))
	var srcs map[string]Source
	var chains map[string][]Source
	if withSource {
		srcs = make(map[string]Source, len(c.configData))
	}
	for key, value := range c.configData {
		keys = append(keys, key)
		if c.isSecretKey(key) {
			values[key] = maskedValue
		} else {
			values[key] = fmt.Sprintf("%v", value)
		}
		if withSource {
			srcs[key] = c.provenance[key]
			if trail, ok := c.provenanceTrail[key]; ok && len(trail) > 1 {
				if chains == nil {
					chains = make(map[string][]Source)
				}
				cp := make([]Source, len(trail))
				copy(cp, trail)
				chains[key] = cp
			}
		}
	}
	c.configMutex.RUnlock()

	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.name + ":\n")
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	for _, key := range keys {
		help := c.GetHelpTag(key)
		if !withSource {
			// Match the lean two-value layout: no per-row string building so a
			// plain dump stays as cheap as a bare format call
			if help != "" {
				fmt.Fprintf(tw, "%s\t%s\t// %s\n", key, values[key], help)
			} else {
				fmt.Fprintf(tw, "%s\t%s\t\n", key, values[key])
			}
			continue
		}
		// chainCol is empty (no allocation) for the common single-source key;
		// it is only built for keys an override actually touched
		chainCol := ""
		if chain := chains[key]; len(chain) > 1 {
			chainCol = "[" + formatSourceChain(chain) + "]"
		}
		if help != "" {
			fmt.Fprintf(tw, "%s\t%s\t(from %s)\t%s\t// %s\n", key, values[key], srcs[key], chainCol, help)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t(from %s)\t%s\t\n", key, values[key], srcs[key], chainCol)
		}
	}
	tw.Flush()
	return sb.String()
}

// formatSourceChain joins a source chain with arrows, eg "default->env->set"
func formatSourceChain(chain []Source) string {
	parts := make([]string, len(chain))
	for i, s := range chain {
		parts[i] = s.String()
	}
	return strings.Join(parts, "->")
}
