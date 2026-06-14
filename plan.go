package cfggo

import (
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
	structureType = reflect.TypeOf(Structure{})
	durationType  = reflect.TypeOf(time.Duration(0))
	intType       = reflect.TypeOf(int(0))
	int8Type      = reflect.TypeOf(int8(0))
	int16Type     = reflect.TypeOf(int16(0))
	int32Type     = reflect.TypeOf(int32(0))
	int64Type     = reflect.TypeOf(int64(0))
	uintType      = reflect.TypeOf(uint(0))
	uint8Type     = reflect.TypeOf(uint8(0))
	uint16Type    = reflect.TypeOf(uint16(0))
	uint32Type    = reflect.TypeOf(uint32(0))
	uint64Type    = reflect.TypeOf(uint64(0))
	float32Type   = reflect.TypeOf(float32(0))
	float64Type   = reflect.TypeOf(float64(0))
	stringType    = reflect.TypeOf("")
	boolType      = reflect.TypeOf(false)
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
}

// structPlanCache memoises structPlan by struct type
// Keyed by reflect.Type value *structPlan
var structPlanCache sync.Map

// planForType returns the cached plan for struct type t, building it once
func planForType(t reflect.Type) *structPlan {
	if cached, ok := structPlanCache.Load(t); ok {
		return cached.(*structPlan)
	}

	p := &structPlan{
		byKey:   make(map[string]*planLeaf),
		ignored: make(map[string]bool),
	}
	p.walk(t, "", nil)
	for i := range p.leaves {
		p.byKey[p.leaves[i].info.Key] = &p.leaves[i]
	}

	actual, _ := structPlanCache.LoadOrStore(t, p)
	return actual.(*structPlan)
}

func (p *structPlan) walk(t reflect.Type, prefix string, index []int) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && field.Type == structureType {
			continue
		}

		configVarName := configNameFromField(field)

		// build this field's index path (a fresh slice so children don't alias)
		fieldIndex := make([]int, len(index)+1)
		copy(fieldIndex, index)
		fieldIndex[len(index)] = i

		// A field tagged "-" is excluded from config. Record the dotted
		// Go-field-name key so shouldIgnoreField matches
		// the same key callers would use for it
		if configVarName == "-" {
			key := field.Name
			if prefix != "" {
				key = prefix + "." + field.Name
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

		// Nested struct / *struct fields are config groups: recurse
		if groupT, isPtr, ok := groupType(field); ok {
			if isPtr {
				p.ptrGroups = append(p.ptrGroups, fieldIndex)
			}
			p.walk(groupT, fullKey, fieldIndex)
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
		if ft.Kind() == reflect.Func && ft.NumIn() == 0 && ft.NumOut() == 1 {
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
// inspecting struct tags in priority order: cfggo, cfg, config, json, Name.
// It runs only during one-time plan construction, so it does no caching
func configNameFromField(field reflect.StructField) string {
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
		name = field.Name
	}
	// Strip options after a comma (e.g. `json:"name,omitempty"`).
	if idx := strings.IndexByte(name, ','); idx != -1 {
		name = name[:idx]
	}
	return name
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
	c.defaultData = make(map[string]interface{}, len(c.configData))
	for k, v := range c.configData {
		c.defaultData[k] = v
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
	default:
		return v
	}
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
