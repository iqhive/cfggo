package convert_test

import (
	"reflect"
	"testing"

	"github.com/iqhive/cfggo/internal/convert"
)

func TestConvertStringEmptyMap(t *testing.T) {
	mapType := reflect.TypeOf(map[string]string{})

	cases := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{name: "empty string yields empty non-nil map", input: "", want: map[string]string{}},
		{name: "whitespace-only yields empty non-nil map", input: "   ", want: map[string]string{}},
		{name: "non-empty map input still parses", input: "a:b", want: map[string]string{"a": "b"}},
		{name: "malformed input still errors", input: "a", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convert.ConvertString(tc.input, mapType, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ConvertString(%q) = %v, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConvertString(%q) returned error: %v", tc.input, err)
			}
			m, ok := got.(map[string]string)
			if !ok {
				t.Fatalf("ConvertString(%q) = %T, want map[string]string", tc.input, got)
			}
			if m == nil {
				t.Fatalf("ConvertString(%q) returned nil map, want empty non-nil map", tc.input)
			}
			if !reflect.DeepEqual(m, tc.want) {
				t.Fatalf("ConvertString(%q) = %v, want %v", tc.input, m, tc.want)
			}
		})
	}
}
