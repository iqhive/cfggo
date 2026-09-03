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
	"sync"

	"github.com/iqhive/cfggo/cfglogger"
	iflags "github.com/iqhive/cfggo/internal/flags"
	"github.com/iqhive/cfggo/sources"
)

// Group composes several independent configuration Structures into one process.
//
// A binary that links multiple services together cannot simply call Init once
// per service: each Init creates its own flag.FlagSet and parses os.Args, so
// service A's parser sees service B's flags as unknown, and identically named
// keys ("port") collide across flags, environment variables, and files.
//
// Group solves this by initialising its members with coordinated options: one
// shared flag set parsed exactly once, one configuration file split into a
// section per member, and namespaced flag/environment names:
//
//	group := cfggo.NewGroup(
//	    cfggo.GroupWithFileConfig("combined.json"),
//	    cfggo.GroupWithEnvPrefix("MYAPP_"),
//	)
//	group.Register("auth", authsvc.Cfg)
//	group.Register("billing", billingsvc.Cfg)
//	if err := group.Init(); err != nil { log.Fatal(err) }
//
// For a member registered under namespace "auth", key "port" is read from the
// --auth.port flag, the MYAPP_AUTH_PORT environment variable, and the "auth"
// section of the combined file. Accessors are untouched: authsvc.Cfg.Port()
// keeps working exactly as it does standalone, at the same cost.
//
// Service packages need no changes and stay standalone-compatible: a Group is
// purely an orchestrator around the normal Structure lifecycle.
type Group struct {
	// mu guards registration and initialisation. Operations that call into
	// members (Save, Reload, Validate, Diagnose, ...) take it only to copy the
	// member list, so a member's OnChange callback or auto-save can call back
	// into the group without deadlocking
	mu sync.Mutex

	// saveMu serialises writes through the handler; reloadMu serialises reloads
	saveMu   sync.Mutex
	reloadMu sync.Mutex

	handler     sources.ConfigHandler
	envPrefix   string
	flagSet     *flag.FlagSet
	ownFlagSet  bool
	noFlags     bool
	ignoreFlags bool
	strictKeys  bool
	logger      cfglogger.Logger

	// ignoredSections names top-level sections of the combined document that
	// belong to other programs sharing the file; they are neither reported
	// as unclaimed nor delivered to a root member
	ignoredSections map[string]bool

	members    []*groupMember
	namespaces map[string]int
	regErrs    []error

	initialized bool
}

// groupConfig is satisfied by any struct that embeds cfggo.Structure.
type groupConfig interface {
	Init(interface{}, ...Option) error
	structure() *Structure
}

type groupMember struct {
	namespace string
	parent    groupConfig
	structure *Structure
	options   []Option
	bytes     *sources.HandlerBytes
}

// GroupOption configures a Group.
type GroupOption func(*Group) error

// NewGroup creates an empty Group. Register members on it, then call Init.
func NewGroup(options ...GroupOption) *Group {
	g := &Group{namespaces: make(map[string]int)}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(g); err != nil {
			g.regErrs = append(g.regErrs, err)
		}
	}
	return g
}

// GroupWithFileConfig makes the group load and save one combined configuration
// file holding a section per namespace, eg {"auth": {"port": 8080}}. The file
// must exist.
func GroupWithFileConfig(filename string) GroupOption {
	return groupWithFileConfig(filename, "GroupWithFileConfig", false)
}

// GroupWithDefaultFileConfig is GroupWithFileConfig for an optional file: a
// missing or unreadable file leaves every member on its other sources.
func GroupWithDefaultFileConfig(filename string) GroupOption {
	return groupWithFileConfig(filename, "GroupWithDefaultFileConfig", true)
}

func groupWithFileConfig(filename, funcName string, defaultConfig bool) GroupOption {
	return func(g *Group) error {
		if g.handler != nil {
			return g.wrapError(nil, ErrCodeInvalidArgument, "%s: a configuration source is already set", funcName)
		}
		if _, err := os.Stat(filename); err != nil {
			if defaultConfig {
				g.log().Debug("cfggo: optional group configuration file unavailable", "filename", filename, "err", err)
				return nil
			}
			return g.wrapError(err, ErrCodeNotFound, "%s: configuration file %q is required but unavailable", funcName, filename)
		}
		g.handler = sources.NewHandlerFile(filename, defaultConfig)
		return nil
	}
}

// GroupWithConfigHandler loads and saves the combined configuration document
// through a custom ConfigHandler instead of a file.
func GroupWithConfigHandler(handler sources.ConfigHandler) GroupOption {
	return func(g *Group) error {
		if handler == nil {
			return g.wrapError(nil, ErrCodeInvalidArgument, "GroupWithConfigHandler: handler must not be nil")
		}
		g.handler = handler
		return nil
	}
}

// GroupWithEnvPrefix prepends prefix to the environment variable name of every
// member key, ahead of the namespace: with prefix "MYAPP_", key "port" of
// member "auth" is read from MYAPP_AUTH_PORT.
func GroupWithEnvPrefix(prefix string) GroupOption {
	return func(g *Group) error {
		g.envPrefix = prefix
		return nil
	}
}

// GroupWithFlagSet registers every member's flags on fs instead of a private
// set. The host then owns the single canonical Parse call (commonly
// flag.Parse), exactly as WithFlagSet does for a standalone Structure.
func GroupWithFlagSet(fs *flag.FlagSet) GroupOption {
	return func(g *Group) error {
		if fs == nil {
			return g.wrapError(nil, ErrCodeInvalidArgument, "GroupWithFlagSet: flag set must not be nil")
		}
		g.flagSet = fs
		g.ownFlagSet = false
		return nil
	}
}

// GroupWithStandardFlags registers every member's flags on flag.CommandLine.
func GroupWithStandardFlags() GroupOption {
	return GroupWithFlagSet(flag.CommandLine)
}

// GroupWithoutFlags disables command-line flags for every member.
func GroupWithoutFlags() GroupOption {
	return func(g *Group) error {
		g.noFlags = true
		return nil
	}
}

// GroupWithIgnoreUnknownFlags drops command-line flags that no member defines
// instead of failing Init, so the group can coexist with flags owned elsewhere.
func GroupWithIgnoreUnknownFlags() GroupOption {
	return func(g *Group) error {
		g.ignoreFlags = true
		return nil
	}
}

// GroupWithStrictKeys makes Init fail when the combined document has a
// top-level section that no member claims (usually a misspelt namespace), and
// applies WithStrictKeys to every member so unknown keys inside a section are
// errors too. Without it such a section is only logged, with the closest
// registered namespace suggested; the section is never applied to the
// suggested member.
func GroupWithStrictKeys() GroupOption {
	return func(g *Group) error {
		g.strictKeys = true
		return nil
	}
}

// GroupWithIgnoredSections names top-level sections of the combined document
// that this program does not use, typically because several programs share
// one file and each registers only its own namespaces. Such a section is not
// reported as unclaimed (see GroupWithStrictKeys) and is not delivered to a
// member registered at the root.
func GroupWithIgnoredSections(names ...string) GroupOption {
	return func(g *Group) error {
		if g.ignoredSections == nil {
			g.ignoredSections = make(map[string]bool, len(names))
		}
		for _, name := range names {
			g.ignoredSections[name] = true
		}
		return nil
	}
}

// GroupWithLogger sets the logger used for group-level messages. Members keep
// their own loggers.
func GroupWithLogger(logger cfglogger.Logger) GroupOption {
	return func(g *Group) error {
		g.logger = logger
		return nil
	}
}

// Register adds a configuration to the group under namespace, optionally with
// per-member options (WithValidation, WithLenientLoad, ...) which are applied
// after the group's own options and therefore win over them.
//
// namespace scopes the member's flags (--auth.port), environment variables
// (MYAPP_AUTH_PORT), and file section ({"auth": {...}}). An empty namespace
// merges the member at the root, with no prefix; Init then reports an error if
// two root members (or a root member and a namespace) claim the same name.
//
// Registration problems are reported by Init, so calls can be chained without
// error handling at each site.
func (g *Group) Register(namespace string, cfg interface{}, options ...Option) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.initialized {
		g.regErrs = append(g.regErrs, g.wrapError(nil, ErrCodeInvalidArgument,
			"Register: group is already initialized (namespace %q)", namespace))
		return
	}
	if strings.ContainsAny(namespace, ". ") {
		g.regErrs = append(g.regErrs, g.wrapError(nil, ErrCodeInvalidArgument,
			"Register: namespace %q must not contain '.' or spaces", namespace))
		return
	}
	if cfg == nil || isNilInterfaceValue(cfg) {
		g.regErrs = append(g.regErrs, g.wrapError(nil, ErrCodeInvalidArgument,
			"Register: configuration for namespace %q must not be nil", namespace))
		return
	}
	allocateEmbeddedStructure(cfg)
	member, ok := cfg.(groupConfig)
	if !ok {
		g.regErrs = append(g.regErrs, g.wrapError(nil, ErrCodeInvalidArgument,
			"Register: configuration for namespace %q (%T) must embed cfggo.Structure", namespace, cfg))
		return
	}
	if namespace != "" {
		if _, exists := g.namespaces[namespace]; exists {
			g.regErrs = append(g.regErrs, g.wrapError(nil, ErrCodeInvalidArgument,
				"Register: namespace %q is already registered", namespace))
			return
		}
		g.namespaces[namespace] = len(g.members)
	}
	g.members = append(g.members, &groupMember{
		namespace: namespace,
		parent:    member,
		structure: member.structure(),
		options:   options,
	})
}

// Init initialises every registered member with coordinated options: the
// combined configuration document is split per namespace, environment lookups
// are namespaced, and all flags are registered on one shared flag set which is
// parsed exactly once. It is the group equivalent of calling Init on each
// member, and returns the first failure it encounters. On failure, members
// initialised so far are reverted, so Init can be called again once the
// problem is fixed.
func (g *Group) Init() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.initialized {
		return g.wrapError(ErrAlreadyInitialized, ErrCodeInvalidArgument, "Group.Init: group already initialized")
	}
	if len(g.regErrs) > 0 {
		return g.regErrs[0]
	}
	if len(g.members) == 0 {
		return g.wrapError(nil, ErrCodeInvalidArgument, "Group.Init: no configurations registered")
	}
	if err := g.checkRootCollisions(); err != nil {
		return err
	}

	document, err := g.loadDocument()
	if err != nil {
		return err
	}

	if !g.noFlags && g.flagSet == nil {
		g.flagSet = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		g.ownFlagSet = true
		g.flagSet.Usage = func() {
			fmt.Fprintf(g.flagSet.Output(), "Usage of %s:\n", os.Args[0])
			g.flagSet.PrintDefaults()
		}
	}

	// Members initialised so far are reverted when a later step fails, so the
	// group can be initialised again once the problem is fixed
	var initialised []*groupMember
	fail := func(err error) error {
		for _, member := range initialised {
			member.structure.revertInit()
		}
		if g.ownFlagSet {
			// A private flag set that has already been parsed would be skipped
			// on retry; start from a fresh one
			g.flagSet = nil
			g.ownFlagSet = false
		}
		return err
	}

	for _, member := range g.members {
		if err := g.initMember(member, document); err != nil {
			return fail(err)
		}
		initialised = append(initialised, member)
	}

	if err := g.checkUnclaimedSections(document, g.unclaimedSectionsAreErrors()); err != nil {
		return fail(err)
	}
	if err := g.checkSurfaceCollisions(); err != nil {
		return fail(err)
	}

	// Flags are registered by every member but parsed once, here, so a flag
	// belonging to one member is never "unknown" to another.
	if g.ownFlagSet {
		if err := g.parseFlags(); err != nil {
			return fail(err)
		}
		// Member Init ran its key and validation checks before any flag was
		// parsed, so re-run them now that command-line values are applied.
		for _, member := range g.members {
			if err := g.recheckMember(member); err != nil {
				return fail(err)
			}
		}
	}

	g.initialized = true
	return nil
}

// checkSurfaceCollisions reports two members that would read the same
// environment variable or register the same flag. The namespace prefix cannot
// prevent every clash: env names are derived by upper-casing and mapping "."
// and "-" to "_", so root key "auth_port" and member "auth" key "port" both
// become AUTH_PORT, as do member "auth" key "x_y" and member "auth_x" key "y".
// It runs after member Init, when each member's effective env prefix and flag
// set (which per-member options may change) are known
func (g *Group) checkSurfaceCollisions() error {
	envOwner := make(map[string]string)
	flagOwner := make(map[string]string)
	for _, member := range g.members {
		s := member.structure
		if s.plan == nil {
			continue
		}
		name := g.memberName(member)
		for _, key := range g.memberKeys(member) {
			if !s.skipEnv {
				env := s.envVarName(key)
				if other, exists := envOwner[env]; exists && other != name {
					return g.wrapError(nil, ErrCodeInvalidArgument,
						"Group.Init: environment variable %s would be read for key %q of %s and for a key of %s; rename one of the keys or use a different namespace or prefix",
						env, key, name, other)
				}
				envOwner[env] = name
			}
			if !s.noFlags && s.flagSet != nil && s.flagSet == g.flagSet {
				flagName := s.flagNamePrefix + key
				if other, exists := flagOwner[flagName]; exists && other != name {
					return g.wrapError(nil, ErrCodeInvalidArgument,
						"Group.Init: flag -%s would be registered for key %q of %s and for a key of %s",
						flagName, key, name, other)
				}
				flagOwner[flagName] = name
			}
		}
	}
	return nil
}

// unclaimedSectionsAreErrors reports whether an unclaimed top-level section
// should fail Init: when the group is strict, or when every member was
// initialised with WithStrictKeys (the members' stance is known only after
// their Init)
func (g *Group) unclaimedSectionsAreErrors() bool {
	if g.strictKeys {
		return true
	}
	for _, member := range g.members {
		if !member.structure.strictKeys {
			return false
		}
	}
	return len(g.members) > 0
}

// checkUnclaimedSections reports top-level keys of the combined document that
// no member claims: neither a registered namespace nor the first segment of a
// root member's key. Such a key is almost always a misspelt namespace, so the
// closest registered namespace is named as a suggestion. The suggestion is
// advisory: the section is never applied to the suggested member. Unclaimed
// sections are logged, or returned as an error when strict is set
func (g *Group) checkUnclaimedSections(document map[string]json.RawMessage, strict bool) error {
	if len(document) == 0 {
		return nil
	}
	claimed := make(map[string]bool, len(g.namespaces))
	for namespace := range g.namespaces {
		claimed[namespace] = true
	}
	for _, member := range g.members {
		if member.namespace != "" {
			continue
		}
		for _, key := range g.memberKeys(member) {
			first, _, _ := strings.Cut(key, ".")
			claimed[first] = true
		}
	}

	var problems []string
	for _, key := range sortedDocumentKeys(document) {
		if claimed[key] || g.ignoredSections[key] {
			continue
		}
		message := fmt.Sprintf("section %q matches no registered namespace", key)
		if suggestion := g.suggestNamespace(key); suggestion != "" {
			message += fmt.Sprintf(" (did you mean %q?)", suggestion)
		}
		if strict {
			problems = append(problems, message)
			continue
		}
		g.log().Warn("cfggo: unclaimed configuration section in the combined document; it is ignored", "section", key, "detail", message)
	}
	if len(problems) > 0 {
		return g.wrapError(wrapKind(ErrUnknownKey, fmt.Errorf("%s", strings.Join(problems, "; "))), ErrCodeNotFound,
			"Group.Init: combined configuration has sections no member claims")
	}
	return nil
}

// suggestNamespace returns the registered namespace closest to name, or "" when
// none is a confident match
func (g *Group) suggestNamespace(name string) string {
	best := ""
	bestDistance := 0
	for namespace := range g.namespaces {
		distance := editDistance(name, namespace)
		if best == "" || distance < bestDistance || (distance == bestDistance && namespace < best) {
			best = namespace
			bestDistance = distance
		}
	}
	if best == "" || bestDistance > suggestionDistanceLimit(name, best) {
		return ""
	}
	return best
}

func sortedDocumentKeys(document map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(document))
	for key := range document {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// membersSnapshot returns the member list without holding mu while the members
// are used
func (g *Group) membersSnapshot() []*groupMember {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]*groupMember(nil), g.members...)
}

func (g *Group) initMember(member *groupMember, document map[string]json.RawMessage) error {
	options := make([]Option, 0, len(member.options)+4)

	if g.handler != nil {
		member.bytes = sources.NewHandlerBytes(g.memberDocument(member, document), g.handler.IsDefault())
		// A member's own Save / SaveIfChanged / auto-save persists the combined
		// document rather than silently recording it in memory
		member.bytes.OnSave(g.Save)
		options = append(options, withBytesConfig(member.bytes))
	}
	options = append(options, WithEnvPrefix(g.memberEnvPrefix(member.namespace)))
	if g.strictKeys {
		options = append(options, WithStrictKeys())
	}
	if g.noFlags {
		options = append(options, WithoutFlags())
	} else {
		// Register-only mode: the group owns the single Parse call.
		options = append(options, WithFlagSet(g.flagSet), withFlagNamePrefix(g.memberFlagPrefix(member.namespace)))
	}
	options = append(options, member.options...)

	if err := member.parent.Init(member.parent, options...); err != nil {
		return g.wrapError(err, ErrorCode(err), "Group.Init: member %q failed to initialize", g.memberName(member))
	}
	return nil
}

// recheckMember re-runs the checks that Init performed before the group parsed
// the shared flag set.
func (g *Group) recheckMember(member *groupMember) error {
	s := member.structure
	if err := s.checkUnrecognizedKeys(); err != nil {
		return g.wrapError(err, ErrorCode(err), "Group.Init: member %q has unrecognized configuration keys", g.memberName(member))
	}
	if err := s.validate(); err != nil {
		if s.lenient {
			s.log().Warn("cfggo: configuration validation failed", "err", err)
			return nil
		}
		return g.wrapError(err, ErrorCode(err), "Group.Init: member %q failed validation", g.memberName(member))
	}
	return nil
}

// checkRootCollisions reports members registered at the root whose keys collide
// with another root member's keys or with a namespace, which would otherwise be
// mis-parsed silently.
func (g *Group) checkRootCollisions() error {
	owner := make(map[string]string)
	for _, member := range g.members {
		if member.namespace != "" {
			continue
		}
		name := g.memberName(member)
		for _, key := range g.memberKeys(member) {
			if other, exists := owner[key]; exists {
				return g.wrapError(nil, ErrCodeInvalidArgument,
					"Group.Init: root-namespace key %q is defined by both %s and %s; register one of them under a namespace",
					key, other, name)
			}
			// A root key whose first segment names a namespace ("auth.port"
			// beside namespace "auth") would share that member's --auth.port
			// flag, <PREFIX>AUTH_PORT variable and "auth" file section, so it
			// is as ambiguous as an exact match and is rejected the same way
			segment := key
			if first, _, found := strings.Cut(key, "."); found {
				segment = first
			}
			if _, exists := g.namespaces[segment]; exists {
				return g.wrapError(nil, ErrCodeInvalidArgument,
					"Group.Init: root-namespace key %q of %s collides with namespace %q; register %s under a namespace or rename the field",
					key, name, segment, name)
			}
			owner[key] = name
		}
	}
	return nil
}

// memberKeys returns a member's configuration keys (accessor-backed fields
// only; a plain field the struct happens to carry is not a key) without
// initialising it, by reusing the cached per-type plan.
func (g *Group) memberKeys(member *groupMember) []string {
	parentType := reflect.TypeOf(member.parent)
	for parentType.Kind() == reflect.Ptr {
		parentType = parentType.Elem()
	}
	if parentType.Kind() != reflect.Struct {
		return nil
	}
	s := member.structure
	plan, err := planForType(parentType, s.useSnakeCaseFieldNames())
	if err != nil {
		// The member's own Init reports the unsupported shape
		return nil
	}
	keys := make([]string, 0, len(plan.byKey))
	for key, leaf := range plan.byKey {
		if leaf.info.IsAccessor {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (g *Group) memberEnvPrefix(namespace string) string {
	if namespace == "" {
		return g.envPrefix
	}
	return g.envPrefix + strings.ToUpper(strings.ReplaceAll(namespace, "-", "_")) + "_"
}

func (g *Group) memberFlagPrefix(namespace string) string {
	if namespace == "" {
		return ""
	}
	return namespace + "."
}

func (g *Group) memberName(member *groupMember) string {
	if member.namespace != "" {
		return member.namespace
	}
	return fmt.Sprintf("<root:%T>", member.parent)
}

// loadDocument reads the combined configuration document and splits it into its
// top-level sections.
func (g *Group) loadDocument() (map[string]json.RawMessage, error) {
	if g.handler == nil {
		return nil, nil
	}
	data, err := g.handler.LoadConfig()
	if err != nil {
		if g.handler.IsDefault() {
			g.log().Warn("cfggo: optional group configuration source could not be loaded", "err", err)
			return nil, nil
		}
		return nil, g.wrapError(wrapKind(ErrSource, err), ErrCodeInvalidArgument, "Group: failed to load combined configuration")
	}
	if len(data) == 0 {
		return nil, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		if g.handler.IsDefault() {
			g.log().Warn("cfggo: ignoring malformed group configuration JSON", "err", err)
			return nil, nil
		}
		return nil, g.wrapError(wrapKind(ErrSource, err), ErrCodeInvalidArgument, "Group: failed to parse combined configuration JSON")
	}
	return document, nil
}

// memberDocument returns the slice of the combined document belonging to a
// member: its namespace section, or every non-namespaced top-level key for a
// member registered at the root.
func (g *Group) memberDocument(member *groupMember, document map[string]json.RawMessage) json.RawMessage {
	if document == nil {
		return nil
	}
	if member.namespace != "" {
		return document[member.namespace]
	}

	root := make(map[string]json.RawMessage, len(document))
	for key, value := range document {
		if _, isNamespace := g.namespaces[key]; isNamespace || g.ignoredSections[key] {
			continue
		}
		root[key] = value
	}
	if len(root) == 0 {
		return nil
	}
	data, err := json.Marshal(root)
	if err != nil {
		g.log().Warn("cfggo: failed to assemble root configuration section", "err", err)
		return nil
	}
	return data
}

func (g *Group) parseFlags() error {
	if g.flagSet.Parsed() {
		return nil
	}
	args := iflags.FilterTestFlags(os.Args[1:])

	if g.ignoreFlags {
		args = filterKnownFlagsIn(g.flagSet, args, func(name string) {
			g.log().Debug("cfggo: ignoring unrecognized flag (not defined by any group member)", "flag", name)
		})
	} else if name, ok := firstUnknownFlagIn(g.flagSet, args); ok {
		suffix := ""
		if suggestion := g.suggestFlag(name); suggestion != "" {
			suffix = fmt.Sprintf(" (did you mean -%s?)", suggestion)
		} else if g.flagSet.Usage != nil {
			g.flagSet.Usage()
		}
		return g.wrapError(
			wrapKind(ErrUnknownKey, fmt.Errorf("flag provided but not defined: -%s%s", name, suffix)),
			ErrCodeNotFound, "Group: failed to parse command-line flags")
	}

	args = normalizeBoolFlagArgsIn(g.flagSet, args)

	if err := g.flagSet.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return g.wrapError(err, ErrCodeInvalidArgument, "Group: help requested")
		}
		// Each member scrubs the raw value of its own secret flags out of the
		// flag package's error text before it is returned or logged
		for _, member := range g.members {
			err = member.structure.redactFlagError(err)
		}
		return g.wrapError(err, ErrCodeInvalidArgument, "Group: failed to parse command-line flags")
	}
	if rest := g.flagSet.Args(); len(rest) > 0 {
		g.log().Warn("cfggo: ignoring unexpected positional arguments after flag parsing "+
			"(boolean flags must use the --flag=value form to set an explicit value)", "count", len(rest))
	}
	return nil
}

// suggestFlag maps an unknown flag name onto the closest flag any member
// defines, keeping the namespace prefix intact.
func (g *Group) suggestFlag(name string) string {
	if namespace, key, found := strings.Cut(name, "."); found {
		if index, exists := g.namespaces[namespace]; exists {
			if suggestion := g.members[index].structure.suggestKey(key); suggestion != "" {
				return namespace + "." + suggestion
			}
			return ""
		}
	}
	for _, member := range g.members {
		if member.namespace != "" {
			continue
		}
		if suggestion := member.structure.suggestKey(name); suggestion != "" {
			return suggestion
		}
	}
	return ""
}

// Validate runs every member's registered validators against its current
// values and returns the first failure.
//
// Call it after the host parses an external flag set (GroupWithFlagSet /
// GroupWithStandardFlags): a member's Init defers validation for keys whose
// flag is present on the command line, because their values only arrive when
// the host calls Parse. A group that owns its flag set re-validates every
// member inside Init and does not need this
func (g *Group) Validate() error {
	for _, member := range g.membersSnapshot() {
		if err := member.structure.Validate(); err != nil {
			return g.wrapError(err, ErrorCode(err), "Group.Validate: member %q failed validation", g.memberName(member))
		}
	}
	return nil
}

// Save writes every member's configuration back through the group's handler as
// one combined document, with a section per namespace, and clears every
// member's dirty flag. As for a standalone Structure, values currently
// overridden by the environment or a flag are written with the value the
// document last supplied (or the default), so runtime overrides and
// environment-injected secrets are not persisted. A member's own Save is
// forwarded here
func (g *Group) Save() error {
	g.saveMu.Lock()
	defer g.saveMu.Unlock()
	return g.save()
}

func (g *Group) save() error {
	if g.handler == nil {
		return nil
	}
	members := g.membersSnapshot()
	versions := make([]uint64, len(members))
	document := make(map[string]json.RawMessage, len(members))
	for i, member := range members {
		_, versions[i] = member.structure.changedState()
		data, err := member.structure.persistableJSONBytes()
		if err != nil {
			return g.wrapError(err, ErrorCode(err), "Group.Save: member %q could not be marshalled", g.memberName(member))
		}
		if member.namespace != "" {
			document[member.namespace] = data
			continue
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(data, &root); err != nil {
			return g.wrapError(wrapKind(ErrSource, err), ErrCodeInternal,
				"Group.Save: member %q produced invalid JSON", g.memberName(member))
		}
		for key, value := range root {
			document[key] = value
		}
	}
	data, err := json.Marshal(document)
	if err != nil {
		return g.wrapError(wrapKind(ErrSource, err), ErrCodeInternal, "Group.Save: failed to marshal combined configuration")
	}
	if err := g.handler.SaveConfig(data); err != nil {
		return g.wrapError(wrapKind(ErrSource, err), ErrCodeInvalidArgument, "Group.Save: failed to write combined configuration")
	}
	for i, member := range members {
		member.structure.clearChangedIf(versions[i])
	}
	return nil
}

// SaveIfChanged saves the combined configuration only when at least one member
// has a value changed with Set since it was last saved.
func (g *Group) SaveIfChanged() error {
	g.saveMu.Lock()
	defer g.saveMu.Unlock()

	changed := false
	for _, member := range g.membersSnapshot() {
		memberChanged, _ := member.structure.changedState()
		changed = changed || memberChanged
	}
	if !changed {
		return nil
	}
	return g.save()
}

// Reload re-reads the combined configuration document and reloads every member
// from its section, preserving each member's own reload semantics: rollback on
// failure, command-line and programmatic overrides re-asserted, and OnChange
// callbacks fired per member.
//
// Only the document load and hand-out is serialised; the per-member reloads
// run without a group lock (each member serialises its own), so an OnChange
// callback may itself call Group.Reload, as it may call Structure.Reload.
// Unclaimed sections are logged but never make a reload fail
func (g *Group) Reload() error {
	members := g.membersSnapshot()

	g.reloadMu.Lock()
	document, err := g.loadDocument()
	if err != nil {
		g.reloadMu.Unlock()
		return err
	}
	_ = g.checkUnclaimedSections(document, false)
	for _, member := range members {
		if member.bytes != nil {
			member.bytes.SetData(g.memberDocument(member, document))
		}
	}
	g.reloadMu.Unlock()

	for _, member := range members {
		if err := member.structure.Reload(); err != nil {
			return g.wrapError(err, ErrorCode(err), "Group.Reload: member %q failed to reload", g.memberName(member))
		}
	}
	return nil
}

// Namespaces returns the registered namespaces in registration order. A member
// registered at the root is reported as "".
func (g *Group) Namespaces() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.members))
	for _, member := range g.members {
		out = append(out, member.namespace)
	}
	return out
}

// FlagSet returns the flag set shared by every member, or nil when the group
// was created GroupWithoutFlags and Init has not created one.
func (g *Group) FlagSet() *flag.FlagSet {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.flagSet
}

// Diagnose returns each member's diagnostics keyed by namespace (the root
// member, if any, under ""). It is the structured form of Report.
func (g *Group) Diagnose() map[string]Diagnostics {
	members := g.membersSnapshot()
	out := make(map[string]Diagnostics, len(members))
	for _, member := range members {
		out[member.namespace] = member.structure.DiagnoseData()
	}
	return out
}

// Report returns every member's human-readable diagnostic report, each headed
// by its namespace. Secret values are masked.
func (g *Group) Report() string {
	return g.render(func(member *groupMember) string { return member.structure.Report() })
}

// String returns every member's configuration dump, each headed by its
// namespace. Secret values are masked, so it is safe to log.
func (g *Group) String() string {
	return g.render(func(member *groupMember) string { return member.structure.String() })
}

func (g *Group) render(render func(*groupMember) string) string {
	var sb strings.Builder
	for _, member := range g.membersSnapshot() {
		fmt.Fprintf(&sb, "=== %s ===\n", g.memberName(member))
		sb.WriteString(render(member))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (g *Group) log() cfglogger.Logger {
	if g.logger == nil {
		return GlobalLogger()
	}
	return g.logger
}

func (g *Group) wrapError(err error, code int, msg string, args ...interface{}) error {
	return GlobalErrorWrapper()(err, code, msg, args...)
}
