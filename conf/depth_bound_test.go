package conf

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func nestedArrays(depth int) string {
	return strings.Repeat("[", depth) + strings.Repeat("]", depth)
}

func nestedObjects(depth int) string {
	return strings.Repeat(`{"k":`, depth) + "0" + strings.Repeat("}", depth)
}

func TestDepthBound(t *testing.T) {
	cases := []struct {
		name    string
		depth   int
		wantErr bool
	}{
		{name: "shallow nesting succeeds", depth: 3, wantErr: false},
		{name: "below-limit nesting succeeds", depth: defaultMaxDepth - 1, wantErr: false},
		{name: "at-limit nesting succeeds", depth: defaultMaxDepth, wantErr: false},
		{name: "above-limit nesting fails", depth: defaultMaxDepth + 1, wantErr: true},
		{name: "far above-limit nesting fails", depth: 200, wantErr: true},
	}
	builders := []struct {
		name  string
		deep  func(depth int) string
		inner func(depth int) string
	}{
		{name: "array", deep: nestedArrays, inner: nestedArrays},
		{name: "object", deep: nestedObjects, inner: nestedObjects},
	}
	for _, b := range builders {
		for _, tc := range cases {
			t.Run("decode/"+b.name+"/"+tc.name, func(t *testing.T) {
				// The key x is one level of the budget, so nest depth-1 deep
				// to keep the total depth (path plus value) at tc.depth.
				input := []byte("x=" + b.deep(tc.depth-1) + "\n")
				_, err := Decode(input)
				if tc.wantErr {
					if !errors.Is(err, ErrLimitExceeded) {
						t.Fatalf("Decode depth %d error = %v, want ErrLimitExceeded", tc.depth, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Decode depth %d error = %v, want nil", tc.depth, err)
				}
			})
			t.Run("encode/"+b.name+"/"+tc.name, func(t *testing.T) {
				// Wrap in {"x":...} adds one level, so nest depth-1 deep
				// to keep the total JSON depth at tc.depth.
				input := json.RawMessage(`{"x":` + b.inner(tc.depth-1) + `}`)
				_, err := Encode(input)
				if tc.wantErr {
					if !errors.Is(err, ErrLimitExceeded) {
						t.Fatalf("Encode depth %d error = %v, want ErrLimitExceeded", tc.depth, err)
					}
					var parseErr *ParseError
					if !errors.As(err, &parseErr) {
						t.Fatalf("Encode depth %d error = %#v, want *ParseError", tc.depth, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Encode depth %d error = %v, want nil", tc.depth, err)
				}
			})
		}
	}
}

func TestDepthBoundCustomLimit(t *testing.T) {
	const maxDepth = 4
	codec, err := New(WithLimits(Limits{MaxDepth: maxDepth}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		depth   int
		wantErr bool
	}{
		{name: "at-limit succeeds", depth: maxDepth, wantErr: false},
		{name: "above-limit fails", depth: maxDepth + 1, wantErr: true},
	} {
		t.Run("decode/"+tc.name, func(t *testing.T) {
			if _, err := codec.Decode([]byte("x=" + nestedObjects(tc.depth-1) + "\n")); tc.wantErr {
				if !errors.Is(err, ErrLimitExceeded) {
					t.Fatalf("Decode depth %d error = %v, want ErrLimitExceeded", tc.depth, err)
				}
			} else if err != nil {
				t.Fatalf("Decode depth %d error = %v, want nil", tc.depth, err)
			}
		})
		t.Run("encode/"+tc.name, func(t *testing.T) {
			input := json.RawMessage(`{"x":` + nestedObjects(tc.depth-1) + `}`)
			_, err := codec.Encode(input)
			if tc.wantErr {
				if !errors.Is(err, ErrLimitExceeded) {
					t.Fatalf("Encode depth %d error = %v, want ErrLimitExceeded", tc.depth, err)
				}
				var parseErr *ParseError
				if !errors.As(err, &parseErr) {
					t.Fatalf("Encode depth %d error = %#v, want *ParseError", tc.depth, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Encode depth %d error = %v, want nil", tc.depth, err)
			}
		})
	}
}

// A key's path and its value share one depth budget, so a document that
// decodes is always one Encode can rewrite, and vice versa
func TestDepthBoundCountsPathAndValue(t *testing.T) {
	const maxDepth = 4
	codec, err := New(WithLimits(Limits{MaxDepth: maxDepth}))
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{
		"[a.b.c]\nd=1\n",
		"[a.b]\nc=[1]\n",
		"[a]\nb=[[1]]\n",
		"x=" + nestedObjects(maxDepth-1) + "\n",
		"x=" + nestedArrays(maxDepth-1) + "\n",
	} {
		first, err := codec.Decode([]byte(doc))
		if err != nil {
			t.Fatalf("Decode(%q): %v", doc, err)
		}
		encoded, err := codec.Encode(first)
		if err != nil {
			t.Fatalf("Encode(%s): %v", first, err)
		}
		second, err := codec.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(encoded %q): %v", encoded, err)
		}
		if string(first) != string(second) {
			t.Fatalf("round trip changed %q: %s -> %s", doc, first, second)
		}
	}
	for _, doc := range []string{
		"[a.b.c]\nd=[]\n",
		"[a.b.c]\nd={}\n",
		"[a.b]\nc=[[1]]\n",
		"x=" + nestedObjects(maxDepth) + "\n",
		"x=" + nestedArrays(maxDepth) + "\n",
	} {
		_, err := codec.Decode([]byte(doc))
		var parseErr *ParseError
		if !errors.Is(err, ErrLimitExceeded) || !errors.As(err, &parseErr) || parseErr.Line == 0 {
			t.Fatalf("Decode(%q) error = %#v, want ErrLimitExceeded ParseError with a line", doc, err)
		}
	}

	// Encode applies the same budget to a leaf under a dotted root key, so it
	// never writes a document Decode would reject
	if _, err := codec.Encode(json.RawMessage(`{"a.b.c":{"x.y":[1]}}`)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("Encode over budget error = %v, want ErrLimitExceeded", err)
	}
	encoded, err := codec.Encode(json.RawMessage(`{"a.b":{"x.y":[1]}}`))
	if err != nil {
		t.Fatalf("Encode at budget: %v", err)
	}
	if got, err := codec.Decode(encoded); err != nil || string(got) != `{"a":{"b":{"x.y":[1]}}}` {
		t.Fatalf("Decode(%q) = %s, %v", encoded, got, err)
	}
}
