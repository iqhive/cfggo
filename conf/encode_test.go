package conf

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestEncodeDeterministicRoundTrip(t *testing.T) {
	input := json.RawMessage(`{"z":"last","port":8080,"database":{"pool":{"size":12},"host":"db#1"},"items":["a","b"]}`)
	got, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := "items=[\"a\",\"b\"]\nport=8080\nz=\"last\"\n\n[database]\nhost=\"db\\u00231\"\n\n[database.pool]\nsize=12\n"
	if string(got) != want {
		t.Fatalf("Encode output:\n%s\nwant:\n%s", got, want)
	}

	roundTrip, err := Decode(got)
	if err != nil {
		t.Fatalf("Decode encoded output: %v", err)
	}
	var after any
	if err := json.Unmarshal(roundTrip, &after); err != nil {
		t.Fatal(err)
	}
	wantDecoded := map[string]any{
		"z": "last", "port": float64(8080), "items": []any{"a", "b"},
		"database": map[string]any{"host": "db#1", "pool": map[string]any{"size": float64(12)}},
	}
	if !reflect.DeepEqual(after, wantDecoded) {
		t.Fatalf("round trip = %#v, want %#v", after, wantDecoded)
	}
}

func TestEncodeGlobalObjectRemainsRepresentable(t *testing.T) {
	got, err := Encode(json.RawMessage(`{"global":{"name":"nested"},"name":"root"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "global.name=\"nested\"\nname=\"root\"\n"
	if string(got) != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestEncodeRejectsInvalidInputAndLimits(t *testing.T) {
	for _, input := range []string{"", "[]", "{} trailing", `{"a":1,"a":2}`} {
		if _, err := Encode(json.RawMessage(input)); !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("Encode(%q) error = %v, want ErrInvalidJSON", input, err)
		}
	}
	codec, err := New(WithLimits(Limits{MaxOutputBytes: 3}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.Encode(json.RawMessage(`{"key":"value"}`)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("error = %v, want ErrLimitExceeded", err)
	}
	if _, err := Encode(json.RawMessage(`{"a.b":1,"a":{"b":2}}`)); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("ambiguous path error = %v, want ErrDuplicateKey", err)
	}
}

func TestEncodeKeepsNonPathKeysAsJSONLeaf(t *testing.T) {
	// Root keys are cfggo's dotted config keys and are split into sections. A
	// nested object whose keys are not plain path segments (a map value keyed
	// by "a.b", or by text parsePath rejects) is written as one JSON value so
	// Decode restores it unchanged instead of splitting its keys
	input := json.RawMessage(`{"labels":{"a.b":1,"plain":"x"},"database.tags":{"host#1":true},"database.pool":{"size":12},"mixed":{"ok":{"k.v":null}}}`)
	got, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want := "labels={\"a.b\":1,\"plain\":\"x\"}\n\n" +
		"[database]\ntags={\"host\\u00231\":true}\n\n" +
		"[database.pool]\nsize=12\n\n" +
		"[mixed]\nok={\"k.v\":null}\n"
	if string(got) != want {
		t.Fatalf("Encode output:\n%s\nwant:\n%s", got, want)
	}

	decoded, err := Decode(got)
	if err != nil {
		t.Fatalf("Decode encoded output: %v", err)
	}
	var after any
	if err := json.Unmarshal(decoded, &after); err != nil {
		t.Fatal(err)
	}
	wantDecoded := map[string]any{
		"labels": map[string]any{"a.b": float64(1), "plain": "x"},
		"database": map[string]any{
			"tags": map[string]any{"host#1": true},
			"pool": map[string]any{"size": float64(12)},
		},
		"mixed": map[string]any{"ok": map[string]any{"k.v": nil}},
	}
	if !reflect.DeepEqual(after, wantDecoded) {
		t.Fatalf("round trip = %#v, want %#v", after, wantDecoded)
	}
	again, err := Encode(decoded)
	if err != nil {
		t.Fatalf("Encode decoded output: %v", err)
	}
	if string(again) != want {
		t.Fatalf("second Encode differs:\n%s\nwant:\n%s", again, want)
	}

	// Root keys are still validated as paths
	if _, err := Encode(json.RawMessage(`{"a#b":1}`)); !errors.Is(err, ErrSyntax) {
		t.Fatalf("root key error = %v, want ErrSyntax", err)
	}
}
