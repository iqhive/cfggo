package cfggo

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"
)

type dynamicVarStruct struct {
	Structure
	Bool     func() bool
	String   func() string
	Int      func() int
	Int64    func() int64
	Float32  func() float32
	Float64  func() float64
	Time     func() time.Time
	Duration func() time.Duration
}

type dynamicVartestCase struct {
	name     string
	input    string
	wantType reflect.Type
	wantVal  interface{}
	wantErr  bool
}

type customTypeText struct {
	Value int
}

func (c *customTypeText) UnmarshalText(b []byte) error {
	var err error
	c.Value, err = strconv.Atoi(string(b))
	return err
}

type customTypeJSON struct {
	Value int
}

func (c *customTypeJSON) UnmarshalJSON(b []byte) error {
	var err error
	fmt.Println("|" + string(b) + "|")
	c.Value, err = strconv.Atoi(string(b))
	fmt.Println("|" + strconv.Itoa(c.Value) + "|")
	return err
}

var dynamicVartestCases = []dynamicVartestCase{
	{"Bool", "true", reflect.TypeOf(true), true, false},
	{"Bool", "false", reflect.TypeOf(true), false, false},
	{"Bool", "abc", reflect.TypeOf(true), false, true},
	{"Int", "42", reflect.TypeOf(int(0)), 42, false},
	{"Int", "abc", reflect.TypeOf(int(0)), 0, true},
	{"Int64", "43", reflect.TypeOf(int64(0)), int64(43), false},
	{"Int64", "abc", reflect.TypeOf(int64(0)), int64(0), true},
	{"Float32", "3.14", reflect.TypeOf(float32(0)), float32(3.14), false},
	{"Float32", "abc", reflect.TypeOf(float32(0)), float32(0), true},
	{"Float64", "2.718", reflect.TypeOf(float64(0)), float64(2.718), false},
	{"Float64", "abc", reflect.TypeOf(float64(0)), float64(0), true},
	{"Time", "2023-10-01T15:04:05Z", reflect.TypeOf(time.Time{}), time.Date(2023, 10, 1, 15, 4, 5, 0, time.UTC), false},
	{"Time", "abc", reflect.TypeOf(time.Time{}), time.Date(2023, 10, 1, 15, 4, 5, 0, time.UTC), true},
	{"Duration", "1h30m", reflect.TypeOf(time.Duration(0)), time.Hour + 30*time.Minute, false},
	{"Duration", "2s", reflect.TypeOf(time.Duration(0)), 2 * time.Second, false},
	{"Duration", "10ms", reflect.TypeOf(time.Duration(0)), 10 * time.Millisecond, false},
	{"Duration", "abc", reflect.TypeOf(time.Duration(0)), time.Second, true},
	{"String", "hello world", reflect.TypeOf(""), "hello world", false},
	{"String", "", reflect.TypeOf(""), "", false},
	{"Uint", "42", reflect.TypeOf(uint(0)), uint(42), false},
	{"Uint", "abc", reflect.TypeOf(uint(0)), uint(0), true},
	{"Uint64", "42", reflect.TypeOf(uint64(0)), uint64(42), false},
	{"Uint64", "abc", reflect.TypeOf(uint64(0)), uint64(0), true},
	{"IntSlice", "1,2,3", reflect.TypeOf([]int{}), []int{1, 2, 3}, false},
	{"IntSlice", "1,abc,3", reflect.TypeOf([]int{}), []int{}, true},
	{"StringSlice", "a,b,c", reflect.TypeOf([]string{}), []string{"a", "b", "c"}, false},
	{"IntMap", "1:10,2:20,3:30", reflect.TypeOf(map[int]int{}), map[int]int{1: 10, 2: 20, 3: 30}, false},
	{"IntMap", "1:10,invalid", reflect.TypeOf(map[int]int{}), map[int]int{}, true},
	{"IntMap", "1:abc", reflect.TypeOf(map[int]int{}), map[int]int{}, true},
	{"Bool", "y", reflect.TypeOf(true), true, false},
	{"Bool", "Y", reflect.TypeOf(true), true, false},
	{"Bool", "1", reflect.TypeOf(true), true, false},
	{"Bool", "n", reflect.TypeOf(true), false, false},
	{"Bool", "N", reflect.TypeOf(true), false, false},
	{"Bool", "0", reflect.TypeOf(true), false, false},
	{"Bool", "", reflect.TypeOf(true), false, false},
}

func TestDynamicVarSet(t *testing.T) {
	for _, tc := range dynamicVartestCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Structure{
				configData: make(map[string]interface{}),
			}
			dv := &dynamicVar{
				config: config,
				name:   tc.name,
				want:   tc.wantType,
			}

			err := dv.Set(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if !tc.wantErr {
				got, ok := config.Get(tc.name)
				if !ok {
					t.Errorf("Expected value for %s not found", tc.name)
					return
				}

				if !reflect.DeepEqual(got, tc.wantVal) {
					t.Errorf("Set() got = %v, want %v", got, tc.wantVal)
				}
			}
		})
	}
}

func TestDynamicVarString(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]interface{}
		varName  string
		wantType reflect.Type
		want     string
	}{
		{
			name:     "existing value",
			config:   map[string]interface{}{"test": "hello"},
			varName:  "test",
			wantType: reflect.TypeOf(""),
			want:     "hello",
		},
		{
			name:     "non-existing value",
			config:   map[string]interface{}{},
			varName:  "missing",
			wantType: reflect.TypeOf(""),
			want:     "",
		},
		{
			name:     "nil config",
			config:   nil,
			varName:  "test",
			wantType: reflect.TypeOf(""),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Structure{
				configData: tt.config,
			}
			dv := &dynamicVar{
				config: config,
				name:   tt.varName,
				want:   tt.wantType,
			}

			if got := dv.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDynamicVarSetCustomTypeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType reflect.Type
		wantVal  interface{}
		wantErr  bool
	}{
		{
			name:     "existing value",
			input:    "10",
			wantType: reflect.TypeOf(customTypeText{}),
			wantVal:  customTypeText{Value: 10},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Structure{
				configData: make(map[string]interface{}),
			}
			dv := &dynamicVar{
				config: config,
				name:   tt.name,
				want:   tt.wantType,
			}

			err := dv.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				got, ok := config.Get(tt.name)
				if !ok {
					t.Errorf("Expected value for %s not found", tt.name)
					return
				}

				if !reflect.DeepEqual(got, tt.wantVal) {
					t.Errorf("Set() got = %v, want %v", got, tt.wantVal)
				}
			}
		})
	}
}

func TestDynamicVarSetCustomTypeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType reflect.Type
		wantVal  interface{}
		wantErr  bool
	}{
		{
			name:     "existing value",
			input:    "10",
			wantType: reflect.TypeOf(customTypeJSON{}),
			wantVal:  customTypeJSON{Value: 10},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Structure{
				configData: make(map[string]interface{}),
			}
			dv := &dynamicVar{
				config: config,
				name:   tt.name,
				want:   tt.wantType,
			}

			err := dv.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				got, ok := config.Get(tt.name)
				if !ok {
					t.Errorf("Expected value for %s not found", tt.name)
					return
				}

				if !reflect.DeepEqual(got, tt.wantVal) {
					t.Errorf("Set() got = %v, want %v", got, tt.wantVal)
				}
			}
		})
	}
}
