package cfggo

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type dynamicVar struct {
	config *Structure
	name   string
	want   reflect.Type
}

func (d *dynamicVar) Set(s string) error {
	var value = reflect.New(d.want).Elem()
	switch d.want.Kind() {
	case reflect.Bool:
		if len(s) == 0 {
			value.SetBool(false)
		} else {
			switch s[0] {
			case '1', 't', 'T', 'y', 'Y':
				value.SetBool(true)
			case '0', 'f', 'F', 'n', 'N':
				value.SetBool(false)
			default:
				return fmt.Errorf("invalid bool value %s", s)
			}
		}
	case reflect.String:
		value.SetString(s)
	case reflect.TypeOf(time.Time{}).Kind():
		unmarshaler, ok := value.Addr().Interface().(encoding.TextUnmarshaler)
		if ok {
			if err := unmarshaler.UnmarshalText([]byte(s)); err != nil {
				return err
			}
			return d.config.Set(d.name, value.Interface())
		}
		jsonUnmarshaler, ok := value.Addr().Interface().(json.Unmarshaler)
		if ok {
			// if err := jsonUnmarshaler.UnmarshalJSON([]byte(strconv.Quote(s))); err != nil {
			// 	return err
			// }
			if err := jsonUnmarshaler.UnmarshalJSON([]byte(s)); err != nil {
				return err
			}
			return d.config.Set(d.name, value.Interface())
		}
		if value.Type().String() == "time.Time" {
			parsedTime, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(parsedTime))
		}
		return fmt.Errorf("Structs must support encoding.TextUnmarshaler or json.Unmarshaler")
	case reflect.Int64, reflect.TypeOf(time.Duration(0)).Kind():
		if d.want == reflect.TypeOf(int64(0)) {
			parsedInt, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return err
			}
			value.SetInt(parsedInt)
		} else if d.want == reflect.TypeOf(time.Duration(0)) {
			parsedDuration, err := time.ParseDuration(s)
			if err != nil {
				return err
			}
			value.Set(reflect.ValueOf(parsedDuration))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
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
	case reflect.Float32, reflect.Float64:
		if _, err := fmt.Sscan(s, value.Addr().Interface()); err != nil {
			return err
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
		return fmt.Errorf("unsupported type %s", d.want)
	}
	if err := d.config.Set(d.name, value.Interface()); err != nil {
		return err
	}
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
