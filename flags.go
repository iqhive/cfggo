package cfggo

import (
	"encoding"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type dynamicVar struct {
	config *Structure
	name   string
	want   reflect.Type
}

func (d *dynamicVar) Set(s string) error {
	var value = reflect.New(d.want).Elem()
	switch d.want.Kind() {
	case reflect.String:
		value.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if _, err := fmt.Sscan(s, value.Addr().Interface()); err != nil {
			return err
		}
	case reflect.Slice:
		split := strings.Split(s, ",")
		value.Set(reflect.MakeSlice(d.want, len(split), len(split)))
		for i, v := range split {
			if _, err := fmt.Sscan(v, value.Index(i).Addr().Interface()); err != nil {
				return err
			}
		}
	case reflect.Map:
		split := strings.Split(s, ",")
		value.Set(reflect.MakeMap(d.want))
		for _, v := range split {
			kv := strings.Split(v, ":")
			if len(kv) != 2 {
				return fmt.Errorf("invalid map value %s", v)
			}
			key := reflect.New(d.want.Key()).Elem()
			if _, err := fmt.Sscan(kv[0], key.Addr().Interface()); err != nil {
				return err
			}
			val := reflect.New(d.want.Elem()).Elem()
			if _, err := fmt.Sscan(kv[1], val.Addr().Interface()); err != nil {
				return err
			}
			value.SetMapIndex(key, val)
		}
	default:
		unmarshaler, ok := value.Addr().Interface().(encoding.TextUnmarshaler)
		if ok {
			if err := unmarshaler.UnmarshalText([]byte(s)); err != nil {
				return err
			}
			return d.config.Set(d.name, value.Interface())
		}
		jsonUnmarshaler, ok := value.Addr().Interface().(json.Unmarshaler)
		if ok {
			if err := jsonUnmarshaler.UnmarshalJSON([]byte(strconv.Quote(s))); err != nil {
				return err
			}
		}
		return fmt.Errorf("unsupported type %s", d.want)
	}
	if err := d.config.Set(d.name, value.Interface()); err != nil {
		return err
	}
	//fmt.Println("Set", d.name, "to", value.Interface())
	return nil
}

func (d *dynamicVar) String() string {
	if d.config == nil {
		return ""
	}
	val, ok := d.config.Get(d.name)
	if !ok {
		return ""
	}
	return fmt.Sprint(val)
}

// NewVar creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewVar(configVarName string, defaultValue interface{}, configDescription string) {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}
	//c.configData[configVarName] = defaultValue
	if flag.Lookup(configVarName) != nil {
		Logger.Error("Flag %s is already set, skipping...\n", configVarName)
		return
	}
	flag.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(c.configData[configVarName])}, configVarName, configDescription)
}
