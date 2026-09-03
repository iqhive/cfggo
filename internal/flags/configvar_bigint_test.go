package flags

import (
	"reflect"
	"testing"
)

func TestConfigVarInterfaceTargetKeepsLargeIntegersExact(t *testing.T) {
	var got interface{}
	cv := &ConfigVar{
		Name:   "id",
		Want:   reflect.TypeOf((*interface{})(nil)).Elem(),
		Setter: func(v interface{}) error { got = v; return nil },
	}
	if err := cv.Set("9007199254740993"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got != int(9007199254740993) {
		t.Fatalf("interface{} flag value = %#v (%T), want the exact int", got, got)
	}
	if err := cv.Set("2.5"); err != nil {
		t.Fatalf("Set(2.5): %v", err)
	}
	if got != 2.5 {
		t.Fatalf("interface{} flag value = %#v, want 2.5", got)
	}
	if err := cv.Set("1e3"); err != nil {
		t.Fatalf("Set(1e3): %v", err)
	}
	if got != 1000 {
		t.Fatalf("interface{} flag value = %#v (%T), want int 1000", got, got)
	}
}
