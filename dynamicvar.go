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
	return d.config.Set(d.name, value)
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
