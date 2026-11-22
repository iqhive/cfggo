package cfggo

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	iconvert "github.com/iqhive/cfggo/internal/convert"
	"github.com/iqhive/cfggo/sources"
)

func TestDecodeJSONConfigRejectsDuplicateObjectKeys(t *testing.T) {
	for _, document := range []string{
		`{"port":8080,"port":9090}`,
		`{"database":{"host":"first","host":"second"}}`,
		`{"items":[{"token":"first","token":"second"}]}`,
	} {
		t.Run(document, func(t *testing.T) {
			if _, err := decodeJSONConfig([]byte(document)); err == nil {
				t.Fatal("decodeJSONConfig accepted duplicate object keys")
			} else if !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("decodeJSONConfig error = %v, want a duplicate-key error", err)
			}
		})
	}
}

type deepSecurityGroupConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestGroupRejectsDuplicateNamespaces(t *testing.T) {
	handler := sources.NewHandlerBytes([]byte(`{"auth":{"port":1111},"auth":{"port":2222}}`), false)
	group := NewGroup(GroupWithConfigHandler(handler), GroupWithoutFlags())
	group.Register("auth", &deepSecurityGroupConfig{})
	if err := group.Init(); err == nil {
		t.Fatal("Group.Init accepted two different values for the same namespace")
	} else if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Group.Init error = %v, want a duplicate-key error", err)
	}
}

func TestConvertMapRejectsKeyCollisions(t *testing.T) {
	input := map[string]string{"1": "first", "01": "second"}
	if _, err := iconvert.ConvertValue(input, reflect.TypeOf(map[int]string{}), nil); err == nil {
		t.Fatal("ConvertValue silently merged distinct source keys into one converted map key")
	} else if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "collision") {
		t.Fatalf("ConvertValue error = %v, want it to describe the key collision", err)
	}
}

type dynamicSecretConfig struct {
	Structure
	Token func() int `cfggo:"token" secret:"true"`
}

func TestDynamicVarDoesNotPassSecretToErrorWrapperBeforeRedaction(t *testing.T) {
	cfg := &dynamicSecretConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const secret = "dynamic-secret-value-7139"
	var wrapped []string
	cfg.SetErrorWrapper(func(err error, _ int, msg string, args ...interface{}) error {
		if err != nil {
			wrapped = append(wrapped, err.Error())
		}
		if len(args) > 0 {
			wrapped = append(wrapped, fmt.Sprintf(msg, args...))
		} else {
			wrapped = append(wrapped, msg)
		}
		return fmt.Errorf("wrapped error")
	})

	dv := &dynamicVar{config: &cfg.Structure, name: "token", want: reflect.TypeOf(0)}
	if err := dv.Set(secret); err == nil {
		t.Fatal("dynamicVar.Set accepted a non-integer secret")
	}
	if strings.Contains(strings.Join(wrapped, "\n"), secret) {
		t.Fatalf("error wrapper received the raw secret before redaction: %s", strings.Join(wrapped, "\n"))
	}
}

func TestNewFlagClonesCyclicValuesWithoutOverflowingStack(t *testing.T) {
	original := map[string]interface{}{"name": "root"}
	original["self"] = original

	cfg := &Structure{}
	cfg.NewFlag("cycle", original, "cyclic value")
	got, ok := cfg.Get("cycle")
	if !ok {
		t.Fatal("cyclic NewFlag value was not stored")
	}
	cloned := got.(map[string]interface{})
	self := cloned["self"].(map[string]interface{})
	self["injected"] = true
	if _, leaked := original["injected"]; leaked {
		t.Fatal("cloned cyclic map still aliases the caller's map")
	}
	if _, preserved := cloned["injected"]; !preserved {
		t.Fatal("the map's self-reference was not preserved in the clone")
	}
}

type cyclicNode struct {
	Next *cyclicNode
}

func TestClonePreservesCyclicSliceAndPointerGraphs(t *testing.T) {
	originalSlice := make([]interface{}, 1)
	originalSlice[0] = originalSlice
	clonedSlice := cloneMutableInterface(originalSlice).([]interface{})
	clonedSelf := clonedSlice[0].([]interface{})
	if reflect.ValueOf(clonedSlice).Pointer() == reflect.ValueOf(originalSlice).Pointer() {
		t.Fatal("cloned slice still aliases the original backing array")
	}
	if reflect.ValueOf(clonedSelf).Pointer() != reflect.ValueOf(clonedSlice).Pointer() {
		t.Fatal("the slice's self-reference was not preserved")
	}

	originalPointer := &cyclicNode{}
	originalPointer.Next = originalPointer
	clonedPointer := cloneMutableInterface(originalPointer).(*cyclicNode)
	if clonedPointer == originalPointer {
		t.Fatal("cloned pointer still aliases the original")
	}
	if clonedPointer.Next != clonedPointer {
		t.Fatal("the pointer's self-reference was not preserved")
	}
}
