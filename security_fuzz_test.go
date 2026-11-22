package cfggo

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	iconvert "github.com/iqhive/cfggo/internal/convert"
	"github.com/iqhive/cfggo/sources"
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

// FuzzRendersSurviveArbitraryDocuments loads an arbitrary JSON document into
// a real configuration and runs every human-readable render path over it.
// These paths (String, DiagnoseData, Report, ExplainKey) format stored values
// directly, so a document that makes any of them panic, hang, or recurse
// without bound is a process crash from untrusted configuration input.
func FuzzRendersSurviveArbitraryDocuments(f *testing.F) {
	for _, seed := range []string{
		`{}`,
		`{"data":{"nested":{"deep":[1,2,{"x":null}]}}}`,
		`{"data":9007199254740993}`,
		`{"data":"text"}`,
		`{"data":[[[[[1]]]]]}`,
		`{"unknown":{"key":true}}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, document string) {
		cfg := &renderSafetyConfig{}
		if err := cfg.Init(cfg,
			WithConfigHandler(sources.NewHandlerBytes(json.RawMessage(document), false)),
			WithoutFlags(), WithoutEnv(),
		); err != nil {
			return
		}
		_ = cfg.String()
		_ = cfg.DiagnoseData().String()
		_ = cfg.Report()
		_ = cfg.ExplainKey("data")
	})
}

// FuzzRedactFlagTextMasksSecretValues drives the flag-error redactor with
// adversarial rejected values -- arbitrary text including quotes, backslashes
// and substrings that themselves look like flag errors -- across secret flags
// of several name shapes (plain, ':' and ' ' included). The security invariant
// is that a secret flag's rejected value is always replaced by the mask, while
// a non-secret flag keeps its value. Before the fix for flag names holding ':'
// or whitespace, such a secret's raw value leaked here.
func FuzzRedactFlagTextMasksSecretValues(f *testing.F) {
	cfg := &specialNameSecretConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{"", "hunter2", `a"b`, `a\b`, "a\nb", `x for flag -password: y`, "S3CR3T"} {
		f.Add(seed, "bad value")
	}
	f.Fuzz(func(t *testing.T, value, other string) {
		for _, name := range []string{"password", "pin:code", "my pass", "port"} {
			secret := name != "port"
			rawPrefix := "invalid value " + strconv.Quote(value) + " for flag -" + name
			msg := rawPrefix + ": " + other
			got := cfg.redactFlagText(msg)
			if secret {
				maskedPrefix := "invalid value \"" + maskedValue + "\" for flag -" + name
				if !strings.HasPrefix(got, maskedPrefix) {
					t.Fatalf("secret value not masked for %q:\n msg: %q\n got: %q", name, msg, got)
				}
			} else if !strings.HasPrefix(got, rawPrefix) {
				t.Fatalf("non-secret value not preserved for %q:\n msg: %q\n got: %q", name, msg, got)
			}
		}
	})
}
