// Package safefmt renders arbitrary values as text the way fmt's %v does,
// without fmt's unbounded recursion.
//
// fmt recurses once per container level with no cycle detection for maps,
// slices, or the interfaces inside them, so formatting a self-referential
// value exhausts the goroutine stack: a fatal error that no recover can
// catch, killing the whole process. Values cfggo stores can be cyclic (Set
// and NewFlag accept any value for an interface-typed key, and the deep clone
// deliberately preserves cycles), so every path that renders a stored value —
// the human-readable dumps, diagnostics, validation errors, and log
// attributes — must render through this package instead of fmt.
//
// Non-cyclic output matches fmt.Sprintf("%v", value) for the common kinds:
// maps print as map[k:v] with keys sorted, slices as [a b c], structs as
// {a b}, and values with a String or Error method still use it. A container
// that references itself prints <cycle>, and nesting beyond maxDepth prints
// ... so pathological values can neither crash the formatter nor produce a
// megabyte dump.
package safefmt

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	// maxDepth is the container nesting Sprint renders before truncating.
	// Structure deeper than this in a human-readable dump is unreadable
	// noise, and bounding it keeps the render stack trivially small no
	// matter how deeply nested the stored value is.
	maxDepth = 100

	cycleMarker = "<cycle>"
	depthMarker = "..."
)

// visit identifies a mutable container currently being rendered, so a
// reference back to it can be reported as a cycle. The length and capacity
// distinguish slices that share a backing array.
type visit struct {
	typ reflect.Type
	ptr uintptr
	len int
	cap int
}

// Sprint renders value as text. It never panics and always returns.
func Sprint(value any) string {
	var sb strings.Builder
	s := sprinter{seen: make(map[visit]bool)}
	s.render(&sb, reflect.ValueOf(value), 0)
	return sb.String()
}

type sprinter struct {
	seen map[visit]bool
}

// enter registers a container about to render its children and reports
// whether it is already being rendered higher up (a cycle). A container
// stays registered only while its subtree renders, so a value shared by two
// siblings (a DAG, not a cycle) still renders in full both times, as fmt
// prints it. Empty containers are never registered by their callers: they
// have no children to cycle back, and distinct empty slices can share a nil
// backing pointer.
func (s *sprinter) enter(key visit) (cyclic bool) {
	if s.seen[key] {
		return true
	}
	s.seen[key] = true
	return false
}

// exit releases the registration enter added
func (s *sprinter) exit(key visit) {
	delete(s.seen, key)
}

// mapKey and sliceKey identify a map or slice by identity plus extent
func sliceKey(v reflect.Value) visit {
	return visit{typ: v.Type(), ptr: v.Pointer(), len: v.Len(), cap: v.Cap()}
}

func mapKey(v reflect.Value) visit {
	return visit{typ: v.Type(), ptr: v.Pointer()}
}

func pointerKey(v reflect.Value) visit {
	return visit{typ: v.Type(), ptr: v.Pointer()}
}

func (s *sprinter) render(sb *strings.Builder, v reflect.Value, depth int) {
	if !v.IsValid() {
		sb.WriteString("<nil>")
		return
	}

	// A value with its own String or Error method renders itself, exactly as
	// fmt's %v does. The method returns a string, so this cannot recurse,
	// and fmt converts a panic inside it into a %!v(PANIC=...) marker
	if hasStringOrError(v) {
		sb.WriteString(fmt.Sprintf("%v", v.Interface()))
		return
	}

	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			sb.WriteString("<nil>")
			return
		}
		// Unwrapping an interface is not a nesting level, or the interfaces
		// inside every decoded JSON container would halve the depth budget
		s.render(sb, v.Elem(), depth)

	case reflect.Map:
		if v.Len() == 0 {
			// A nil or empty map renders as fmt does, has no children, and
			// cannot take part in a cycle
			sb.WriteString("map[]")
			return
		}
		key := mapKey(v)
		if cyclic := s.enter(key); cyclic || depth >= maxDepth {
			s.marker(sb, cyclic)
			return
		}
		s.renderMap(sb, v, depth)
		s.exit(key)

	case reflect.Slice:
		if v.IsNil() {
			sb.WriteString("[]")
			return
		}
		if v.Len() == 0 {
			sb.WriteString("[]")
			return
		}
		key := sliceKey(v)
		if cyclic := s.enter(key); cyclic || depth >= maxDepth {
			s.marker(sb, cyclic)
			return
		}
		sb.WriteByte('[')
		s.renderElems(sb, v, depth)
		sb.WriteByte(']')
		s.exit(key)

	case reflect.Array:
		if depth >= maxDepth {
			sb.WriteString(depthMarker)
			return
		}
		sb.WriteByte('[')
		s.renderElems(sb, v, depth)
		sb.WriteByte(']')

	case reflect.Pointer:
		if v.IsNil() {
			sb.WriteString("<nil>")
			return
		}
		switch v.Elem().Kind() {
		case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array, reflect.Interface:
			// fmt prints a pointer to an aggregate as & followed by the
			// aggregate; matching that keeps dumps readable
			key := pointerKey(v)
			if cyclic := s.enter(key); cyclic || depth >= maxDepth {
				s.marker(sb, cyclic)
				return
			}
			sb.WriteByte('&')
			s.render(sb, v.Elem(), depth+1)
			s.exit(key)
		default:
			// A pointer to a scalar prints as its address, like fmt
			sb.WriteString(fmt.Sprintf("%v", v.Interface()))
		}

	case reflect.Struct:
		if depth >= maxDepth {
			sb.WriteString(depthMarker)
			return
		}
		sb.WriteByte('{')
		for i := 0; i < v.NumField(); i++ {
			if i > 0 {
				sb.WriteByte(' ')
			}
			field := v.Field(i)
			if !field.CanInterface() {
				// fmt reads unexported fields with unsafe; the safe form is
				// a placeholder so reflect never has to read them
				sb.WriteString("?")
				continue
			}
			s.render(sb, field, depth+1)
		}
		sb.WriteByte('}')

	default:
		// Scalars, channels, and funcs cannot recurse
		sb.WriteString(fmt.Sprintf("%v", v.Interface()))
	}
}

// renderMap writes map[...] with entries sorted by their rendered key, so
// the output is deterministic regardless of map iteration order
func (s *sprinter) renderMap(sb *strings.Builder, v reflect.Value, depth int) {
	entries := make([][2]string, 0, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		var pair [2]string
		var keySB, valSB strings.Builder
		s.render(&keySB, iter.Key(), depth)
		s.render(&valSB, iter.Value(), depth)
		pair[0], pair[1] = keySB.String(), valSB.String()
		entries = append(entries, pair)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i][0] < entries[j][0] })

	sb.WriteString("map[")
	for i, pair := range entries {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(pair[0])
		sb.WriteByte(':')
		sb.WriteString(pair[1])
	}
	sb.WriteByte(']')
}

// renderElems writes slice or array elements separated by spaces
func (s *sprinter) renderElems(sb *strings.Builder, v reflect.Value, depth int) {
	for i := 0; i < v.Len(); i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		s.render(sb, v.Index(i), depth+1)
	}
}

func (s *sprinter) marker(sb *strings.Builder, cyclic bool) {
	if cyclic {
		sb.WriteString(cycleMarker)
	} else {
		sb.WriteString(depthMarker)
	}
}

// hasStringOrError reports whether v's value or its pointer has a String or
// Error method, mirroring the methods fmt's %v consults
func hasStringOrError(v reflect.Value) bool {
	t := v.Type()
	if t.Kind() == reflect.Pointer {
		return implementsStringOrError(t)
	}
	return implementsStringOrError(t) || implementsStringOrError(reflect.PointerTo(t))
}

func implementsStringOrError(t reflect.Type) bool {
	return t.Implements(stringerType) || t.Implements(errorType)
}

var (
	stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	errorType    = reflect.TypeOf((*error)(nil)).Elem()
)
