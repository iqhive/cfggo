package conf

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeSectionsGlobalsCommentsAndValues(t *testing.T) {
	input := []byte("\ufeffapp = gateway # root comment\r\n" +
		"port=8080\n" +
		"[database] # section comment\n" +
		"host = db.internal # discarded\n" +
		"labels = {\"tier\":\"primary\"}\n" +
		"[database.pool]\n" +
		"size = 12\n" +
		"[global]\n" +
		"debug = true\n" +
		"empty =\n")

	got, err := Decode(input)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	want := map[string]any{
		"app": "gateway", "port": float64(8080), "debug": true, "empty": "",
		"database": map[string]any{
			"host":   "db.internal",
			"labels": map[string]any{"tier": "primary"},
			"pool":   map[string]any{"size": float64(12)},
		},
	}
	if !reflect.DeepEqual(document, want) {
		t.Fatalf("decoded document = %#v, want %#v", document, want)
	}
}

func TestDecodeHashAlwaysStartsComment(t *testing.T) {
	got, err := Decode([]byte("value = before#after\nquoted = \"before#after\"\nescaped = \"before\\u0023after\"\n"))
	if err == nil {
		t.Fatalf("Decode unexpectedly succeeded: %s", got)
	}
	if !errors.Is(err, ErrSyntax) {
		t.Fatalf("error = %v, want ErrSyntax", err)
	}

	got, err = Decode([]byte("value = before#after\nescaped = \"before\\u0023after\"\n"))
	if err != nil {
		t.Fatalf("Decode escaped hash: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	if document["value"] != "before" || document["escaped"] != "before#after" {
		t.Fatalf("decoded hash values = %#v", document)
	}
}

func TestDecodeNullQuotedAndStructuredValues(t *testing.T) {
	got, err := Decode([]byte("clear=null\nword=\"true\"\nitems=[1,\"two\"]\nobject={\"enabled\":true}\n"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	if document["clear"] != nil || document["word"] != "true" {
		t.Fatalf("decoded scalars = %#v", document)
	}
	if !reflect.DeepEqual(document["items"], []any{float64(1), "two"}) {
		t.Fatalf("items = %#v", document["items"])
	}
}

func TestDecodeRejectsInvalidDocuments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  error
	}{
		{"missing equals", "key value", ErrSyntax},
		{"empty key", "=value", ErrSyntax},
		{"bad section", "[section", ErrSyntax},
		{"empty segment", "a..b=value", ErrSyntax},
		{"malformed quoted value", "key=\"value", ErrSyntax},
		{"malformed array", "key=[1,", ErrSyntax},
		{"duplicate inline object key", `key={"a":1,"a":2}`, ErrSyntax},
		{"duplicate", "key=one\n[global]\nkey=two", ErrDuplicateKey},
		{"scalar then child", "key=one\n[key]\nchild=two", ErrKeyConflict},
		{"child then scalar", "[key]\nchild=two\n[global]\nkey=one", ErrKeyConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode([]byte(test.input))
			if !errors.Is(err, test.kind) {
				t.Fatalf("error = %v, want %v", err, test.kind)
			}
		})
	}
}

func TestDecodeLimitsAndLocations(t *testing.T) {
	codec, err := New(WithLimits(Limits{MaxInputBytes: 8, MaxLineBytes: 5, MaxLines: 2, MaxEntries: 1, MaxDepth: 2}))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"123456789", "key=12", "a=1\nb=2\nc=3", "a=1\nb=2", "[a.b]\nc=1"} {
		if _, err := codec.Decode([]byte(input)); !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("Decode(%q) error = %v, want limit error", input, err)
		}
	}

	_, err = Decode([]byte("ok=1\nbroken"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Line != 2 || !strings.Contains(err.Error(), ":2:") {
		t.Fatalf("error = %#v, want line 2 ParseError", err)
	}
	if _, err := New(WithLimits(Limits{MaxDepth: -1})); err == nil {
		t.Fatal("New accepted a negative limit")
	}
}

func TestDecodeRejectsInvalidUTF8(t *testing.T) {
	if _, err := Decode([]byte{0xff, '=', 'x'}); !errors.Is(err, ErrSyntax) {
		t.Fatalf("error = %v, want ErrSyntax", err)
	}
}

func TestDecodeBareJSONLiteralsAreTyped(t *testing.T) {
	got, err := Decode([]byte("n=8080\nf=1.5\nneg=-3\nexp=1e3\nt=true\nfl=false\nq=\"8080\"\n" +
		"lead=007\nplus=+5\nword=truex\nver=1.0.0\nbig=9007199254740993\n"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"n": float64(8080), "f": 1.5, "neg": float64(-3), "exp": float64(1000), "t": true, "fl": false,
		"q": "8080", "lead": "007", "plus": "+5", "word": "truex", "ver": "1.0.0", "big": float64(9007199254740992),
	}
	if !reflect.DeepEqual(document, want) {
		t.Fatalf("decoded = %#v\nwant    = %#v", document, want)
	}
	// The exact literal is preserved in the canonical JSON even when float64 cannot hold it
	if !strings.Contains(string(got), `"big":9007199254740993`) {
		t.Fatalf("canonical JSON = %s, want the exact big literal", got)
	}
}

func TestEncodeDecodeRoundTripPreservesTypes(t *testing.T) {
	for _, doc := range []string{
		"o={\"n\":1,\"b\":true,\"s\":\"x\",\"t\":\"1\"}\n",
		"port=8080\nname=api\n[db]\nsize=12\nhost=h\n",
		"items=[1,\"two\",true]\nempty=\n",
	} {
		first, err := Decode([]byte(doc))
		if err != nil {
			t.Fatalf("Decode(%q): %v", doc, err)
		}
		encoded, err := Encode(first)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		second, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(encoded %q): %v", encoded, err)
		}
		var a, b any
		if err := json.Unmarshal(first, &a); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(second, &b); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("round trip changed %q:\nfirst  = %s\nsecond = %s\nvia    = %q", doc, first, second, encoded)
		}
	}
}
