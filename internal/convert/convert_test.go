package convert_test

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iqhive/cfggo/internal/convert"
)

var (
	typeBool     = reflect.TypeOf(true)
	typeString   = reflect.TypeOf("")
	typeInt      = reflect.TypeOf(int(0))
	typeInt64    = reflect.TypeOf(int64(0))
	typeUint     = reflect.TypeOf(uint(0))
	typeUint8    = reflect.TypeOf(uint8(0))
	typeFloat32  = reflect.TypeOf(float32(0))
	typeFloat64  = reflect.TypeOf(float64(0))
	typeDuration = reflect.TypeOf(time.Duration(0))
	typeTime     = reflect.TypeOf(time.Time{})
	typeAnySlice = reflect.TypeOf([]interface{}{})
	typeStrSlice = reflect.TypeOf([]string{})
	typeIntSlice = reflect.TypeOf([]int{})
	typeIntMap   = reflect.TypeOf(map[int]int{})
	typeAnyMap   = reflect.TypeOf(map[string]interface{}{})
	typeStrMap   = reflect.TypeOf(map[string]string{})
	typeTextType = reflect.TypeOf(textType{})
	typeTextPtr  = reflect.TypeOf((*textType)(nil))
)

// textType implements encoding.TextUnmarshaler via a pointer receiver.
type textType struct{ V int }

func (t *textType) UnmarshalText(b []byte) error {
	v := 0
	for _, c := range b {
		v = v*10 + int(c-'0')
	}
	t.V = v
	return nil
}

type valueTextType string

func (valueTextType) UnmarshalText([]byte) error {
	return nil
}

type wrappingErrorWrapper struct {
	called bool
}

func (w *wrappingErrorWrapper) WrapError(err error, code int, msg string, args ...interface{}) error {
	w.called = true
	if msg == "" {
		return fmt.Errorf("wrapped[%d]: %w", code, err)
	}
	return fmt.Errorf("wrapped[%d]: "+msg, append([]interface{}{code}, args...)...)
}

func TestConvertString(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		target  reflect.Type
		want    interface{}
		wantErr bool
	}{
		// bool - standard strconv values
		{"bool true", "true", typeBool, true, false},
		{"bool false", "false", typeBool, false, false},
		// bool - extended words
		{"bool y", "y", typeBool, true, false},
		{"bool Y", "Y", typeBool, true, false},
		{"bool yes", "yes", typeBool, true, false},
		{"bool 1", "1", typeBool, true, false},
		{"bool n", "n", typeBool, false, false},
		{"bool N", "N", typeBool, false, false},
		{"bool no", "no", typeBool, false, false},
		{"bool 0", "0", typeBool, false, false},
		{"bool empty string", "", typeBool, false, false},
		{"bool bad", "abc", typeBool, nil, true},
		// string
		{"string", "hello", typeString, "hello", false},
		{"string empty", "", typeString, "", false},
		// int
		{"int positive", "42", typeInt, 42, false},
		{"int negative", "-7", typeInt, -7, false},
		{"int bad", "abc", typeInt, nil, true},
		// int64
		{"int64", "43", typeInt64, int64(43), false},
		// uint
		{"uint", "10", typeUint, uint(10), false},
		{"uint bad", "abc", typeUint, nil, true},
		// float32
		{"float32", "3.14", typeFloat32, float32(3.14), false},
		{"float32 bad", "abc", typeFloat32, nil, true},
		// float64
		{"float64", "2.718", typeFloat64, float64(2.718), false},
		// time.Duration
		{"duration 1h30m", "1h30m", typeDuration, time.Hour + 30*time.Minute, false},
		{"duration 2s", "2s", typeDuration, 2 * time.Second, false},
		{"duration bad", "abc", typeDuration, nil, true},
		// time.Time
		{"time RFC3339", "2023-10-01T15:04:05Z", typeTime, time.Date(2023, 10, 1, 15, 4, 5, 0, time.UTC), false},
		{"time bad", "notadate", typeTime, nil, true},
		// []string
		{"[]string csv", "a,b,c", typeStrSlice, []string{"a", "b", "c"}, false},
		{"[]string json", `["x","y","z"]`, typeStrSlice, []string{"x", "y", "z"}, false},
		{"[]string empty", "", typeStrSlice, []string{}, false},
		// []int
		{"[]int csv", "1,2,3", typeIntSlice, []int{1, 2, 3}, false},
		{"[]int bad element", "1,abc,3", typeIntSlice, nil, true},
		// map[int]int
		{"map[int]int ok", "1:10,2:20,3:30", typeIntMap, map[int]int{1: 10, 2: 20, 3: 30}, false},
		{"map[int]int bad val", "1:abc", typeIntMap, nil, true},
		{"map[int]int bad format", "1:10,invalid", typeIntMap, nil, true},
		// TextUnmarshaler (pointer receiver)
		{"TextUnmarshaler", "42", typeTextType, textType{V: 42}, false},
		{"pointer TextUnmarshaler", "42", typeTextPtr, &textType{V: 42}, false},
		{"value TextUnmarshaler", "ignored", reflect.TypeOf(valueTextType("")), valueTextType("ignored"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertString(tc.s, tc.target, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ConvertString(%q, %v) error = %v, wantErr %v", tc.s, tc.target, err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

func TestConvertValueEdgeCases(t *testing.T) {
	type jsonTarget struct {
		N int `json:"n"`
	}

	cases := []struct {
		name    string
		value   interface{}
		target  reflect.Type
		want    interface{}
		wantErr string
	}{
		{
			name:   "nil target returns original value",
			value:  map[string]int{"answer": 42},
			target: nil,
			want:   map[string]int{"answer": 42},
		},
		{
			name:   "empty interface target accepts any value",
			value:  []int{1, 2, 3},
			target: reflect.TypeOf((*interface{})(nil)).Elem(),
			want:   []int{1, 2, 3},
		},
		{
			name:   "json fallback converts map to struct",
			value:  map[string]interface{}{"n": float64(7)},
			target: reflect.TypeOf(jsonTarget{}),
			want:   jsonTarget{N: 7},
		},
		{
			name:    "json marshal failure is reported",
			value:   func() {},
			target:  typeString,
			wantErr: "cannot marshal",
		},
		{
			name:   "float to uint64 exercises wide unsigned bounds",
			value:  float64(42),
			target: reflect.TypeOf(uint64(0)),
			want:   uint64(42),
		},
		{
			name:    "infinite float to uint is rejected",
			value:   math.Inf(1),
			target:  typeUint,
			wantErr: "lossy numeric conversion",
		},
		{
			name:    "map key conversion error is returned",
			value:   map[interface{}]interface{}{"bad": "1"},
			target:  typeIntMap,
			wantErr: "cannot parse int",
		},
		{
			name:    "map value conversion error is returned",
			value:   map[string]interface{}{"1": "bad"},
			target:  typeIntMap,
			wantErr: "cannot parse int",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertValue(tc.value, tc.target, nil)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ConvertValue() error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConvertValue() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ConvertValue() = %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

func TestConvertStringUsesErrorWrapper(t *testing.T) {
	wrapper := &wrappingErrorWrapper{}
	_, err := convert.ConvertString("not-an-int", typeInt, wrapper)
	if err == nil {
		t.Fatal("ConvertString() expected wrapped error, got nil")
	}
	if !wrapper.called {
		t.Fatal("ConvertString() did not call ErrorWrapper")
	}
	if got := err.Error(); !strings.Contains(got, "wrapped[400]") || !strings.Contains(got, "cannot parse int") {
		t.Fatalf("wrapped error = %q, want code and parse message", got)
	}
}

func TestConvertValue(t *testing.T) {
	cases := []struct {
		name    string
		value   interface{}
		target  reflect.Type
		want    interface{}
		wantErr bool
	}{
		{"nil to int zero", nil, typeInt, 0, false},
		{"same type", 42, typeInt, 42, false},
		{"float64 to int", float64(7), typeInt, 7, false},
		{"float64 fractional to int", float64(7.5), typeInt, nil, true},
		{"negative int to uint", int(-1), typeUint, nil, true},
		{"overflowing uint", uint64(300), typeUint8, nil, true},
		{"int to float64", int(3), typeFloat64, float64(3), false},
		{"int64 to Duration", int64(1e9), typeDuration, time.Duration(1e9), false},
		{"float64 to Duration nanoseconds", float64(5e8), typeDuration, time.Duration(5e8), false},
		{"string to int", "99", typeInt, 99, false},
		{"string to bool y", "y", typeBool, true, false},
		{"string to time.Time", "2023-10-01T15:04:05Z", typeTime, time.Date(2023, 10, 1, 15, 4, 5, 0, time.UTC), false},
		{"string to Duration", "1h", typeDuration, time.Hour, false},
		// Slice coercions from JSON unmarshal output.
		{"[]interface{} to []string", []interface{}{"a", "b"}, typeStrSlice, []string{"a", "b"}, false},
		{"[]interface{} to []int", []interface{}{float64(1), float64(2)}, typeIntSlice, []int{1, 2}, false},
		{"[]interface{} preserves nil", []interface{}{"a", nil}, typeAnySlice, []interface{}{"a", nil}, false},
		// Map coercions from JSON unmarshal output.
		{"map[string]interface{} to map[string]string", map[string]interface{}{"k": "v"}, typeStrMap, map[string]string{"k": "v"}, false},
		{"map[string]interface{} preserves nil", map[string]interface{}{"k": nil}, typeAnyMap, map[string]interface{}{"k": nil}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertValue(tc.value, tc.target, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ConvertValue(%v -> %v) error = %v, wantErr %v", tc.value, tc.target, err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}
