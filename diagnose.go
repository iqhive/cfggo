package cfggo

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/iqhive/cfggo/validcfg"
)

// KeyDiagnostic is the resolved state of a single configuration key
// as reported by Diagnose
type KeyDiagnostic struct {
	// Key is the dotted configuration key
	Key string
	// Value is the currently resolved value
	Value interface{}
	// Source is where the current value came from
	Source Source
	// Help is the field's `help` tag, if any.
	Help string
	// EnvVar is the environment variable name cfggo reads for this key
	EnvVar string
	// Type is the accessor's return type (eg "int"), or "" when unknown
	Type string
	// Recognized reports whether the key is backed by a struct field. A false
	// value usually indicates a typo in a config file or environment variable
	Recognized bool
	// Err is the validation error for this key
	// or nil when it passes or has no validator
	Err error
}

// Diagnostics is a structured, machine-readable snapshot of a configuration's
// resolved state: every key with its value, provenance, type, help text, and
// validation result, plus any unrecognized keys. It is the one-stop building
// block for a "--config-check" / dry-run command
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

// Diagnose returns a structured snapshot of the configuration's resolved state.
// Unlike Explain (which returns a formatted string), Diagnose returns data the
// caller can inspect programmatically — eg to implement a `--config-check`
// flag that prints the report and exits non-zero when Valid is false
func (c *Structure) Diagnose() Diagnostics {
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
	c.configMutex.RUnlock()

	c.validationMutex.RLock()
	validators := make(map[string]validcfg.Validator)
	for k, v := range c.validationMap[c.name] {
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
		info := c.fields[key]
		_, recognized := c.fields[key]
		kd := KeyDiagnostic{
			Key:        key,
			Value:      data[key],
			Source:     prov[key],
			Help:       info.Help,
			EnvVar:     internalEnvLoader.KeyToEnvVar(key),
			Recognized: recognized,
		}
		if info.Type != nil {
			kd.Type = info.Type.String()
		}
		if v, ok := validators[key]; ok {
			if err := v(data[key]); err != nil {
				kd.Err = validcfg.ValidationError{Key: key, Err: err}.WithProvenance(data[key], prov[key].String())
				valid = false
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

// ConfigReference returns a human-readable reference table of every
// configuration field: its key, type, environment variable name, default, and
// help text. It is generated from the struct definition (not the current
// values), making it ideal for documentation or a `--help` style listing
func (c *Structure) ConfigReference() string {
	c.ensureInit()

	infos := make([]fieldInfo, 0, len(c.fields))
	for _, info := range c.fields {
		if !info.IsAccessor {
			continue
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Key < infos[j].Key })

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s configuration reference:\n", c.name)
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tTYPE\tENV\tDEFAULT\tHELP")
	for _, info := range infos {
		typ := "-"
		if info.Type != nil {
			typ = info.Type.String()
		}
		def := "-"
		if info.HasDefault {
			def = info.DefaultTag
		}
		help := info.Help
		if help == "" {
			help = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", info.Key, typ, internalEnvLoader.KeyToEnvVar(info.Key), def, help)
	}
	tw.Flush()
	return sb.String()
}
