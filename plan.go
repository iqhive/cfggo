package cfggo

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

// Frequently-compared reflect.Type values, computed once. Comparing against
// these (rather than calling reflect.TypeOf on every field) keeps plan
// construction allocation-free for the common builtin accessor types
var (
	structureType      = reflect.TypeOf(Structure{})
	structurePtrType   = reflect.PointerTo(structureType)
	emptyInterfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	durationType       = reflect.TypeOf(time.Duration(0))
	intType            = reflect.TypeOf(int(0))
	int8Type           = reflect.TypeOf(int8(0))
	int16Type          = reflect.TypeOf(int16(0))
	int32Type          = reflect.TypeOf(int32(0))
	int64Type          = reflect.TypeOf(int64(0))
	uintType           = reflect.TypeOf(uint(0))
	uint8Type          = reflect.TypeOf(uint8(0))
	uint16Type         = reflect.TypeOf(uint16(0))
	uint32Type         = reflect.TypeOf(uint32(0))
	uint64Type         = reflect.TypeOf(uint64(0))
	float32Type        = reflect.TypeOf(float32(0))
	float64Type        = reflect.TypeOf(float64(0))
	stringType         = reflect.TypeOf("")
	boolType           = reflect.TypeOf(false)
)

// accessorKind classifies an accessor's return type so its read closure can be
// built without reflect.MakeFunc (and read without reflect.Value boxing) for
// the common builtin types. kindReflect is the catch-all that falls back to
// reflect.MakeFunc, preserving correctness for named/custom types
type accessorKind uint8

const (
	kindReflect accessorKind = iota
	kindInt
	kindInt8
	kindInt16
	kindInt32
	kindInt64
	kindUint
	kindUint8
	kindUint16
	kindUint32
	kindUint64
	kindFloat32
	kindFloat64
	kindString
	kindBool
	kindDuration
)

// classify maps an accessor's return type to an accessorKind. It only matches
// exact builtin types: a named type (eg `type Level int`) would make a
// `func() int` closure unassignable to the field's `func() Level`, so those
// fall back to kindReflect
func classify(t reflect.Type) accessorKind {
	switch t {
	case durationType:
		return kindDuration
	case intType:
		return kindInt
	case int8Type:
		return kindInt8
	case int16Type:
		return kindInt16
	case int32Type:
		return kindInt32
	case int64Type:
		return kindInt64
	case uintType:
		return kindUint
	case uint8Type:
		return kindUint8
	case uint16Type:
		return kindUint16
	case uint32Type:
		return kindUint32
	case uint64Type:
		return kindUint64
	case float32Type:
		return kindFloat32
	case float64Type:
		return kindFloat64
	case stringType:
		return kindString
	case boolType:
		return kindBool
	default:
		return kindReflect
	}
}

// planLeaf is the precomputed description of a single leaf (non-group) field
type planLeaf struct {
	info  fieldInfo    // Key/Help/DefaultTag/HasDefault/Type/IsAccessor
	index []int        // field-index path from the root struct to this field
	ftype reflect.Type // the accessor's func type (func() T); nil for non-accessors
	kind  accessorKind // fast-path classification of the accessor return type
	zero  interface{}  // cached zero value of the accessor return type
}

// structPlan is the cached, reflection-free blueprint for a configuration
// struct type. It is computed once per Go type and shared by every Structure
// of that type, so repeated Init calls do no per-field tag parsing or
// StructField copying
type structPlan struct {
	leaves    []planLeaf
	ptrGroups [][]int              // index paths to *struct config groups, pre-order
	byKey     map[string]*planLeaf // lookup by dotted config key
	ignored   map[string]bool      // dotted keys of `-`-tagged fields
	// suspects lists leaf fields that carry an explicit cfggo/cfg/config tag
	// but are not func() T accessors, so they will never back a config value.
	// They are almost always a mistake (eg `Port int` instead of
	// `Port func() int`) and are surfaced as a warning during Init()
	suspects []suspectField
}

// suspectField describes a tagged-but-ignored field for the Init warning.
type suspectField struct {
	Key  string // the dotted config key the tag would have produced
	Type string // the field's Go type, for the diagnostic message
	// Reason, when set, replaces the default "not a func() T accessor"
	// explanation (eg for an unexported field cfggo cannot wire)
	Reason string
}

// structPlanCache memoises structPlan by struct type and fallback naming mode.
var structPlanCache sync.Map

type structPlanCacheKey struct {
	t         reflect.Type
	snakeCase bool
}

// planForType returns the cached plan for struct type t, building it once. It
// fails for struct shapes cfggo cannot represent, such as a config group that
// (directly or through other groups) contains a field of its own type
func planForType(t reflect.Type, snakeCaseFieldNames bool) (*structPlan, error) {
	key := structPlanCacheKey{t: t, snakeCase: snakeCaseFieldNames}
	if cached, ok := structPlanCache.Load(key); ok {
		return cached.(*structPlan), nil
	}

	p := &structPlan{
		byKey:   make(map[string]*planLeaf),
		ignored: make(map[string]bool),
	}
	if err := p.walk(t, "", nil, snakeCaseFieldNames, []reflect.Type{t}); err != nil {
		return nil, err
	}
	for i := range p.leaves {
		p.byKey[p.leaves[i].info.Key] = &p.leaves[i]
	}

	actual, _ := structPlanCache.LoadOrStore(key, p)
	return actual.(*structPlan), nil
}

// isAccessorType reports whether t is the func() T shape that backs a
// configuration value
func isAccessorType(t reflect.Type) bool {
	return t.Kind() == reflect.Func && t.NumIn() == 0 && t.NumOut() == 1
}

// walk records every leaf of t under prefix. path holds the struct types
// currently being walked, root first, and is used to reject recursive shapes:
// without it a group such as `Next *Node` inside Node would recurse until the
// process ran out of stack
func (p *structPlan) walk(t reflect.Type, prefix string, index []int, snakeCaseFieldNames bool, path []reflect.Type) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && (field.Type == structureType || field.Type == structurePtrType) {
			continue
		}

		configVarName := configNameFromField(field, snakeCaseFieldNames)

		// build this field's index path (a fresh slice so children don't alias)
		fieldIndex := make([]int, len(index)+1)
		copy(fieldIndex, index)
		fieldIndex[len(index)] = i

		// A field tagged "-" is excluded from config. Record the dotted
		// fallback key so shouldIgnoreField matches the same key callers would
		// use for it under the current naming mode
		if configVarName == "-" {
			key := fallbackConfigName(field, snakeCaseFieldNames)
			if prefix != "" {
				key = prefix + "." + key
			}
			p.ignored[key] = true
			continue
		}
		if configVarName == "" {
			continue
		}

		fullKey := configVarName
		if prefix != "" {
			fullKey = prefix + "." + configVarName
		}

		// An unexported field cannot be written through reflection, so cfggo
		// could register a key, flag and env var for it but never install the
		// accessor; calling the field would then dereference a nil func. Skip
		// such fields entirely (an unexported *embedded* struct is still
		// walked, because its exported fields remain settable) and warn when
		// the field was evidently meant to be a configuration value
		if field.PkgPath != "" && !field.Anonymous {
			if isAccessorType(field.Type) || hasExplicitConfigTag(field) {
				p.suspects = append(p.suspects, suspectField{
					Key:    fullKey,
					Type:   field.Type.String(),
					Reason: "field is unexported, so cfggo cannot install its accessor (export the field)",
				})
			}
			continue
		}

		// Nested struct / *struct fields are config groups: recurse
		if groupT, isPtr, ok := groupType(field); ok {
			for _, seen := range path {
				if seen == groupT {
					return fmt.Errorf("recursive configuration struct: field %s (key %q) has type %s, "+
						"which already contains this group; cfggo cannot represent unbounded nesting",
						field.Name, fullKey, field.Type)
				}
			}
			if isPtr {
				p.ptrGroups = append(p.ptrGroups, fieldIndex)
			}
			if err := p.walk(groupT, fullKey, fieldIndex, snakeCaseFieldNames, append(path, groupT)); err != nil {
				return err
			}
			continue
		}

		leaf := planLeaf{
			index: fieldIndex,
			info: fieldInfo{
				Key:      fullKey,
				Help:     field.Tag.Get("help"),
				IsSecret: isSecretTag(field.Tag.Get("secret")),
			},
		}
		if dv, ok := field.Tag.Lookup("default"); ok {
			leaf.info.DefaultTag = dv
			leaf.info.HasDefault = true
		}

		// Only zero-arg, single-return funcs (func() T) back a config value
		ft := field.Type
		if isAccessorType(ft) {
			out := ft.Out(0)
			leaf.info.IsAccessor = true
			leaf.info.Type = out
			leaf.ftype = ft
			leaf.kind = classify(out)
			leaf.zero = reflect.Zero(out).Interface()
		} else if hasExplicitConfigTag(field) {
			// A field with an explicit cfggo/cfg/config tag that is not a
			// func() T accessor is silently dropped from the config map, which
			// is a common and confusing mistake. Record it so Init() can warn
			p.suspects = append(p.suspects, suspectField{Key: fullKey, Type: ft.String()})
		}

		p.leaves = append(p.leaves, leaf)
	}
	return nil
}

// groupType reports whether field is a nested config group (a struct or
// pointer-to-struct, excluding the embedded Structure) and returns the struct
// element type to recurse into and whether the field is a pointer
func groupType(field reflect.StructField) (t reflect.Type, isPtr bool, ok bool) {
	if field.Anonymous && field.Type == structureType {
		return nil, false, false
	}
	ft := field.Type
	if ft.Kind() == reflect.Ptr {
		elem := ft.Elem()
		if elem.Kind() == reflect.Struct && elem != structureType {
			return elem, true, true
		}
		return nil, false, false
	}
	if ft.Kind() == reflect.Struct && ft != structureType {
		return ft, false, true
	}
	return nil, false, false
}

// isSecretTag reports whether a `secret` struct tag value marks the field as
// sensitive. A truthy value (true/1/yes/y/on, case-insensitive) counts;
// anything else (including an absent or empty tag) is treated as not secret
func isSecretTag(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// hasExplicitConfigTag reports whether the field carries a cfggo-family config
// tag (cfggo/cfg/config). The json tag is deliberately excluded: many structs
// carry json tags for serialization reasons unrelated to cfggo, so warning on
// those would be noisy
func hasExplicitConfigTag(field reflect.StructField) bool {
	if _, ok := field.Tag.Lookup("cfggo"); ok {
		return true
	}
	if _, ok := field.Tag.Lookup("cfg"); ok {
		return true
	}
	if _, ok := field.Tag.Lookup("config"); ok {
		return true
	}
	return false
}

// configNameFromField returns the config map key for a struct field by
// inspecting struct tags in priority order: cfggo, cfg, config, json, field
// name. The field-name fallback uses snake_case only when enabled.
// It runs only during one-time plan construction, so it does no caching
func configNameFromField(field reflect.StructField, snakeCaseFieldNames bool) string {
	name := field.Tag.Get("cfggo")
	if name == "" {
		name = field.Tag.Get("cfg")
	}
	if name == "" {
		name = field.Tag.Get("config")
	}
	if name == "" {
		name = field.Tag.Get("json")
	}
	if name == "" {
		name = fallbackConfigName(field, snakeCaseFieldNames)
	}
	// Strip options after a comma (e.g. `json:"name,omitempty"`).
	if idx := strings.IndexByte(name, ','); idx != -1 {
		name = name[:idx]
	}
	return name
}

func fallbackConfigName(field reflect.StructField, snakeCaseFieldNames bool) string {
	if !snakeCaseFieldNames {
		return field.Name
	}
	return toSnakeCase(field.Name)
}

func toSnakeCase(name string) string {
	if name == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(name) + 4)
	var last byte
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '_' {
			if b.Len() > 0 && last != '_' {
				b.WriteByte('_')
				last = '_'
			}
			continue
		}

		if isASCIIUpper(ch) {
			lower := ch + ('a' - 'A')
			if i > 0 && b.Len() > 0 {
				prev := name[i-1]
				nextLower := i+1 < len(name) && isASCIILower(name[i+1])
				if prev != '_' && (!isASCIIUpper(prev) || nextLower) {
					b.WriteByte('_')
				}
			}
			b.WriteByte(lower)
			last = lower
			continue
		}

		b.WriteByte(ch)
		last = ch
	}
	return b.String()
}

func isASCIIUpper(ch byte) bool {
	return ch >= 'A' && ch <= 'Z'
}

func isASCIILower(ch byte) bool {
	return ch >= 'a' && ch <= 'z'
}

// applyPlan uses the cached plan to seed c.configData with defaults and wire
// every accessor func field, in place of the historical four separate
// reflection walks (buildFieldMeta / setupConfigData / setDefaultsFromTags /
// replaceConfigFuncs). All field-index navigation uses precomputed paths via
// reflect.Value.Field, so it never copies a StructField or re-parses a tag
func (c *Structure) applyPlan() error {
	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return c.WrapError(nil, ErrCodeInvalidArgument, "Init: parent must not be a nil pointer")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return c.WrapError(nil, ErrCodeInvalidArgument, "Init: expected a struct, got %s", v.Kind())
	}

	if c.configData == nil {
		c.configData = make(map[string]interface{}, len(c.plan.leaves))
	}

	// Allocate pointer-backed config groups so the accessor fields beneath them
	// are addressable. Pre-order ordering guarantees each group's ancestors are
	// already non-nil before it is reached
	for _, idx := range c.plan.ptrGroups {
		fv := v.FieldByIndex(idx)
		if fv.Kind() == reflect.Ptr && fv.IsNil() && fv.CanSet() {
			fv.Set(reflect.New(fv.Type().Elem()))
		}
	}

	// Seed values and record provenance. This mirrors the original
	// setupConfigData/setDefaultsFromTags and runs without the config lock: a
	// caller-supplied default func is arbitrary code that may itself read
	// config, so it must not run while the lock is held
	for i := range c.plan.leaves {
		leaf := &c.plan.leaves[i]
		if !leaf.info.IsAccessor {
			continue
		}
		fv := v.FieldByIndex(leaf.index)

		if !fv.IsNil() && fv.CanInterface() {
			// A caller-supplied default func wins; call it for the initial value
			if err := c.set(leaf.info.Key, fv.Call(nil)[0].Interface()); err != nil {
				c.log().Warn("cfggo: failed to set value", "key", leaf.info.Key, "err", err)
			}
		} else {
			if err := c.set(leaf.info.Key, leaf.zero); err != nil {
				c.log().Warn("cfggo: failed to set default value", "key", leaf.info.Key, "err", err)
			}
			if leaf.info.HasDefault && leaf.info.DefaultTag != "" {
				if val, err := iconvert.ConvertString(leaf.info.DefaultTag, leaf.info.Type, c); err != nil {
					return c.WrapError(err, ErrCodeInvalidArgument, "invalid default value for key %q", leaf.info.Key)
				} else if err := c.set(leaf.info.Key, val); err != nil {
					c.log().Warn("cfggo: could not apply default value", "key", leaf.info.Key, "err", err)
				}
			}
		}
		c.recordSourceLocked(leaf.info.Key, SourceDefault)
	}
	// Keep an independent copy of every default so the snapshot Reload
	// restores from can never be reached through a value handed to a caller
	c.defaultData = make(map[string]interface{}, len(c.configData))
	for k, v := range c.configData {
		c.defaultData[k] = cloneMutableInterface(v)
	}

	// Wire the accessor func fields. This takes the write lock to match the
	// original replaceConfigFuncs contract: the installed closures read
	// configData under the read lock, so writers (eg a concurrent reload) and
	// this installation must not interleave
	c.configMutex.Lock()
	for i := range c.plan.leaves {
		leaf := &c.plan.leaves[i]
		if !leaf.info.IsAccessor {
			continue
		}
		fv := v.FieldByIndex(leaf.index)
		if fv.CanSet() {
			fv.Set(c.makeAccessor(leaf))
		}
	}
	c.configMutex.Unlock()
	return nil
}

// makeAccessor builds the func value installed into an accessor field. For the
// common builtin return types it returns a statically-typed closure (no
// reflect.MakeFunc, and an allocation-free read), falling back to MakeFunc for
// named or otherwise non-builtin types
func (c *Structure) makeAccessor(leaf *planLeaf) reflect.Value {
	key := leaf.info.Key
	switch leaf.kind {
	case kindInt:
		return reflect.ValueOf(func() int { return readTyped[int](c, key) })
	case kindInt8:
		return reflect.ValueOf(func() int8 { return readTyped[int8](c, key) })
	case kindInt16:
		return reflect.ValueOf(func() int16 { return readTyped[int16](c, key) })
	case kindInt32:
		return reflect.ValueOf(func() int32 { return readTyped[int32](c, key) })
	case kindInt64:
		return reflect.ValueOf(func() int64 { return readTyped[int64](c, key) })
	case kindUint:
		return reflect.ValueOf(func() uint { return readTyped[uint](c, key) })
	case kindUint8:
		return reflect.ValueOf(func() uint8 { return readTyped[uint8](c, key) })
	case kindUint16:
		return reflect.ValueOf(func() uint16 { return readTyped[uint16](c, key) })
	case kindUint32:
		return reflect.ValueOf(func() uint32 { return readTyped[uint32](c, key) })
	case kindUint64:
		return reflect.ValueOf(func() uint64 { return readTyped[uint64](c, key) })
	case kindFloat32:
		return reflect.ValueOf(func() float32 { return readTyped[float32](c, key) })
	case kindFloat64:
		return reflect.ValueOf(func() float64 { return readTyped[float64](c, key) })
	case kindString:
		return reflect.ValueOf(func() string { return readTyped[string](c, key) })
	case kindBool:
		return reflect.ValueOf(func() bool { return readTyped[bool](c, key) })
	case kindDuration:
		return reflect.ValueOf(func() time.Duration { return readTyped[time.Duration](c, key) })
	default:
		outType := leaf.info.Type
		return reflect.MakeFunc(leaf.ftype, func(_ []reflect.Value) []reflect.Value {
			c.configMutex.RLock()
			raw := c.configData[key]
			c.configMutex.RUnlock()
			// A nil value (eg an explicit JSON null, an unset interface{} field,
			// or a failed conversion) yields an invalid reflect.Value, so fall
			// back to the typed zero value
			if raw == nil {
				return []reflect.Value{reflect.Zero(outType)}
			}
			rv := reflect.ValueOf(raw)
			if !rv.Type().AssignableTo(outType) {
				if rv.Type().ConvertibleTo(outType) {
					rv = rv.Convert(outType)
				} else {
					return []reflect.Value{reflect.Zero(outType)}
				}
			}
			rv = cloneMutableReflectValue(rv)
			return []reflect.Value{rv}
		})
	}
}

func cloneMutableReflectValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		cp := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			cp.SetMapIndex(iter.Key(), cloneMutableReflectValue(iter.Value()))
		}
		return cp
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		cp := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			cp.Index(i).Set(cloneMutableReflectValue(v.Index(i)))
		}
		return cp
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		cp := reflect.New(v.Type().Elem())
		cp.Elem().Set(cloneMutableReflectValue(v.Elem()))
		return cp
	case reflect.Interface:
		// A container held behind an interface (eg the nested objects and
		// arrays of a map[string]interface{} decoded from JSON) must be cloned
		// too, or the copy handed to a caller still aliases live config.
		// Pointers behind an interface keep their identity: an interface{}
		// value is a common place to park a shared client or handle
		if v.IsNil() {
			return v
		}
		if elem := v.Elem(); elem.Kind() == reflect.Map || elem.Kind() == reflect.Slice {
			return cloneMutableReflectValue(elem)
		}
		return v
	case reflect.Struct:
		if cp, cloned := cloneStructValue(v); cloned {
			return cp
		}
		return v
	default:
		return v
	}
}

// cloneStructValue copies v and deep-clones its exported map, slice and
// interface-held-container fields (recursively through nested structs), so a
// struct-typed accessor such as func() Options cannot leak a slice that
// aliases live config. Unexported fields are copied as-is because reflection
// cannot assign to them. It reports false, returning v itself, when no field
// needed cloning, which keeps plain value structs (time.Time, ...) free
func cloneStructValue(v reflect.Value) (reflect.Value, bool) {
	t := v.Type()
	var out reflect.Value
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).PkgPath != "" {
			continue
		}
		fv := v.Field(i)
		var cp reflect.Value
		switch fv.Kind() {
		case reflect.Map, reflect.Slice:
			if fv.IsNil() {
				continue
			}
			cp = cloneMutableReflectValue(fv)
		case reflect.Interface:
			if fv.IsNil() {
				continue
			}
			if elem := fv.Elem(); elem.Kind() != reflect.Map && elem.Kind() != reflect.Slice {
				continue
			}
			cp = cloneMutableReflectValue(fv)
		case reflect.Struct:
			nested, cloned := cloneStructValue(fv)
			if !cloned {
				continue
			}
			cp = nested
		default:
			continue
		}
		if !out.IsValid() {
			out = reflect.New(t).Elem()
			out.Set(v)
		}
		out.Field(i).Set(cp)
	}
	if !out.IsValid() {
		return v, false
	}
	return out, true
}

// readTyped reads the live value for key as T. The common case (the stored
// value is already a T) is a lock-protected map lookup and a type assertion
// with no allocation
// The rare slow path converts a compatible stored value
// (eg a numeric of a different width) via reflection
func readTyped[T any](c *Structure, key string) T {
	c.configMutex.RLock()
	raw := c.configData[key]
	c.configMutex.RUnlock()

	if v, ok := raw.(T); ok {
		return v
	}

	var zero T
	if raw == nil {
		return zero
	}

	rv := reflect.ValueOf(raw)
	tt := reflect.TypeOf(&zero).Elem()
	if rv.Type().ConvertibleTo(tt) {
		if cv, ok := rv.Convert(tt).Interface().(T); ok {
			return cv
		}
	}
	return zero
}
