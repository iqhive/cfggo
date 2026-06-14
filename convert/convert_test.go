package convert_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/iqhive/cfggo/convert"
)

type recordingWrapper struct {
	called bool
}

func (w *recordingWrapper) WrapError(err error, _ int, _ string, _ ...interface{}) error {
	w.called = true
	if err == nil {
		return errors.New("wrapper received nil error")
	}
	return err
}

func TestConvertValueDelegatesToInternalConverter(t *testing.T) {
	got, err := convert.ConvertValue("8080", reflect.TypeOf(0), nil)
	if err != nil {
		t.Fatalf("ConvertValue() error = %v", err)
	}
	if got != 8080 {
		t.Fatalf("ConvertValue() = %#v, want 8080", got)
	}
}

func TestConvertValueReturnsWrappedErrors(t *testing.T) {
	wrapper := &recordingWrapper{}

	if _, err := convert.ConvertValue("not-int", reflect.TypeOf(0), wrapper); err == nil {
		t.Fatal("ConvertValue() expected error, got nil")
	}
	if !wrapper.called {
		t.Fatal("expected error wrapper to be called")
	}
}
