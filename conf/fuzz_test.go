package conf

import (
	"bytes"
	"testing"
)

func FuzzDecode(f *testing.F) {
	f.Add([]byte("port=8080\n[database]\nhost=localhost\n"))
	f.Add([]byte("value=before#comment\n"))
	f.Add([]byte("[global]\ndebug=true\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = Decode(input)
	})
}

func FuzzEncodeDecodeRoundTrip(f *testing.F) {
	for _, seed := range []string{
		`{"port":8080}`,
		`{"database":{"host":"localhost","ports":[5432,5433]}}`,
		`{"a.b":true,"empty":{},"text":"# not a comment"}`,
		`{"large":9007199254740993}`,
		`not JSON`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		encoded, err := Encode([]byte(input))
		if err != nil {
			return
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(Encode(input)) failed: %v\nencoded: %q", err, encoded)
		}
		reencoded, err := Encode(decoded)
		if err != nil {
			t.Fatalf("second Encode failed: %v\ndecoded: %s", err, decoded)
		}
		if !bytes.Equal(reencoded, encoded) {
			t.Fatalf("canonical output changed after round trip\nfirst:  %q\nsecond: %q", encoded, reencoded)
		}
	})
}
