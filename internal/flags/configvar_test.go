package flags

import (
	"errors"
	"reflect"
	"testing"
)

func TestConfigVarSetConvertsConcreteTypes(t *testing.T) {
	var got interface{}
	cv := &ConfigVar{
		Name: "port",
		Want: reflect.TypeOf(0),
		Setter: func(value interface{}) error {
			got = value
			return nil
		},
	}

	if err := cv.Set("8080"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got != 8080 {
		t.Fatalf("setter value = %#v, want 8080", got)
	}
}

func TestConfigVarSetInfersInterfaceValues(t *testing.T) {
	tests := []struct {
		input string
		want  interface{}
	}{
		{"42", 42},
		{"3.5", 3.5},
		{"true", true},
		{"hello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var got interface{}
			cv := &ConfigVar{
				Name: "value",
				Want: reflect.TypeOf((*interface{})(nil)).Elem(),
				Setter: func(value interface{}) error {
					got = value
					return nil
				},
			}
			if err := cv.Set(tt.input); err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("setter value = %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestConfigVarSetErrors(t *testing.T) {
	if err := (&ConfigVar{Name: "bad"}).Set("value"); err == nil {
		t.Fatal("Set() with nil type: expected error, got nil")
	}

	cv := &ConfigVar{
		Name:   "port",
		Want:   reflect.TypeOf(0),
		Setter: func(interface{}) error { return nil },
	}
	if err := cv.Set("not-int"); err == nil {
		t.Fatal("Set() with invalid conversion: expected error, got nil")
	}

	setterErr := errors.New("setter failed")
	cv = &ConfigVar{
		Name: "port",
		Want: reflect.TypeOf(0),
		Setter: func(interface{}) error {
			return setterErr
		},
	}
	if err := cv.Set("8080"); !errors.Is(err, setterErr) {
		t.Fatalf("Set() error = %v, want setter error", err)
	}
}

func TestConfigVarBoolFlagAndString(t *testing.T) {
	cv := &ConfigVar{IsBool: true}
	if !cv.IsBoolFlag() {
		t.Fatal("IsBoolFlag() = false, want true")
	}
	if got := cv.String(); got != "" {
		t.Fatalf("String() = %q, want empty string", got)
	}
}

func TestFilterTestFlags(t *testing.T) {
	got := FilterTestFlags([]string{
		"cmd",
		"-test.v",
		"-test.run=TestName",
		"--app-flag=value",
		"value",
	})
	want := []string{"cmd", "--app-flag=value", "value"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterTestFlags() = %#v, want %#v", got, want)
	}
}
