package cfggo

import (
	"fmt"
	"reflect"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

type dynamicVar struct {
	config *Structure
	name   string
	want   reflect.Type
	// source is the provenance recorded when this var applies a value. Defaults
	// to SourceSet when unset (zero value would be SourceUnknown, so callers
	// that care set this explicitly).
	source Source
}

func (d *dynamicVar) Set(s string) error {
	if d.config == nil {
		return fmt.Errorf("dynamicVar config is nil")
	}
	if d.want == nil {
		return fmt.Errorf("dynamicVar has nil type")
	}

	value, err := iconvert.ConvertString(s, d.want, d.config)
	if err != nil {
		return err
	}
	src := d.source
	if src == SourceUnknown {
		src = SourceSet
	}
	return d.config.applyLoaded(d.name, value, src)
}

func (d *dynamicVar) String() string {
	if d == nil || d.config == nil {
		return ""
	}
	val, ok := d.config.Get(d.name)
	if !ok {
		return ""
	}
	return fmt.Sprint(val)
}
