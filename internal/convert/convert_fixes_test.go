package convert_test

import (
	"encoding"
	"fmt"
	"log/slog"
	"math"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iqhive/cfggo/internal/convert"
)

// A float that is NaN, infinite, fractional or outside int64 must not reach a
// time.Duration as whatever int64(f) happens to produce (MinInt64 on amd64)
func TestConvertValueFloatToDurationIsRangeChecked(t *testing.T) {
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1.5, 1e19, -1e19} {
		got, err := convert.ConvertValue(f, typeDuration, nil)
		if err == nil {
			t.Errorf("ConvertValue(%v, time.Duration) = %v, want error", f, got)
		}
	}
	if got, err := convert.ConvertValue(float32(1e9), typeDuration, nil); err != nil || got != time.Second {
		t.Errorf("ConvertValue(float32(1e9), time.Duration) = %v, %v; want 1s", got, err)
	}
	if got, err := convert.ConvertValue(float64(-2.5e9), typeDuration, nil); err != nil || got != -2500*time.Millisecond {
		t.Errorf("ConvertValue(-2.5e9, time.Duration) = %v, %v; want -2.5s", got, err)
	}
}

type namedByte byte

// base64 text must decode into a slice whose element is a named byte type;
// reflect.Copy panics when the element types differ
func TestConvertStringBase64IntoNamedByteElementSlice(t *testing.T) {
	target := reflect.TypeOf([]namedByte(nil))
	got, err := convert.ConvertString("aGVsbG8=", target, nil)
	if err != nil {
		t.Fatalf("ConvertString(base64, []namedByte) error = %v", err)
	}
	want := []namedByte{'h', 'e', 'l', 'l', 'o'}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConvertString(base64, []namedByte) = %#v (%T), want %#v", got, got, want)
	}
	if got, err := convert.ConvertString("", target, nil); err != nil || !reflect.DeepEqual(got, []namedByte{}) {
		t.Errorf("ConvertString(\"\", []namedByte) = %#v, %v; want empty slice", got, err)
	}
}

// reflect.Convert accepts a slice to array conversion but panics when the
// slice is shorter than the array. Any length mismatch is an error
func TestConvertValueSliceToArrayNeverPanics(t *testing.T) {
	arrayType := reflect.TypeOf([3]int{})
	ptrType := reflect.TypeOf((*[3]int)(nil))

	for _, short := range []interface{}{[]int{1, 2}, []int{}, []interface{}{float64(1)}} {
		if got, err := convert.ConvertValue(short, arrayType, nil); err == nil {
			t.Errorf("ConvertValue(%v, [3]int) = %v, want length error", short, got)
		}
		if got, err := convert.ConvertValue(short, ptrType, nil); err == nil {
			t.Errorf("ConvertValue(%v, *[3]int) = %v, want length error", short, got)
		}
	}
	if got, err := convert.ConvertValue([]int{1, 2, 3, 4}, arrayType, nil); err == nil {
		t.Errorf("ConvertValue([1 2 3 4], [3]int) = %v, want length error (not silent truncation)", got)
	}
	if got, err := convert.ConvertValue([]int{1, 2, 3}, arrayType, nil); err != nil || got != [3]int{1, 2, 3} {
		t.Errorf("ConvertValue([1 2 3], [3]int) = %v, %v; want [1 2 3]", got, err)
	}
	// Elements decoded from JSON are converted individually
	if got, err := convert.ConvertValue([]interface{}{float64(1), "2", float64(3)}, arrayType, nil); err != nil || got != [3]int{1, 2, 3} {
		t.Errorf("ConvertValue([]interface{}, [3]int) = %v, %v; want [1 2 3]", got, err)
	}
	if _, err := convert.ConvertValue([]interface{}{"a", "b", "c"}, arrayType, nil); err == nil || !strings.Contains(err.Error(), "array[0]") {
		t.Errorf("ConvertValue([a b c], [3]int) error = %v, want element error", err)
	}
	got, err := convert.ConvertValue([]int{1, 2, 3}, ptrType, nil)
	if err != nil {
		t.Fatalf("ConvertValue([1 2 3], *[3]int) error = %v", err)
	}
	if p, ok := got.(*[3]int); !ok || *p != [3]int{1, 2, 3} {
		t.Errorf("ConvertValue([1 2 3], *[3]int) = %#v, want &[1 2 3]", got)
	}
}

// JSON array text is decoded with exact numbers and converted element by
// element into the target; malformed JSON is an error, never a CSV fallback
func TestConvertStringJSONArrayConvertsElements(t *testing.T) {
	cases := []struct {
		name   string
		s      string
		target reflect.Type
		want   interface{}
	}{
		{"strings into []int", `["1","2"]`, typeIntSlice, []int{1, 2}},
		{"numbers into []string", `["a",1,2.5,true]`, typeStrSlice, []string{"a", "1", "2.5", "true"}},
		{"big integer stays exact in []interface{}", `[9007199254740993, 1]`, typeAnySlice, []interface{}{int64(9007199254740993), float64(1)}},
		{"big integer into []int64", `[9007199254740993]`, reflect.TypeOf([]int64(nil)), []int64{9007199254740993}},
		{"bool words into []bool", `["yes","n"]`, typeBoolSlice, []bool{true, false}},
		{"durations into []time.Duration", `["1h", 1000000000]`, reflect.TypeOf([]time.Duration(nil)), []time.Duration{time.Hour, time.Second}},
		{"nested arrays", `[[1,2],[3]]`, reflect.TypeOf([][]int(nil)), [][]int{{1, 2}, {3}}},
		{"objects into []map", `[{"a":1}]`, reflect.TypeOf([]map[string]int(nil)), []map[string]int{{"a": 1}}},
		{"JSON numbers into []byte", `[104,105]`, reflect.TypeOf([]byte(nil)), []byte("hi")},
		{"empty array", `[]`, typeIntSlice, []int{}},
		{"trailing whitespace", "[1, 2] \n", typeIntSlice, []int{1, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertString(tc.s, tc.target, nil)
			if err != nil {
				t.Fatalf("ConvertString(%q, %v) error = %v", tc.s, tc.target, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ConvertString(%q, %v) = %#v, want %#v", tc.s, tc.target, got, tc.want)
			}
		})
	}

	errCases := []struct {
		name   string
		s      string
		target reflect.Type
		want   string
	}{
		{"malformed JSON is not CSV", `["a",`, typeStrSlice, "JSON array"},
		{"trailing data", `[1] 2`, typeIntSlice, "JSON array"},
		{"element that does not fit", `["a"]`, typeIntSlice, "slice[0]"},
		{"number beyond float64", `[1e400]`, typeAnySlice, "JSON array"},
		{"malformed JSON for []byte", `[1,`, reflect.TypeOf([]byte(nil)), "JSON array"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertString(tc.s, tc.target, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ConvertString(%q, %v) = %#v, err %v; want error containing %q", tc.s, tc.target, got, err, tc.want)
			}
		})
	}

	// The error wrapper sees the JSON failure
	wrapper := &wrappingErrorWrapper{}
	if _, err := convert.ConvertString(`[`, typeIntSlice, wrapper); err == nil || !wrapper.called {
		t.Errorf("malformed JSON array: err %v, wrapper called %v", err, wrapper.called)
	}

	// Text that does not start with "[" keeps the comma-separated parsing
	if got, err := convert.ConvertString("a,[b]", typeStrSlice, nil); err != nil || !reflect.DeepEqual(got, []string{"a", "[b]"}) {
		t.Errorf("ConvertString(\"a,[b]\") = %#v, %v; want CSV parsing", got, err)
	}
}

// JSON object text is decoded with exact numbers and converted entry by entry
// into the target; malformed JSON is an error, never a key:value fallback
func TestConvertStringJSONObjectConvertsEntries(t *testing.T) {
	cases := []struct {
		name   string
		s      string
		target reflect.Type
		want   interface{}
	}{
		{"strings into map[string]int", `{"a":"1","b":2}`, reflect.TypeOf(map[string]int(nil)), map[string]int{"a": 1, "b": 2}},
		{"numbers into map[string]string", `{"a":1,"b":true}`, typeStrMap, map[string]string{"a": "1", "b": "true"}},
		{"big integer stays exact in map[string]interface{}", `{"n":9007199254740993,"m":1}`, typeAnyMap, map[string]interface{}{"n": int64(9007199254740993), "m": float64(1)}},
		{"keys converted", `{"1":"10","2":20}`, typeIntMap, map[int]int{1: 10, 2: 20}},
		{"nested values", `{"a":{"b":"1"}}`, reflect.TypeOf(map[string]map[string]int(nil)), map[string]map[string]int{"a": {"b": 1}}},
		{"empty object", `{}`, typeStrMap, map[string]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertString(tc.s, tc.target, nil)
			if err != nil {
				t.Fatalf("ConvertString(%q, %v) error = %v", tc.s, tc.target, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ConvertString(%q, %v) = %#v, want %#v", tc.s, tc.target, got, tc.want)
			}
		})
	}

	errCases := []struct {
		name   string
		s      string
		target reflect.Type
		want   string
	}{
		{"malformed JSON is not key:value", `{"a":`, typeStrMap, "JSON object"},
		{"trailing data", `{"a":"b"} x`, typeStrMap, "JSON object"},
		{"value that does not fit", `{"a":"x"}`, reflect.TypeOf(map[string]int(nil)), "cannot parse int"},
		{"key that does not fit", `{"x":1}`, typeIntMap, "cannot parse int"},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertString(tc.s, tc.target, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ConvertString(%q, %v) = %#v, err %v; want error containing %q", tc.s, tc.target, got, err, tc.want)
			}
		})
	}

	// Text that does not start with "{" keeps the key:value parsing
	if got, err := convert.ConvertString("a:{b}", typeStrMap, nil); err != nil || !reflect.DeepEqual(got, map[string]string{"a": "{b}"}) {
		t.Errorf("ConvertString(\"a:{b}\") = %#v, %v; want key:value parsing", got, err)
	}
}

// strconv.ParseFloat accepts NaN and infinity spellings, which json.Marshal
// later refuses; they are rejected for float targets and left as text for an
// interface{} target
func TestConvertStringRejectsNonFiniteFloats(t *testing.T) {
	anyType := reflect.TypeOf((*interface{})(nil)).Elem()
	for _, s := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "inf", "infinity", "-Infinity", "1e400"} {
		for _, target := range []reflect.Type{typeFloat32, typeFloat64} {
			if got, err := convert.ConvertString(s, target, nil); err == nil {
				t.Errorf("ConvertString(%q, %v) = %v, want error", s, target, got)
			}
		}
		if got, err := convert.ConvertString(s, anyType, nil); err != nil || got != s {
			t.Errorf("ConvertString(%q, interface{}) = %#v (%T), %v; want the text unchanged", s, got, got, err)
		}
	}
	for _, s := range []string{"1.5", "-0", "1e3", "3.4e38"} {
		if _, err := convert.ConvertString(s, typeFloat64, nil); err != nil {
			t.Errorf("ConvertString(%q, float64) error = %v", s, err)
		}
	}
	if got, err := convert.ConvertString("2.5", anyType, nil); err != nil || got != 2.5 {
		t.Errorf("ConvertString(\"2.5\", interface{}) = %#v, %v; want 2.5", got, err)
	}
	if _, err := convert.ConvertValue("Inf", typeFloat64, nil); err == nil {
		t.Error("ConvertValue(\"Inf\", float64) succeeded, want error")
	}
}

// A non-empty interface target that itself declares UnmarshalText has no
// concrete value to unmarshal into; it must fail cleanly rather than panic
func TestConvertStringInterfaceTargetWithUnmarshalTextDoesNotPanic(t *testing.T) {
	target := reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	if got, err := convert.ConvertString("abc", target, nil); err == nil {
		t.Errorf("ConvertString(\"abc\", encoding.TextUnmarshaler) = %#v, want error", got)
	}
	if got, err := convert.ConvertValue("abc", target, nil); err == nil {
		t.Errorf("ConvertValue(\"abc\", encoding.TextUnmarshaler) = %#v, want error", got)
	}
	// A pointer to such an interface has no methods either
	if got, err := convert.ConvertString("abc", reflect.PointerTo(target), nil); err == nil {
		t.Errorf("ConvertString(\"abc\", *encoding.TextUnmarshaler) = %#v, want error", got)
	}
}

// colour is a named string whose UnmarshalText validates and normalises
type colour string

func (c *colour) UnmarshalText(b []byte) error {
	switch s := strings.ToLower(string(b)); s {
	case "red", "green", "blue":
		*c = colour(s)
		return nil
	}
	return fmt.Errorf("unknown colour %q", b)
}

// A named type whose underlying kind is a builtin still parses through its
// own UnmarshalText rather than as that kind
func TestConvertStringPrefersTextUnmarshalerOverBuiltinKind(t *testing.T) {
	ipType := reflect.TypeOf(net.IP(nil))
	levelType := reflect.TypeOf(slog.Level(0))
	colourType := reflect.TypeOf(colour(""))

	// net.IP is a []byte kind: previously base64-decoded (or rejected)
	got, err := convert.ConvertString("192.0.2.1", ipType, nil)
	if err != nil {
		t.Fatalf("ConvertString(\"192.0.2.1\", net.IP) error = %v", err)
	}
	if ip, ok := got.(net.IP); !ok || !ip.Equal(net.ParseIP("192.0.2.1")) {
		t.Fatalf("ConvertString(\"192.0.2.1\", net.IP) = %#v (%T)", got, got)
	}
	if _, err := convert.ConvertString("not-an-ip", ipType, nil); err == nil {
		t.Error("ConvertString(\"not-an-ip\", net.IP) succeeded, want error")
	}

	// slog.Level is an int kind: previously "info" failed to parse as an integer
	levels := map[string]slog.Level{"info": slog.LevelInfo, "DEBUG": slog.LevelDebug, "warn+2": slog.LevelWarn + 2, "ERROR-1": slog.LevelError - 1}
	for s, want := range levels {
		if got, err := convert.ConvertString(s, levelType, nil); err != nil || got != want {
			t.Errorf("ConvertString(%q, slog.Level) = %v, %v; want %v", s, got, err, want)
		}
	}
	if _, err := convert.ConvertString("loud", levelType, nil); err == nil {
		t.Error("ConvertString(\"loud\", slog.Level) succeeded, want error")
	}
	// A numeric source still converts as a number
	if got, err := convert.ConvertValue(float64(4), levelType, nil); err != nil || got != slog.Level(4) {
		t.Errorf("ConvertValue(4, slog.Level) = %v, %v; want 4", got, err)
	}

	// A named string with UnmarshalText: previously a plain string conversion
	if got, err := convert.ConvertString("Red", colourType, nil); err != nil || got != colour("red") {
		t.Errorf("ConvertString(\"Red\", colour) = %#v, %v; want \"red\"", got, err)
	}
	if _, err := convert.ConvertString("mauve", colourType, nil); err == nil {
		t.Error("ConvertString(\"mauve\", colour) succeeded, want error")
	}

	// Pointers to such types allocate a value
	if got, err := convert.ConvertString("10.0.0.1", reflect.PointerTo(ipType), nil); err != nil || !got.(*net.IP).Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("ConvertString(\"10.0.0.1\", *net.IP) = %#v, %v", got, err)
	}
	if got, err := convert.ConvertString("green", reflect.PointerTo(colourType), nil); err != nil || *got.(*colour) != "green" {
		t.Errorf("ConvertString(\"green\", *colour) = %#v, %v", got, err)
	}

	// Elements of slices and maps, from comma-separated text and from JSON
	ips := []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2")}
	if got, err := convert.ConvertString("10.0.0.1, 10.0.0.2", reflect.TypeOf([]net.IP(nil)), nil); err != nil || !reflect.DeepEqual(got, ips) {
		t.Errorf("ConvertString(csv, []net.IP) = %#v, %v", got, err)
	}
	if got, err := convert.ConvertString(`["10.0.0.1","10.0.0.2"]`, reflect.TypeOf([]net.IP(nil)), nil); err != nil || !reflect.DeepEqual(got, ips) {
		t.Errorf("ConvertString(json, []net.IP) = %#v, %v", got, err)
	}
	wantLevels := map[string]slog.Level{"a": slog.LevelDebug, "b": slog.LevelError}
	if got, err := convert.ConvertString("a:debug,b:error", reflect.TypeOf(map[string]slog.Level(nil)), nil); err != nil || !reflect.DeepEqual(got, wantLevels) {
		t.Errorf("ConvertString(kv, map[string]slog.Level) = %#v, %v", got, err)
	}
	if got, err := convert.ConvertString(`{"a":"debug","b":8}`, reflect.TypeOf(map[string]slog.Level(nil)), nil); err != nil || !reflect.DeepEqual(got, wantLevels) {
		t.Errorf("ConvertString(json, map[string]slog.Level) = %#v, %v", got, err)
	}

	// ConvertValue with a string source takes the same route
	if got, err := convert.ConvertValue("info", levelType, nil); err != nil || got != slog.LevelInfo {
		t.Errorf("ConvertValue(\"info\", slog.Level) = %v, %v", got, err)
	}

	// Builtin kinds and the time types are unchanged
	if got, err := convert.ConvertString("1.2.3.4", typeString, nil); err != nil || got != "1.2.3.4" {
		t.Errorf("ConvertString(\"1.2.3.4\", string) = %#v, %v", got, err)
	}
	if got, err := convert.ConvertString("7", typeInt, nil); err != nil || got != 7 {
		t.Errorf("ConvertString(\"7\", int) = %#v, %v", got, err)
	}
	if got, err := convert.ConvertString("1h", typeDuration, nil); err != nil || got != time.Hour {
		t.Errorf("ConvertString(\"1h\", time.Duration) = %#v, %v", got, err)
	}
	if _, err := convert.ConvertString("2023-10-01 15:04:05", typeTime, nil); err == nil {
		t.Error("ConvertString(non-RFC3339, time.Time) succeeded, want RFC 3339 parse error")
	}
	type plainKey []byte
	if got, err := convert.ConvertString("aGVsbG8=", reflect.TypeOf(plainKey(nil)), nil); err != nil || string(got.(plainKey)) != "hello" {
		t.Errorf("ConvertString(base64, plainKey) = %#v, %v; a byte slice without UnmarshalText is still base64", got, err)
	}
}
