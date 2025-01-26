package cfggo

import (
	"reflect"
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
	{"Duration", "abc", reflect.TypeOf(time.Duration(0)), time.Second, true},
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
