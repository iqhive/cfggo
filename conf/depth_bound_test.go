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
				input := []byte("x=" + b.deep(tc.depth) + "\n")
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
			if _, err := codec.Decode([]byte("x=" + nestedObjects(tc.depth) + "\n")); tc.wantErr {
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
