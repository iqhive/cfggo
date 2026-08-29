package conf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type encodedEntry struct {
	path  []string
	value any
}

// Encode renders canonical JSON as deterministic conf text.
func (c *Codec) Encode(input json.RawMessage) ([]byte, error) {
	if int64(len(input)) > c.limits.MaxInputBytes {
		return nil, ErrLimitExceeded
	}
	value, err := decodeJSONValue(string(input))
	if err != nil {
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
			return ErrLimitExceeded
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
		return nil, ErrLimitExceeded
	}
	return output.Bytes(), nil
}

func flattenObject(object map[string]any, prefix []string, entries *[]encodedEntry, limits Limits, depth int) error {
	if depth > limits.MaxDepth {
		return ErrLimitExceeded
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
		if nested, ok := object[key].(map[string]any); ok {
			if len(nested) == 0 {
				*entries = append(*entries, encodedEntry{path: path, value: nested})
			} else if err := flattenObject(nested, path, entries, limits, depth+1); err != nil {
				return err
			}
		} else {
			*entries = append(*entries, encodedEntry{path: path, value: object[key]})
		}
		if len(*entries) > limits.MaxEntries {
			return ErrLimitExceeded
		}
	}
	return nil
}
