package conf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type encodedEntry struct {
	path  []string
	value any
}

// Encode renders canonical JSON as deterministic conf text. Root keys are
// cfggo's dotted configuration keys and are split into sections. A nested
// object is flattened the same way only when every key is a plain path
// segment; otherwise (for example a map value keyed by "a.b") it is written
// as a single JSON value so Decode restores it unchanged
func (c *Codec) Encode(input json.RawMessage) ([]byte, error) {
	if int64(len(input)) > c.limits.MaxInputBytes {
		return nil, &ParseError{Err: ErrLimitExceeded}
	}
	value, err := decodeJSONValue(string(input), c.limits.MaxDepth)
	if err != nil {
		if errors.Is(err, ErrLimitExceeded) {
			return nil, &ParseError{Err: err}
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: root must be an object", ErrInvalidJSON)
	}

	var entries []encodedEntry
	if err := flattenObject(root, nil, &entries, c.limits, 0); err != nil {
		return nil, err
	}
	validatedRoot := make(map[string]any)
	validatedPaths := &pathNode{children: make(map[string]*pathNode)}
	for _, entry := range entries {
		if err := insertPath(validatedRoot, validatedPaths, entry.path, entry.value, 0); err != nil {
			return nil, fmt.Errorf("conf: canonical JSON contains ambiguous paths: %w", err)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.Join(entries[i].path, ".") < strings.Join(entries[j].path, ".")
	})

	rootEntries := make([]encodedEntry, 0)
	sections := make(map[string][]encodedEntry)
	for _, entry := range entries {
		if len(entry.path) == 1 || entry.path[0] == "global" {
			rootEntries = append(rootEntries, entry)
			continue
		}
		section := strings.Join(entry.path[:len(entry.path)-1], ".")
		sections[section] = append(sections[section], entry)
	}

	var output bytes.Buffer
	writeEntry := func(entry encodedEntry, key string) error {
		encoded, err := json.Marshal(entry.value)
		if err != nil {
			return err
		}
		encoded = bytes.ReplaceAll(encoded, []byte("#"), []byte(`\u0023`))
		output.WriteString(key)
		output.WriteByte('=')
		output.Write(encoded)
		output.WriteByte('\n')
		if int64(output.Len()) > c.limits.MaxOutputBytes {
			return &ParseError{Err: ErrLimitExceeded}
		}
		return nil
	}
	for _, entry := range rootEntries {
		if err := writeEntry(entry, strings.Join(entry.path, ".")); err != nil {
			return nil, err
		}
	}
	sectionNames := make([]string, 0, len(sections))
	for section := range sections {
		sectionNames = append(sectionNames, section)
	}
	sort.Strings(sectionNames)
	for _, section := range sectionNames {
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteByte('[')
		output.WriteString(section)
		output.WriteString("]\n")
		for _, entry := range sections[section] {
			if err := writeEntry(entry, entry.path[len(entry.path)-1]); err != nil {
				return nil, err
			}
		}
	}
	if int64(output.Len()) > c.limits.MaxOutputBytes {
		return nil, &ParseError{Err: ErrLimitExceeded}
	}
	return output.Bytes(), nil
}

func flattenObject(object map[string]any, prefix []string, entries *[]encodedEntry, limits Limits, depth int) error {
	if depthExceeded(depth, limits.MaxDepth) {
		return &ParseError{Err: ErrLimitExceeded}
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts, err := parsePath(key, limits.MaxDepth-len(prefix))
		if err != nil {
			return fmt.Errorf("conf: cannot encode key %q: %w", key, err)
		}
		path := append(append([]string(nil), prefix...), parts...)
		value := object[key]
		if nested, ok := value.(map[string]any); ok && len(nested) != 0 && plainSegments(nested) {
			if err := flattenObject(nested, path, entries, limits, depth+1); err != nil {
				return err
			}
			continue
		}
		// A leaf shares the depth budget with its path, exactly as Decode
		// bounds a value by MaxDepth minus the key's depth, so Encode never
		// writes a document Decode would reject
		if nestingDepth(value) > limits.MaxDepth-len(path) {
			return &ParseError{Err: ErrLimitExceeded}
		}
		*entries = append(*entries, encodedEntry{path: path, value: value})
		if len(*entries) > limits.MaxEntries {
			return &ParseError{Err: ErrLimitExceeded}
		}
	}
	return nil
}

// plainSegments reports whether every key of a nested object is a single path
// segment, so the object can be flattened into dotted keys and sections. A key
// containing "." would be split into nested objects, and one parsePath rejects
// cannot be written at all, so such an object is emitted as one JSON leaf
func plainSegments(object map[string]any) bool {
	for key := range object {
		if _, err := parsePath(key, 1); err != nil {
			return false
		}
	}
	return true
}

// nestingDepth counts the container levels of a decoded JSON value: 0 for a
// scalar, 1 for [] or {}, 2 for [[]] and so on
func nestingDepth(value any) int {
	deepest := 0
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			deepest = max(deepest, nestingDepth(child))
		}
	case []any:
		for _, child := range typed {
			deepest = max(deepest, nestingDepth(child))
		}
	default:
		return 0
	}
	return deepest + 1
}
