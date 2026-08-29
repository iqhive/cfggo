package conf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type pathNode struct {
	children map[string]*pathNode
	value    bool
	line     int
}

// Decode parses a conf document into canonical JSON.
func (c *Codec) Decode(input []byte) (json.RawMessage, error) {
	return c.decode(input, "")
}

func (c *Codec) decode(input []byte, filename string) (json.RawMessage, error) {
	if int64(len(input)) > c.limits.MaxInputBytes {
		return nil, &ParseError{Filename: filename, Err: ErrLimitExceeded}
	}
	input = bytes.TrimPrefix(input, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(input) {
		return nil, &ParseError{Filename: filename, Err: fmt.Errorf("%w: input is not valid UTF-8", ErrSyntax)}
	}

	root := make(map[string]any)
	paths := &pathNode{children: make(map[string]*pathNode)}
	var section []string
	entries := 0
	lines := bytes.Split(input, []byte{'\n'})
	if len(lines) > c.limits.MaxLines {
		return nil, &ParseError{Filename: filename, Err: ErrLimitExceeded}
	}
	for index, raw := range lines {
		lineNumber := index + 1
		if len(raw) > c.limits.MaxLineBytes {
			return nil, &ParseError{Filename: filename, Line: lineNumber, Err: ErrLimitExceeded}
		}
		line := strings.TrimSuffix(string(raw), "\r")
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = line[:comment]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") || strings.Count(line, "[") != 1 || strings.Count(line, "]") != 1 {
				return nil, syntaxError(filename, lineNumber, 1, "malformed section header")
			}
			name := strings.TrimSpace(line[1 : len(line)-1])
			if name == "global" {
				section = nil
				continue
			}
			parts, err := parsePath(name, c.limits.MaxDepth)
			if err != nil {
				return nil, &ParseError{Filename: filename, Line: lineNumber, Column: 2, Err: err}
			}
			section = parts
			continue
		}

		equals := strings.IndexByte(line, '=')
		if equals < 0 {
			return nil, syntaxError(filename, lineNumber, 1, "expected key=value assignment")
		}
		key := strings.TrimSpace(line[:equals])
		parts, err := parsePath(key, c.limits.MaxDepth-len(section))
		if err != nil {
			return nil, &ParseError{Filename: filename, Line: lineNumber, Column: 1, Err: err}
		}
		fullPath := append(append([]string(nil), section...), parts...)
		valueText := strings.TrimSpace(line[equals+1:])
		value, err := parseValue(valueText)
		if err != nil {
			return nil, &ParseError{Filename: filename, Line: lineNumber, Column: equals + 2, Err: err}
		}
		entries++
		if entries > c.limits.MaxEntries {
			return nil, &ParseError{Filename: filename, Line: lineNumber, Err: ErrLimitExceeded}
		}
		if err := insertPath(root, paths, fullPath, value, lineNumber); err != nil {
			if parseErr, ok := err.(*ParseError); ok {
				parseErr.Filename = filename
				parseErr.Line = lineNumber
			}
			return nil, err
		}
	}

	result, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("conf: encode canonical JSON: %w", err)
	}
	return result, nil
}

func syntaxError(filename string, line, column int, message string) error {
	return &ParseError{Filename: filename, Line: line, Column: column, Err: fmt.Errorf("%w: %s", ErrSyntax, message)}
}

func parsePath(path string, maxDepth int) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty key or section", ErrSyntax)
	}
	parts := strings.Split(path, ".")
	if maxDepth < 1 || len(parts) > maxDepth {
		return nil, ErrLimitExceeded
	}
	for _, part := range parts {
		if strings.TrimSpace(part) != part || part == "" || strings.ContainsAny(part, "#[]=\r\n\x00") {
			return nil, fmt.Errorf("%w: invalid path", ErrSyntax)
		}
	}
	return parts, nil
}

func parseValue(input string) (any, error) {
	if input == "" {
		return "", nil
	}
	structured := strings.HasPrefix(input, `"`) || strings.HasPrefix(input, "[") || strings.HasPrefix(input, "{")
	if structured || input == "null" {
		value, err := decodeJSONValue(input)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid JSON value", ErrSyntax)
		}
		return value, nil
	}
	return input, nil
}

func insertPath(root map[string]any, paths *pathNode, path []string, value any, line int) error {
	node := paths
	current := root
	for index, part := range path {
		if node.value {
			return &ParseError{FirstLine: node.line, Err: ErrKeyConflict}
		}
		child := node.children[part]
		if child == nil {
			child = &pathNode{children: make(map[string]*pathNode)}
			node.children[part] = child
		}
		last := index == len(path)-1
		if last {
			if child.value {
				return &ParseError{FirstLine: child.line, Err: ErrDuplicateKey}
			}
			if len(child.children) != 0 {
				return &ParseError{FirstLine: earliestLine(child), Err: ErrKeyConflict}
			}
			child.value = true
			child.line = line
			current[part] = value
			return nil
		}
		node = child
		nested, ok := current[part].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			current[part] = nested
		}
		current = nested
	}
	return nil
}

func earliestLine(node *pathNode) int {
	if node.line != 0 {
		return node.line
	}
	for _, child := range node.children {
		if line := earliestLine(child); line != 0 {
			return line
		}
	}
	return 0
}
