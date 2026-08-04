package cfggo

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/internal/flags"
	"github.com/iqhive/cfggo/sources"
)

// Group orchestrates multiple cfggo.Structure instances so they can be
// initialised from a single combined configuration file, a shared set of
// command-line flags, and namespaced environment variables. Service packages
// that embed cfggo.Structure continue to work standalone; Group is a purely
// additive composition layer.
type Group struct {
	name       string
	envPrefix  string
	fileConfig string
	fileDefault bool
	handler    sources.ConfigHandler

	members      []groupMember
	memberIndex  map[string]int
	flagSet      *flag.FlagSet
	initialized  bool
	ignoreUnknownVars bool

	logger       cfglogger.Logger
	errorWrapper cfgerror.Wrapper
}

// groupMember holds one registered member of a Group.
type groupMember struct {
	namespace string
	parent    interface{}
	structure *Structure
	options   []Option
}

// GroupOption configures a Group before it is initialised.
type GroupOption func(*Group) error

// GroupWithFileConfig sets the combined configuration file for the group. The
// file must contain a top-level JSON object with one key per registered
// namespace.
func GroupWithFileConfig(filename string) GroupOption {
	return func(g *Group) error {
		g.fileConfig = filename
		return nil
	}
}

// GroupWithDefaultFileConfig sets an optional combined configuration file. If
// the file does not exist, initialisation continues without it.
func GroupWithDefaultFileConfig(filename string) GroupOption {
	return func(g *Group) error {
		g.fileConfig = filename
		g.fileDefault = true
		return nil
	}
}

// GroupWithEnvPrefix sets a global prefix used for every member's environment
// variables. For example, with GroupWithEnvPrefix("MYAPP_") and a member
// registered as "auth", the member's key "port" is read from MYAPP_AUTH_PORT.
func GroupWithEnvPrefix(prefix string) GroupOption {
	return func(g *Group) error {
		g.envPrefix = prefix
		return nil
	}
}

// GroupWithName sets the group's name, used only in log messages and the
// human-readable String output.
func GroupWithName(name string) GroupOption {
	return func(g *Group) error {
		g.name = name
		return nil
	}
}

// NewGroup creates a new, uninitialised Group. Use Register to add members and
// Init to load configuration for all of them.
func NewGroup(options ...GroupOption) *Group {
	g := &Group{
		memberIndex: make(map[string]int),
	}
	for _, opt := range options {
		_ = opt(g) // NewGroup does not return an error; invalid options surface in Init
	}
	return g
}

// Register adds a configuration struct (which must embed cfggo.Structure and be
// a non-nil pointer) to the group under the given namespace. An empty namespace
// merges the member at the root. Per-member options such as WithValidation are
// passed as extra arguments.
func (g *Group) Register(namespace string, cfg interface{}, options ...Option) error {
	if cfg == nil {
		return g.WrapError(nil, ErrCodeInvalidArgument, "Group.Register: cfg must not be nil")
	}

	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return g.WrapError(nil, ErrCodeInvalidArgument, "Group.Register: cfg must be a non-nil pointer to a struct that embeds cfggo.Structure")
	}

	structure, err := g.extractStructure(cfg)
	if err != nil {
		return g.WrapError(err, ErrCodeInvalidArgument, "Group.Register: cfg must embed cfggo.Structure")
	}

	if namespace == "" {
		// Multiple root members are allowed, but their config keys must not
		// collide; this is checked during Init.
	} else {
		if _, exists := g.memberIndex[namespace]; exists {
			return g.WrapError(nil, ErrCodeInvalidArgument, "Group.Register: namespace %q already registered", namespace)
		}
		g.memberIndex[namespace] = len(g.members)
	}

	g.members = append(g.members, groupMember{
		namespace: namespace,
		parent:    cfg,
		structure: structure,
		options:   options,
	})
	return nil
}

// extractStructure returns the embedded *Structure from a pointer to a struct
// that embeds cfggo.Structure. If cfg is already *Structure, it is returned as-is.
func (g *Group) extractStructure(cfg interface{}) (*Structure, error) {
	if s, ok := cfg.(*Structure); ok {
		return s, nil
	}

	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil, errors.New("cfg is not a non-nil pointer")
	}
	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return nil, errors.New("cfg does not point to a struct")
	}

	// The embedded field name is the type name "Structure".
	sf := elem.FieldByName("Structure")
	if !sf.IsValid() {
		return nil, errors.New("struct does not embed cfggo.Structure")
	}
	if !sf.CanAddr() {
		return nil, errors.New("embedded cfggo.Structure is not addressable")
	}
	structure, ok := sf.Addr().Interface().(*Structure)
	if !ok {
		return nil, errors.New("embedded field is not cfggo.Structure")
	}
	return structure, nil
}

// Init initialises every registered member. It loads the combined file once,
// splits it into per-namespace JSON documents, then runs each member's normal
// Structure.Init path with namespaced flags and environment variables.
func (g *Group) Init() error {
	if g.initialized {
		return g.WrapError(ErrAlreadyInitialized, ErrCodeInvalidArgument, "Group.Init: group already initialized")
	}

	if len(g.members) == 0 {
		return g.WrapError(nil, ErrCodeInvalidArgument, "Group.Init: no members registered")
	}

	if g.logger == nil {
		g.logger = GlobalLogger()
	}
	if g.errorWrapper == nil {
		g.errorWrapper = GlobalErrorWrapper()
	}

	if g.flagSet == nil {
		g.flagSet = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		g.flagSet.Usage = func() {
			fmt.Fprintf(g.flagSet.Output(), "Usage of %s:\n", os.Args[0])
			g.flagSet.PrintDefaults()
		}
	}

	// Load the combined file once, if configured.
	combined, err := g.loadCombinedFile()
	if err != nil {
		return g.WrapError(err, ErrCodeInvalidArgument, "Group.Init: failed to load combined configuration file")
	}

	// Initialise each member with a shared FlagSet, namespaced env prefix,
	// and its section of the combined file.
	for i := range g.members {
		m := &g.members[i]

		flagPrefix := ""
		if m.namespace != "" {
			flagPrefix = m.namespace + "."
		}

		envPrefix := g.envPrefix
		if m.namespace != "" {
			envPrefix = g.envPrefix + strings.ToUpper(m.namespace) + "_"
		}

		subDoc := g.memberSubDoc(combined, m.namespace)

		memberOpts := make([]Option, 0, 6+len(m.options))
		memberOpts = append(memberOpts,
			withBytesConfig(subDoc),
			WithEnvPrefix(envPrefix),
			WithFlagSet(g.flagSet),
			withFlagNamePrefix(flagPrefix),
		)
		memberOpts = append(memberOpts, m.options...)

		if err := m.structure.Init(m.parent, memberOpts...); err != nil {
			return g.WrapError(err, ErrorCode(err), "Group.Init: member %q failed to initialize", g.memberName(m))
		}
	}

	// Now that every member has registered its flags on the shared set, parse
	// the command line once. This reuses the Structure bool-flag normalisation
	// and unknown-flag logic.
	if err := g.parseFlags(); err != nil {
		return err
	}

	// Detect key collisions among root (empty-namespace) members.
	if err := g.checkRootCollisions(); err != nil {
		return err
	}

	g.initialized = true
	return nil
}

// loadCombinedFile loads the configured combined file, if any, and returns its
// raw bytes. If no file is configured it returns nil without error.
func (g *Group) loadCombinedFile() ([]byte, error) {
	if g.fileConfig == "" {
		return nil, nil
	}

	info, err := os.Stat(g.fileConfig)
	if err != nil {
		if g.fileDefault && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q is a directory", g.fileConfig)
	}

	if g.handler == nil {
		g.handler = sources.NewHandlerFile(g.fileConfig, g.fileDefault)
	}
	return g.handler.LoadConfig()
}

// memberSubDoc extracts the JSON section for the given namespace from the
// combined file. For an empty namespace (root), it returns the top-level object
// minus any keys that match other registered namespaces.
func (g *Group) memberSubDoc(combined []byte, namespace string) []byte {
	if len(combined) == 0 {
		return nil
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(combined, &top); err != nil {
		// Members will surface the malformed JSON through their normal
		// loadConfig path when subDoc is the full file.
		return combined
	}

	if namespace == "" {
		root := make(map[string]json.RawMessage, len(top))
		for k, v := range top {
			if _, used := g.memberIndex[k]; used {
				continue
			}
			root[k] = v
		}
		if len(root) == 0 {
			return nil
		}
		b, err := json.Marshal(root)
		if err != nil {
			return nil
		}
		return b
	}

	v, ok := top[namespace]
	if !ok {
		return nil
	}
	return v
}

// parseFlags parses the shared flag set once using cfggo's normalisation and
// unknown-flag handling.
func (g *Group) parseFlags() error {
	args := flags.FilterTestFlags(os.Args[1:])

	if g.ignoreUnknownVars {
		g.flagSet.Init(g.flagSet.Name(), flag.ContinueOnError)
		args = filterKnownFlags(g.flagSet, args, g.log())
	} else if name, ok := firstUnknownFlag(g.flagSet, args); ok {
		if suggestion := g.suggestKey(name); suggestion != "" {
			return g.WrapError(
				wrapKind(ErrUnknownKey, fmt.Errorf("flag provided but not defined: -%s (did you mean -%s?)", name, suggestion)),
				ErrCodeNotFound,
				"",
			)
		}
	}

	args = normalizeBoolFlagArgs(g.flagSet, args)

	if err := g.flagSet.Parse(args); err != nil {
		return g.WrapError(err, ErrCodeInvalidArgument, "Group.Init: failed to parse command-line flags")
	}

	if rest := g.flagSet.Args(); len(rest) > 0 {
		g.log().Warn("cfggo: ignoring unexpected positional arguments after flag parsing "+
			"(boolean flags must use the --flag=value form to set an explicit value)", "args", rest)
	}
	return nil
}

// checkRootCollisions reports when two root-namespace members define the same
// configuration key.
func (g *Group) checkRootCollisions() error {
	seen := make(map[string]string) // key -> member name
	var collisions []string
	for _, m := range g.members {
		if m.namespace != "" {
			continue
		}
		keys := m.structure.getAllKeys()
		for _, k := range keys {
			if other, ok := seen[k]; ok {
				collisions = append(collisions, fmt.Sprintf("key %q is defined by both %q and %q", k, other, g.memberName(&m)))
			}
			seen[k] = g.memberName(&m)
		}
	}
	if len(collisions) == 0 {
		return nil
	}
	sort.Strings(collisions)
	return g.WrapError(wrapKind(ErrUnknownKey, errors.New(strings.Join(collisions, "; "))), ErrCodeInvalidArgument,
		"Group.Init: root namespace key collisions")
}

// suggestKey returns the closest known flag name for an unknown flag, searched
// across all member plans.
func (g *Group) suggestKey(name string) string {
	flagName := strings.TrimPrefix(name, "-")
	flagName = strings.TrimPrefix(flagName, "-")

	best := ""
	bestDistance := 0
	for _, m := range g.members {
		if m.structure.plan == nil {
			continue
		}
		prefix := ""
		if m.namespace != "" {
			prefix = m.namespace + "."
		}
		for candidate, leaf := range m.structure.plan.byKey {
			if !leaf.info.IsAccessor {
				continue
			}
			target := prefix + candidate
			distance := editDistance(flagName, target)
			if best == "" || distance < bestDistance || (distance == bestDistance && target < best) {
				best = target
				bestDistance = distance
			}
		}
	}
	if best == "" || bestDistance > suggestionDistanceLimit(flagName, best) {
		return ""
	}
	return best
}

// Save writes the combined configuration for all members through the group's
// configured handler.
func (g *Group) Save() error {
	data, err := g.combinedJSON()
	if err != nil {
		return err
	}
	if g.handler == nil {
		return g.WrapError(ErrNoHandler, ErrCodeInvalidArgument, "Group.Save: no configuration handler configured")
	}
	return g.handler.SaveConfig(data)
}

// SaveIfChanged writes the combined configuration only when at least one member
// has changed since the last save, and clears each member's changed flag on
// success.
func (g *Group) SaveIfChanged() error {
	if g.handler == nil {
		return g.WrapError(ErrNoHandler, ErrCodeInvalidArgument, "Group.SaveIfChanged: no configuration handler configured")
	}

	anyChanged := false
	versions := make([]uint64, len(g.members))
	for i := range g.members {
		m := &g.members[i]
		changed, version := m.structure.changedState()
		versions[i] = version
		if changed {
			anyChanged = true
		}
	}
	if !anyChanged {
		return nil
	}

	data, err := g.combinedJSON()
	if err != nil {
		return err
	}
	if err := g.handler.SaveConfig(data); err != nil {
		return g.WrapError(err, ErrCodeInvalidArgument, "Group.SaveIfChanged: failed to save combined configuration")
	}

	for i := range g.members {
		m := &g.members[i]
		m.structure.clearChangedIf(versions[i])
	}
	return nil
}

// combinedJSON assembles {"namespace": <member JSON>, ...} from each member's
// current configuration.
func (g *Group) combinedJSON() ([]byte, error) {
	combined := make(map[string]json.RawMessage, len(g.members))
	for i := range g.members {
		m := &g.members[i]
		b, err := m.structure.GetJSONBytes()
		if err != nil {
			return nil, g.WrapError(err, ErrCodeInternal, "Group.Save: failed to get JSON for member %q", g.memberName(m))
		}
		if m.namespace == "" {
			// Root members are merged into the top-level object.
			var root map[string]json.RawMessage
			if len(b) > 0 {
				if err := json.Unmarshal(b, &root); err != nil {
					return nil, g.WrapError(err, ErrCodeInternal, "Group.Save: member %q JSON is not an object", g.memberName(m))
				}
			}
			for k, v := range root {
				if _, exists := combined[k]; exists {
					return nil, g.WrapError(nil, ErrCodeInvalidArgument, "Group.Save: root member %q would overwrite key %q", g.memberName(m), k)
				}
				combined[k] = v
			}
			continue
		}
		combined[m.namespace] = b
	}
	data, err := json.Marshal(combined)
	if err != nil {
		return nil, g.WrapError(err, ErrCodeInternal, "Group.Save: failed to marshal combined configuration")
	}
	return data, nil
}

// Reload re-reads the combined file and dispatches updated sub-documents to
// each member's existing Reload path.
func (g *Group) Reload() error {
	combined, err := g.loadCombinedFile()
	if err != nil {
		return g.WrapError(err, ErrCodeInvalidArgument, "Group.Reload: failed to load combined configuration file")
	}

	for i := range g.members {
		m := &g.members[i]
		subDoc := g.memberSubDoc(combined, m.namespace)
		if m.structure.configHandler != nil {
			if h, ok := m.structure.configHandler.(*sources.HandlerBytes); ok {
				h.Data = subDoc
			} else {
				m.structure.configHandler = sources.NewHandlerBytes(subDoc)
			}
		} else {
			m.structure.configHandler = sources.NewHandlerBytes(subDoc)
		}
		if err := m.structure.Reload(); err != nil {
			return g.WrapError(err, ErrorCode(err), "Group.Reload: member %q failed to reload", g.memberName(m))
		}
	}
	return nil
}

// Diagnose returns a source-annotated report for every member, prefixed by
// namespace.
func (g *Group) Diagnose() string {
	var sb strings.Builder
	for i := range g.members {
		m := &g.members[i]
		if m.namespace != "" {
			sb.WriteString("[" + m.namespace + "]\n")
		} else {
			sb.WriteString("[root]\n")
		}
		sb.WriteString(m.structure.Explain())
		sb.WriteString("\n")
	}
	return sb.String()
}

// String returns a human-readable dump of every member's configuration,
// prefixed by namespace.
func (g *Group) String() string {
	var sb strings.Builder
	for i := range g.members {
		m := &g.members[i]
		sb.WriteString("[" + g.memberName(m) + "]\n")
		sb.WriteString(m.structure.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

// memberName returns a printable name for a member.
func (g *Group) memberName(m *groupMember) string {
	if m.namespace != "" {
		return m.namespace
	}
	if m.structure.name != "" {
		return m.structure.name
	}
	return "root"
}

// log returns the group's logger.
func (g *Group) log() cfglogger.Logger {
	if g.logger == nil {
		return GlobalLogger()
	}
	return g.logger
}

// WrapError wraps an error using the group's error wrapper.
func (g *Group) WrapError(err error, errorcode int, msg string, args ...interface{}) error {
	if g.errorWrapper == nil {
		g.errorWrapper = GlobalErrorWrapper()
	}
	return g.errorWrapper(err, errorcode, msg, args...)
}

// SetLogger sets the group's logger.
func (g *Group) SetLogger(logger cfglogger.Logger) {
	g.logger = logger
}

// SetErrorWrapper sets the group's error wrapper.
func (g *Group) SetErrorWrapper(wrapper cfgerror.Wrapper) {
	g.errorWrapper = wrapper
}
