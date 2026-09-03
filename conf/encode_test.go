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
