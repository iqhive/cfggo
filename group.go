package cfggo

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/iqhive/cfggo/internal/flags"
	"github.com/iqhive/cfggo/sources"
)

// GroupOption configures a Group.
type GroupOption func(*Group) error

// Group combines independently-defined Structures under optional namespaces.
type Group struct {
	mu            sync.Mutex
	fileHandler   sources.ConfigHandler
	envPrefix     string
	envPrefixSet  bool
	ignoreUnknown bool
	flagSet       *flag.FlagSet
	initialized   bool
	optionError   error
	pendingError  error
	members       []*groupMember
	byNamespace   map[string]*groupMember
}

type groupMember struct {
	namespace string
	parent    interface{}
	options   []Option
	config    *Structure
	bytes     *sources.HandlerBytes
}

// NewGroup creates a configuration group.
func NewGroup(options ...GroupOption) *Group {
	g := &Group{byNamespace: make(map[string]*groupMember)}
	for _, option := range options {
		if err := option(g); err != nil {
			g.optionError = err
		}
	}
	return g
}

// GroupWithFileConfig configures the single file used by the group.
func GroupWithFileConfig(filename string) GroupOption {
	return func(g *Group) error {
		if _, err := os.Stat(filename); err != nil {
			return fmt.Errorf("GroupWithFileConfig: configuration file %q is required but unavailable: %w", filename, err)
		}
		g.fileHandler = sources.NewHandlerFile(filename, false)
		return nil
	}
}

// GroupWithEnvPrefix applies a prefix to members that do not override it.
func GroupWithEnvPrefix(prefix string) GroupOption {
	return func(g *Group) error {
		g.envPrefix, g.envPrefixSet = prefix, true
		return nil
	}
}

// GroupWithIgnoreUnknownVars ignores command-line flags not registered by the group.
func GroupWithIgnoreUnknownVars() GroupOption {
	return func(g *Group) error {
		g.ignoreUnknown = true
		return nil
	}
}

// Register adds a Structure to the group. Registration is allowed only before Init.
func (g *Group) Register(namespace string, config interface{}, options ...Option) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.initialized {
		GlobalLogger().Warn("cfggo: Register called after Group.Init; ignoring member", "namespace", namespace)
		return
	}
	if g.byNamespace == nil {
		g.byNamespace = make(map[string]*groupMember)
	}
	if namespace != "" {
		if _, exists := g.byNamespace[namespace]; exists {
			g.pendingError = fmt.Errorf("duplicate group namespace %q", namespace)
			return
		}
	}
	member := &groupMember{namespace: namespace, parent: config, options: options}
	g.byNamespace[namespace] = member
	g.members = append(g.members, member)
}

func (g *Group) Init() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.optionError != nil {
		return g.optionError
	}
	if g.pendingError != nil {
		return g.pendingError
	}
	if g.initialized {
		return fmt.Errorf("group already initialized")
	}
	if err := g.validateMembers(); err != nil {
		return err
	}
	g.flagSet = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	var raw map[string]interface{}
	if g.fileHandler != nil {
		data, err := g.fileHandler.LoadConfig()
		if err != nil {
			return fmt.Errorf("load group configuration: %w", err)
		}
		if len(data) != 0 {
			if err := json.Unmarshal(data, &raw); err != nil {
				return fmt.Errorf("parse group configuration: %w", err)
			}
		}
	}
	for _, member := range g.members {
		member.config = structureOf(member.parent)
		if member.config == nil {
			return fmt.Errorf("group member %q config must embed cfggo.Structure", member.namespace)
		}
		doc := memberDocument(raw, member, g.members, g.byNamespace)
		bytes, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal %q configuration: %w", member.namespace, err)
		}
		member.bytes = sources.NewHandlerBytes(bytes, false)
		opts := make([]Option, 0, len(member.options)+4)
		if g.envPrefixSet {
			prefix := g.envPrefix
			if member.namespace != "" {
				prefix += strings.ToUpper(member.namespace) + "_"
			}
			opts = append(opts, WithEnvPrefix(prefix))
		}
		opts = append(opts, member.options...)
		opts = append(opts, withBytesConfig(member.bytes), withRegisterOnlyFlagSet(g.flagSet), withFlagNamePrefix(member.namespacePrefix()))
		if err := member.config.Init(member.parent, opts...); err != nil {
			return fmt.Errorf("initialize group member %q: %w", member.namespace, err)
		}
	}
	args := flags.FilterTestFlags(os.Args[1:])
	if g.ignoreUnknown {
		args = filterKnownFlags(g.flagSet, args)
	} else if name, ok := firstUnknownFlag(g.flagSet, args); ok {
		if suggestion := g.suggestFlag(name); suggestion != "" {
			return fmt.Errorf("flag provided but not defined: -%s (did you mean -%s?)", name, suggestion)
		}
		return fmt.Errorf("flag provided but not defined: -%s", name)
	}
	args = normalizeBoolFlagArgs(g.flagSet, args)
	if err := g.flagSet.Parse(args); err != nil {
		return fmt.Errorf("parse group command-line flags: %w", err)
	}
	g.initialized = true
	return nil
}

func structureOf(parent interface{}) *Structure {
	v := reflect.ValueOf(parent)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	field := v.Elem().FieldByName("Structure")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	if s, ok := field.Addr().Interface().(*Structure); ok {
		return s
	}
	return nil
}

func (g *Group) validateMembers() error {
	seenRoot := make(map[string]string)
	namespaces := make(map[string]bool)
	for _, m := range g.members {
		if m.parent == nil || (reflect.ValueOf(m.parent).Kind() == reflect.Pointer && reflect.ValueOf(m.parent).IsNil()) {
			return fmt.Errorf("group member %q config must not be nil", m.namespace)
		}
		if m.namespace != "" {
			namespaces[m.namespace] = true
		}
	}
	for _, m := range g.members {
		if m.namespace != "" {
			continue
		}
		t := reflect.TypeOf(m.parent)
		if t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
			return fmt.Errorf("group member %q config must be a pointer to a struct", m.namespace)
		}
		for key := range planForType(t.Elem(), memberSnakeCaseNames(m)).byKey {
			if previous, ok := seenRoot[key]; ok {
				return fmt.Errorf("root configuration key %q collides between members %q and %q", key, previous, m.namespace)
			}
			seenRoot[key] = m.namespace
			if namespaces[key] {
				return fmt.Errorf("root configuration key %q collides with namespace %q", key, key)
			}
		}
	}
	return nil
}

func memberDocument(raw map[string]interface{}, member *groupMember, orderedMembers []*groupMember, members map[string]*groupMember) map[string]interface{} {
	if member.namespace != "" {
		if doc, ok := raw[member.namespace].(map[string]interface{}); ok {
			return doc
		}
		return map[string]interface{}{}
	}
	doc := make(map[string]interface{}, len(raw))
	firstRoot := member
	for _, candidate := range orderedMembers {
		if candidate.namespace == "" {
			firstRoot = candidate
			break
		}
	}
	t := reflect.TypeOf(member.parent)
	var plan *structPlan
	if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct {
		plan = planForType(t.Elem(), memberSnakeCaseNames(member))
	}
	for key, value := range raw {
		if _, namespaced := members[key]; namespaced && key != "" {
			continue
		}
		if plan != nil {
			matched := false
			for fieldKey := range plan.byKey {
				if fieldKey == key || strings.HasPrefix(fieldKey, key+".") {
					matched = true
					break
				}
			}
			if matched || member == firstRoot {
				doc[key] = value
			}
		}
	}
	return doc
}

func memberSnakeCaseNames(member *groupMember) bool {
	scratch := &Structure{}
	for _, option := range member.options {
		_ = option(scratch)
	}
	return scratch.useSnakeCaseFieldNames()
}

func (m *groupMember) namespacePrefix() string {
	if m.namespace == "" {
		return ""
	}
	return m.namespace + "."
}

func (g *Group) suggestFlag(name string) string {
	best := ""
	bestDistance := 0
	g.flagSet.VisitAll(func(f *flag.Flag) {
		d := editDistance(name, f.Name)
		if best == "" || d < bestDistance || (d == bestDistance && f.Name < best) {
			best, bestDistance = f.Name, d
		}
	})
	if best != "" && bestDistance <= suggestionDistanceLimit(name, best) {
		return best
	}
	return ""
}

// Save writes all member configurations to the group's file once.
func (g *Group) Save() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.saveLocked()
}

func (g *Group) saveLocked() error {
	if g.fileHandler == nil {
		return nil
	}
	root := make(map[string]interface{})
	for _, member := range g.members {
		data, err := member.config.GetJSONBytes()
		if err != nil {
			return err
		}
		var doc map[string]interface{}
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("marshal group member %q: %w", member.namespace, err)
		}
		if member.namespace == "" {
			for key, value := range doc {
				root[key] = value
			}
		} else {
			root[member.namespace] = doc
		}
	}
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}
	if err := g.fileHandler.SaveConfig(data); err != nil {
		return err
	}
	for _, member := range g.members {
		member.config.configMutex.Lock()
		member.config.changed = false
		member.config.configMutex.Unlock()
	}
	return nil
}

// SaveIfChanged writes the group only when any member has changed.
func (g *Group) SaveIfChanged() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, member := range g.members {
		member.config.configMutex.RLock()
		changed := member.config.changed
		member.config.configMutex.RUnlock()
		if changed {
			return g.saveLocked()
		}
	}
	return nil
}

// Reload redispatches the combined document and reloads every member.
func (g *Group) Reload() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fileHandler != nil {
		data, err := g.fileHandler.LoadConfig()
		if err != nil {
			return err
		}
		var raw map[string]interface{}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &raw); err != nil {
				return err
			}
		}
		for _, member := range g.members {
			doc, err := json.Marshal(memberDocument(raw, member, g.members, g.byNamespace))
			if err != nil {
				return err
			}
			member.bytes.Replace(doc)
		}
	}
	for _, member := range g.members {
		if err := member.config.Reload(); err != nil {
			return fmt.Errorf("reload group member %q: %w", member.namespace, err)
		}
	}
	return nil
}

// Diagnose returns concatenated diagnostics prefixed by namespace.
func (g *Group) Diagnose() string { return g.String() }

// String returns concatenated member diagnostics prefixed by namespace.
func (g *Group) String() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var b strings.Builder
	for _, member := range g.members {
		fmt.Fprintf(&b, "%s:\n%s", member.namespace, member.config.String())
	}
	return b.String()
}

// Explain returns concatenated provenance reports prefixed by namespace.
func (g *Group) Explain() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var b strings.Builder
	for _, member := range g.members {
		fmt.Fprintf(&b, "%s:\n%s", member.namespace, member.config.Explain())
	}
	return b.String()
}
