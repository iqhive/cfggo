package flags

import (
	"fmt"
	"reflect"

	"github.com/iqhive/cfggo/internal/convert"
)

// ConfigVar is a flag.Value implementation that converts a string flag value
// to the target type and delegates storage to a caller-supplied setter.
// It is the primary bridge between Go's flag package and the cfggo config map.
type ConfigVar struct {
	Name   string
	Want   reflect.Type
	Setter func(interface{}) error
	// IsSecret marks the backing field as secret-tagged. Conversion errors
	// quote the raw input, so they are redacted for secret flags: the error the
	// flag package prints (and any log of it) must never carry the credential.
	IsSecret bool
	// IsBool marks this flag as boolean so the standard flag package treats it
	// like the built-in bool flags (i.e. "--flag" without a value, and
	// "--flag=true"/"--flag=false"). It also allows boolean values to be
	// propagated to the config map during Parse, no matter which flag set
	// (private or flag.CommandLine) performs the parsing.
	IsBool bool
}

// IsBoolFlag reports whether this flag is boolean. The standard library's flag
// package looks for this method to decide whether the flag may be used without a
// following value.
func (d *ConfigVar) IsBoolFlag() bool {
	return d.IsBool
}

// Set implements flag.Value: converts s to the target type then calls Setter.
func (d *ConfigVar) Set(s string) error {
	if d.Want == nil {
		return fmt.Errorf("ConfigVar has nil type")
	}

	// interface{} targets are handled by ConvertString, which infers a typed
	// value (int, float64, bool, JSON container, or string) from the text
	value, err := convert.ConvertString(s, d.Want, nil)
	if err != nil {
		if d.IsSecret {
			return fmt.Errorf("flag %q: invalid value (redacted: field is secret)", d.Name)
		}
		return fmt.Errorf("flag %q: %w", d.Name, err)
	}
	if err := d.Setter(value); err != nil {
		return fmt.Errorf("flag %q: %w", d.Name, err)
	}
	return nil
}

// String implements flag.Value. ConfigVar does not cache the value itself;
// callers retrieve values through their own storage.
func (d *ConfigVar) String() string {
	return ""
}
