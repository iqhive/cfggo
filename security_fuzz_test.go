package cfggo

import (
	"reflect"
	"testing"
	"time"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

func FuzzDecodeJSONConfig(f *testing.F) {
	for _, seed := range []string{
		`{"port":8080}`,
		`{"nested":{"array":[1,true,null,"text"]}}`,
		`{"port":1,"port":2}`,
		`[{"not":"an object"}]`,
		`{"large":9007199254740993}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = decodeJSONConfig([]byte(input))
	})
}

func FuzzConvertStringDoesNotPanic(f *testing.F) {
	for _, seed := range []string{"", "true", "-1", "1e309", "[1,2]", `{"1":"one","01":"two"}`, "a:b,c:d", "\x00\xff"} {
		f.Add(seed)
	}
	targets := []reflect.Type{
		reflect.TypeOf(false),
		reflect.TypeOf(int(0)),
		reflect.TypeOf(int8(0)),
		reflect.TypeOf(uint64(0)),
		reflect.TypeOf(float64(0)),
		reflect.TypeOf(""),
		reflect.TypeOf(time.Duration(0)),
		reflect.TypeOf([]int{}),
		reflect.TypeOf([]interface{}{}),
		reflect.TypeOf([2]int{}),
		reflect.TypeOf(map[string]int{}),
		reflect.TypeOf(map[int]string{}),
		reflect.TypeOf(map[string]interface{}{}),
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, target := range targets {
			converted, err := iconvert.ConvertString(input, target, nil)
			if err != nil {
				continue
			}
			if converted == nil || !reflect.TypeOf(converted).AssignableTo(target) {
				t.Fatalf("ConvertString(%q, %v) returned %T (%v), which is not assignable to the target", input, target, converted, converted)
			}
		}
	})
}
