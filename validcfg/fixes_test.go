package validcfg

import (
	"math"
	"strings"
	"testing"
)

func TestRangeRejectsNaN(t *testing.T) {
	err := Range(0, 10)(math.NaN())
	if err == nil {
		t.Fatal("Range(0, 10)(NaN): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NaN") {
		t.Fatalf("Range(0, 10)(NaN) error = %q, want mention of NaN", err.Error())
	}
	if err := Range(0, 10)(float32(math.NaN())); err == nil {
		t.Fatal("Range(0, 10)(float32 NaN): expected error, got nil")
	}
	// Infinities are ordinary out-of-range values, not NaN
	if err := Range(0, 10)(math.Inf(1)); err == nil {
		t.Fatal("Range(0, 10)(+Inf): expected error, got nil")
	}
	if err := Range(math.Inf(-1), math.Inf(1))(5.0); err != nil {
		t.Fatalf("Range(-Inf, +Inf)(5): unexpected error %v", err)
	}
}

func TestRequiredRejectsTypedNil(t *testing.T) {
	type T struct{}

	nils := map[string]interface{}{
		"pointer": (*T)(nil),
		"func":    (func() *T)(nil),
		"chan":    (chan int)(nil),
		"map":     (map[string]int)(nil),
		"slice":   ([]int)(nil),
	}
	for name, value := range nils {
		t.Run(name, func(t *testing.T) {
			if err := Required()(value); err == nil {
				t.Fatalf("Required()(%#v): expected error for typed nil, got nil", value)
			}
		})
	}

	nonNils := map[string]interface{}{
		"pointer": &T{},
		"func":    func() *T { return nil },
		"chan":    make(chan int),
		"map":     map[string]int{"a": 1},
		"slice":   []int{1},
		"int":     0,
		"bool":    false,
	}
	for name, value := range nonNils {
		t.Run(name, func(t *testing.T) {
			if err := Required()(value); err != nil {
				t.Fatalf("Required()(%#v): unexpected error %v", value, err)
			}
		})
	}
}

func TestOneOfComparesNumbersAcrossTypes(t *testing.T) {
	const big = int64(9007199254740993) // 2^53 + 1: not representable as float64

	tests := []struct {
		name    string
		options []interface{}
		value   interface{}
		wantErr bool
	}{
		{"int64 matches untyped int option", []interface{}{1, 2, 3}, int64(2), false},
		{"uint16 matches untyped int option", []interface{}{1, 2, 3}, uint16(2), false},
		{"float64 matches untyped int option", []interface{}{1, 2, 3}, float64(2), false},
		{"float32 matches untyped int option", []interface{}{1, 2, 3}, float32(3), false},
		{"int matches float option", []interface{}{1.0, 2.0}, 2, false},
		{"uint8 matches int64 option", []interface{}{int64(2)}, uint8(2), false},
		{"fraction does not match int", []interface{}{1, 2, 3}, 2.5, true},
		{"int does not match fraction option", []interface{}{2.5}, 2, true},
		{"negative int does not match huge uint", []interface{}{uint64(math.MaxUint64)}, int64(-1), true},
		{"huge uint does not match negative int", []interface{}{int64(-1)}, uint64(math.MaxUint64), true},
		{"uint64 max matches uint64 max", []interface{}{uint64(math.MaxUint64)}, uint64(math.MaxUint64), false},
		{"integers beyond 2^53 compare exactly", []interface{}{big}, big - 1, true},
		{"integers beyond 2^53 match across signedness", []interface{}{big}, uint64(big), false},
		{"float 2^53 does not match int 2^53+1", []interface{}{float64(1 << 53)}, big, true},
		{"int 2^53+1 does not match float 2^53", []interface{}{big}, float64(1 << 53), true},
		{"float 2^53 matches int 2^53", []interface{}{float64(1 << 53)}, int64(1 << 53), false},
		{"NaN never matches", []interface{}{1, 2, math.NaN()}, math.NaN(), true},
		{"numeric string does not match number", []interface{}{1, 2, 3}, "2", true},
		{"number does not match numeric string", []interface{}{"1", "2"}, 2, true},
		{"bool is not numeric", []interface{}{1}, true, true},
		{"string still matches string", []interface{}{"dev", "prod"}, "prod", false},
		{"nil value does not match", []interface{}{1, 2}, nil, true},
		{"nil option matches nil value", []interface{}{nil}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := OneOf(tt.options...)(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("OneOf(%v)(%#v) error = %v, wantErr %v", tt.options, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestOneOfErrorMessageIsStable(t *testing.T) {
	validator := OneOf("dev", "prod")
	const want = "value must be one of [dev prod]"
	for _, value := range []interface{}{"test", 2, nil} {
		err := validator(value)
		if err == nil {
			t.Fatalf("OneOf(dev, prod)(%#v): expected error, got nil", value)
		}
		if got := err.Error(); got != want {
			t.Fatalf("OneOf(dev, prod)(%#v) error = %q, want %q", value, got, want)
		}
	}
	if got, want := OneOf(1, 2, 3)(int64(4)).Error(), "value must be one of [1 2 3]"; got != want {
		t.Fatalf("OneOf(1, 2, 3)(4) error = %q, want %q", got, want)
	}
}

func TestEmailTypeMismatchNamesEmailValidator(t *testing.T) {
	err := Email()(123)
	if err == nil {
		t.Fatal("Email()(123): expected error, got nil")
	}
	if got := err.Error(); !strings.HasPrefix(got, "Email validator") {
		t.Fatalf("Email()(123) error = %q, want it to name the Email validator", got)
	}
}
