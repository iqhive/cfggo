package flags

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/iqhive/cfggo/internal/convert"
)

// ConfigVar is a flag.Value implementation that converts a string flag value
// to the target type and delegates storage to a caller-supplied setter.
// It is the primary bridge between Go's flag package and the cfggo config map.
type ConfigVar struct {
	Name   string
	Want   reflect.Type
	Setter func(interface{}) error
}

// Set implements flag.Value: converts s to the target type then calls Setter.
func (d *ConfigVar) Set(s string) error {
	if d.Want == nil {
		return fmt.Errorf("ConfigVar has nil type")
	}

	// For interface{} targets, infer a concrete Go type from the string so that
	// callers get a typed value (int, float64, bool, or string) rather than a
	// bare string.
	if d.Want.Kind() == reflect.Interface {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			if float64(int(f)) == f {
				return d.Setter(int(f))
			}
			return d.Setter(f)
		}
		if b, err := strconv.ParseBool(s); err == nil {
			return d.Setter(b)
		}
		return d.Setter(s)
	}

	value, err := convert.ConvertString(s, d.Want, nil)
	if err != nil {
		return err
	}
	return d.Setter(value)
}

// String implements flag.Value. ConfigVar does not cache the value itself;
// callers retrieve values through their own storage.
func (d *ConfigVar) String() string {
	return ""
}
