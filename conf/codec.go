package conf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	defaultMaxInputBytes  int64 = 10 << 20
	defaultMaxOutputBytes int64 = 10 << 20
	defaultMaxLineBytes         = 1 << 20
	defaultMaxLines             = 100_000
	defaultMaxEntries           = 100_000
	defaultMaxDepth             = 64
)

// Limits bounds parser and encoder resource use. Zero fields use defaults.
type Limits struct {
	MaxInputBytes  int64
	MaxOutputBytes int64
	MaxLineBytes   int
	MaxLines       int
	MaxEntries     int
	MaxDepth       int
}

// DefaultLimits returns the limits used by Decode, Encode, and New.
func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:  defaultMaxInputBytes,
		MaxOutputBytes: defaultMaxOutputBytes,
		MaxLineBytes:   defaultMaxLineBytes,
		MaxLines:       defaultMaxLines,
		MaxEntries:     defaultMaxEntries,
		MaxDepth:       defaultMaxDepth,
	}
}

// Option configures a Codec.
type Option func(*Limits) error

// WithLimits replaces non-zero default limits. Negative limits are invalid.
func WithLimits(limits Limits) Option {
	return func(target *Limits) error {
		if limits.MaxInputBytes < 0 || limits.MaxOutputBytes < 0 || limits.MaxLineBytes < 0 ||
			limits.MaxLines < 0 || limits.MaxEntries < 0 || limits.MaxDepth < 0 {
			return fmt.Errorf("conf: limits must not be negative")
		}
		if limits.MaxInputBytes != 0 {
			target.MaxInputBytes = limits.MaxInputBytes
		}
		if limits.MaxOutputBytes != 0 {
			target.MaxOutputBytes = limits.MaxOutputBytes
		}
		if limits.MaxLineBytes != 0 {
			target.MaxLineBytes = limits.MaxLineBytes
		}
		if limits.MaxLines != 0 {
			target.MaxLines = limits.MaxLines
		}
		if limits.MaxEntries != 0 {
			target.MaxEntries = limits.MaxEntries
		}
		if limits.MaxDepth != 0 {
			target.MaxDepth = limits.MaxDepth
		}
		return nil
	}
}

// Codec translates between conf documents and cfggo's canonical JSON.
type Codec struct{ limits Limits }

// New constructs a Codec.
func New(options ...Option) (*Codec, error) {
	limits := DefaultLimits()
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&limits); err != nil {
			return nil, err
		}
	}
	return &Codec{limits: limits}, nil
}

// Decode parses a conf document with default limits.
func Decode(input []byte) (json.RawMessage, error) {
	codec, _ := New()
	return codec.Decode(input)
}

// Encode renders canonical JSON as a deterministic conf document.
func Encode(input json.RawMessage) ([]byte, error) {
	codec, _ := New()
	return codec.Encode(input)
}

func decodeJSONValue(input string) (any, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(input))
	decoder.UseNumber()
	value, err := decodeJSONToken(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func decodeJSONToken(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", key)
			}
			value, err := decodeJSONToken(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("invalid object terminator")
		}
		return object, nil
	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeJSONToken(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("invalid array terminator")
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}
