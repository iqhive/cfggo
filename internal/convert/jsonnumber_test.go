package convert

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestNormalizeJSONNumbers(t *testing.T) {
	tests := []struct {
		in   interface{}
		want interface{}
	}{
		{json.Number("7"), float64(7)},
		{json.Number("-7"), float64(-7)},
		{json.Number("9007199254740992"), float64(9007199254740992)},
		{json.Number("9007199254740993"), int64(9007199254740993)},
		{json.Number("-9007199254740993"), int64(-9007199254740993)},
		{json.Number("9223372036854775807"), int64(math.MaxInt64)},
		{json.Number("18446744073709551615"), uint64(math.MaxUint64)},
		{json.Number("1e3"), float64(1000)},
		{json.Number("0.5"), float64(0.5)},
		{"text", "text"},
		{true, true},
		{nil, nil},
		{
			map[string]interface{}{"a": json.Number("9007199254740993"), "b": []interface{}{json.Number("1"), "x"}},
			map[string]interface{}{"a": int64(9007199254740993), "b": []interface{}{float64(1), "x"}},
		},
	}
	for _, tt := range tests {
		got, err := NormalizeJSONNumbers(tt.in)
		if err != nil {
			t.Errorf("NormalizeJSONNumbers(%#v) error = %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("NormalizeJSONNumbers(%#v) = %#v (%T), want %#v (%T)", tt.in, got, got, tt.want, tt.want)
		}
	}

	if _, err := NormalizeJSONNumbers(json.Number("1e400")); err == nil {
		t.Error("NormalizeJSONNumbers(1e400) succeeded, want a range error")
	}
	if _, err := NormalizeJSONNumbers([]interface{}{json.Number("1e400")}); err == nil {
		t.Error("NormalizeJSONNumbers([1e400]) succeeded, want a range error")
	}
}

func TestConvertValueJSONNumberIsExact(t *testing.T) {
	big := json.Number("9007199254740993")

	if got, err := ConvertValue(big, reflect.TypeOf(int64(0)), nil); err != nil || got != int64(9007199254740993) {
		t.Errorf("int64: got %#v, err %v", got, err)
	}
	if got, err := ConvertValue(json.Number("18446744073709551615"), reflect.TypeOf(uint64(0)), nil); err != nil || got != uint64(math.MaxUint64) {
		t.Errorf("uint64: got %#v, err %v", got, err)
	}
	if got, err := ConvertValue(json.Number("1500000000"), reflect.TypeOf(time.Duration(0)), nil); err != nil || got != 1500*time.Millisecond {
		t.Errorf("duration: got %#v, err %v", got, err)
	}
	if got, err := ConvertValue(json.Number("1e3"), reflect.TypeOf(int(0)), nil); err != nil || got != 1000 {
		t.Errorf("int from exponent: got %#v, err %v", got, err)
	}
	if got, err := ConvertValue(json.Number("2.5"), reflect.TypeOf(float32(0)), nil); err != nil || got != float32(2.5) {
		t.Errorf("float32: got %#v, err %v", got, err)
	}
	if got, err := ConvertValue(big, reflect.TypeOf((*interface{})(nil)).Elem(), nil); err != nil || got != int64(9007199254740993) {
		t.Errorf("interface{}: got %#v (%T), err %v", got, got, err)
	}
	if got, err := ConvertValue(json.Number("7"), reflect.TypeOf((*interface{})(nil)).Elem(), nil); err != nil || got != float64(7) {
		t.Errorf("interface{} small: got %#v (%T), err %v", got, got, err)
	}
	if _, err := ConvertValue(json.Number("1e30"), reflect.TypeOf(int64(0)), nil); err == nil {
		t.Error("int64 from 1e30 succeeded, want overflow error")
	}
	if _, err := ConvertValue(json.Number("-1"), reflect.TypeOf(uint(0)), nil); err == nil {
		t.Error("uint from -1 succeeded, want underflow error")
	}
	if got, err := ConvertValue(json.Number("8080"), reflect.TypeOf(""), nil); err != nil || got != "8080" {
		t.Errorf("string from a JSON number: got %#v, err %v (want the literal text)", got, err)
	}
	// A JSON number nested in a container reaches a typed element exactly
	got, err := ConvertValue([]interface{}{big}, reflect.TypeOf([]int64(nil)), nil)
	if err != nil || !reflect.DeepEqual(got, []int64{9007199254740993}) {
		t.Errorf("[]int64: got %#v, err %v", got, err)
	}
}

type namedString string

func TestConvertValueNamedStringSource(t *testing.T) {
	if got, err := ConvertValue(namedString("info"), reflect.TypeOf(""), nil); err != nil || got != "info" {
		t.Errorf("named string to string: got %#v, err %v", got, err)
	}
	if got, err := ConvertValue(namedString("8"), reflect.TypeOf(int(0)), nil); err != nil || got != 8 {
		t.Errorf("named string to int: got %#v, err %v", got, err)
	}
}

func TestConvertValueScalarsToStringUseLiteralText(t *testing.T) {
	tests := []struct {
		in   interface{}
		want string
	}{
		{65, "65"}, // never the rune "A"
		{int64(-7), "-7"},
		{uint8(200), "200"},
		{2.5, "2.5"},
		{float64(1), "1"},
		{true, "true"},
		{json.Number("1.0"), "1.0"}, // exact literal kept
		{json.Number("1e3"), "1e3"},
	}
	for _, tt := range tests {
		got, err := ConvertValue(tt.in, reflect.TypeOf(""), nil)
		if err != nil || got != tt.want {
			t.Errorf("ConvertValue(%#v, string) = %#v, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	if got, err := ConvertValue(int64(65), reflect.TypeOf(namedString("")), nil); err != nil || got != namedString("65") {
		t.Errorf("ConvertValue(int64(65), namedString) = %#v (%T), err %v", got, got, err)
	}
	if _, err := ConvertValue([]int{1}, reflect.TypeOf(""), nil); err == nil {
		t.Error("a slice was coerced into a string")
	}
}

func TestConvertStringInfersInterfaceValues(t *testing.T) {
	target := reflect.TypeOf((*interface{})(nil)).Elem()
	tests := []struct {
		in   string
		want interface{}
	}{
		{"42", 42},
		{"-7", -7},
		{"9007199254740993", 9007199254740993},
		{"2.5", 2.5},
		{"1e3", 1000},
		{"true", true},
		{"false", false},
		{"[1,2]", []interface{}{float64(1), float64(2)}},
		{`{"a":"b"}`, map[string]interface{}{"a": "b"}},
		{"[not json", "[not json"},
		{"hello", "hello"},
		{"", ""},
	}
	for _, tt := range tests {
		got, err := ConvertString(tt.in, target, nil)
		if err != nil {
			t.Errorf("ConvertString(%q) error = %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ConvertString(%q) = %#v (%T), want %#v (%T)", tt.in, got, got, tt.want, tt.want)
		}
	}
}

type namedBool bool

func TestConvertStringNamedBoolAndByteSlices(t *testing.T) {
	if got, err := ConvertString("yes", reflect.TypeOf(namedBool(false)), nil); err != nil || got != namedBool(true) {
		t.Errorf("named bool: got %#v (%T), err %v", got, got, err)
	}
	if got, err := ConvertString("", reflect.TypeOf(namedBool(false)), nil); err != nil || got != namedBool(false) {
		t.Errorf("named bool empty: got %#v (%T), err %v", got, got, err)
	}
	got, err := ConvertString("true,f", reflect.TypeOf([]namedBool(nil)), nil)
	if err != nil || !reflect.DeepEqual(got, []namedBool{true, false}) {
		t.Errorf("[]namedBool: got %#v, err %v", got, err)
	}
	gotMap, err := ConvertString("a:1,b:no", reflect.TypeOf(map[string]namedBool(nil)), nil)
	if err != nil || !reflect.DeepEqual(gotMap, map[string]namedBool{"a": true, "b": false}) {
		t.Errorf("map[string]namedBool: got %#v, err %v", gotMap, err)
	}

	bytesType := reflect.TypeOf([]byte(nil))
	if got, err := ConvertString("aGVsbG8=", bytesType, nil); err != nil || string(got.([]byte)) != "hello" {
		t.Errorf("padded base64: got %#v, err %v", got, err)
	}
	if got, err := ConvertString("aGVsbG8", bytesType, nil); err != nil || string(got.([]byte)) != "hello" {
		t.Errorf("unpadded base64: got %#v, err %v", got, err)
	}
	if got, err := ConvertString("[104,105]", bytesType, nil); err != nil || string(got.([]byte)) != "hi" {
		t.Errorf("JSON array: got %#v, err %v", got, err)
	}
	if got, err := ConvertString("", bytesType, nil); err != nil || len(got.([]byte)) != 0 {
		t.Errorf("empty: got %#v, err %v", got, err)
	}
	if _, err := ConvertString("not base64!", bytesType, nil); err == nil {
		t.Error("raw text was accepted for []byte")
	}
	type key []byte
	if got, err := ConvertString("aGVsbG8=", reflect.TypeOf(key(nil)), nil); err != nil || string(got.(key)) != "hello" {
		t.Errorf("named byte slice: got %#v (%T), err %v", got, got, err)
	}
}
