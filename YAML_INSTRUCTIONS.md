# YAML Support Implementation Specification

## 1. Purpose

Implement official YAML configuration support for cfggo without adding third-party
dependencies to the root `github.com/iqhive/cfggo` module.

The implementation must provide:

1. An optional YAML integration module.
2. Strict YAML-to-JSON and JSON-to-YAML conversion APIs.
3. File handlers usable with both `cfggo.Structure` and `cfggo.Group`.
4. A command-line helper for converting configuration files between JSON and
   YAML.
5. Deterministic, secure, and well-tested behavior.

This document is an implementation specification. Follow its required behavior
unless a repository constraint makes it impossible. If a deviation is needed,
document the reason in the pull request and add a test for the chosen behavior.

## 2. Design Decision

Use a nested, opt-in Go module under `yaml/` backed by a mature YAML parser.

The root cfggo module must remain dependency-free. Do not add a YAML dependency
to the root `go.mod`, do not vendor a parser, and do not implement an incomplete
parser in the root module while advertising it as YAML.

The new module path is:

```text
github.com/iqhive/cfggo/yaml
```

Callers should normally alias the package because `yaml` is a format name:

```go
import cfgyaml "github.com/iqhive/cfggo/yaml"
```

Use a maintained, pure-Go YAML implementation with a node-level API. Prefer the
stable release of `go.yaml.in/yaml/v3`. If a stable v4 release exists when this
work begins, evaluate it against the requirements below before choosing it. Do
not select a release candidate merely to use a newer major version.

The parser dependency belongs only in `yaml/go.mod`. The root dependency guard
in `deps_guard_test.go` deliberately skips nested modules and must continue to
pass unchanged.

## 3. Architectural Constraints

### 3.1 Canonical cfggo representation

cfggo's existing handler boundary is canonical JSON:

```go
type ConfigHandler interface {
    IsDefault() bool
    LoadConfig() (json.RawMessage, error)
    SaveConfig(json.RawMessage) error
}
```

Relevant files:

- `sources/sources.go`
- `loadsave.go`
- `group.go`

The YAML integration must translate at this boundary:

```text
Load:
YAML bytes -> strict YAML node validation -> canonical JSON -> cfggo

Save:
cfggo canonical JSON -> validated generic document -> deterministic YAML bytes
```

Do not add YAML parsing to cfggo's reflection plan, field conversion, validation,
reload, or group code. Those systems must continue to operate on the existing
JSON-compatible representation.

### 3.2 Backwards compatibility

Do not change the behavior of these existing APIs:

- `WithFileConfig`
- `WithDefaultFileConfig`
- `GroupWithFileConfig`
- `GroupWithDefaultFileConfig`
- `WithHTTPConfig`
- `sources.ConfigHandler`
- `sources.HandlerFile`

In particular, do not make `WithFileConfig("config.yaml")` infer YAML. Existing
file handling is format-blind but the cfggo core interprets its result as JSON.
Changing this behavior inside a stable v1 API would be surprising and could make
previously invalid inputs valid.

YAML must use explicit APIs from the optional module.

### 3.3 Field names and struct tags

Do not add `yaml` struct tags to cfggo's field-name resolution in `plan.go`.

The logical configuration schema must be independent of the serialization
format. `cfggo:"name"` remains authoritative. Existing `cfg`, `config`, and
`json` fallback behavior remains unchanged.

This is valid:

```go
Port func() int `cfggo:"port" default:"8080"`
```

Do not require this:

```go
Port func() int `yaml:"port"`
```

The YAML document key `port` reaches cfggo as the same logical key as JSON
`{"port": ...}`.

### 3.4 Round-trip guarantee

Promise semantic round trips only. Do not promise presentation-preserving YAML
editing.

Loading and saving may lose:

- Comments.
- Original key order.
- Quote style.
- Scalar style.
- Blank lines.
- Anchors and alias names.
- Merge-key declarations.
- Explicit tags.
- Original nesting if cfggo's canonical data contains dotted keys.

The required guarantee is:

```text
YAML -> cfggo values -> generated YAML -> cfggo values
```

The values after reload must be equivalent. The generated text does not need to
look like the original document.

## 4. Required Repository Layout

Add the following files. Small naming adjustments are allowed only when they
make the public API clearer.

```text
yaml/
  go.mod
  go.sum
  doc.go
  codec.go
  decode.go
  encode.go
  handler.go
  options.go
  errors.go
  codec_test.go
  decode_test.go
  encode_test.go
  handler_test.go
  fuzz_test.go
  testdata/
    valid/
    invalid/
  cmd/
    cfggo-convert/
      main.go
      main_test.go
```

Also update:

```text
.github/workflows/ci.yml
.gitignore
README.md
STRUCTURE.md
```

Do not place the YAML parser dependency in the root `go.mod`.

## 5. Nested Module Setup

Create `yaml/go.mod` with:

```go
module github.com/iqhive/cfggo/yaml

go 1.22.0

require (
    github.com/iqhive/cfggo <compatible-version>
    go.yaml.in/yaml/v3 <stable-version>
)

replace github.com/iqhive/cfggo => ..
```

Implementation notes:

- Determine `<compatible-version>` from the repository's latest compatible
  release. Do not invent a version.
- The local `replace` is required for repository development and CI.
- A dependency's `replace` directive is ignored by downstream main modules, so
  the declared cfggo version must still be valid and published.
- Run `go mod tidy` from `yaml/` after implementation.
- Do not create a root workspace file solely for this feature.
- Before publishing the nested module, confirm the release/tag strategy. Nested
  module tags normally use a path prefix such as `yaml/v1.0.0`.

The package documentation in `yaml/doc.go` must state that this is an optional
module and describe the strict YAML profile.

## 6. Public API

Implement the following public API or a source-compatible equivalent with the
same capabilities.

### 6.1 Format type

```go
type Format string

const (
    FormatAuto Format = "auto"
    FormatJSON Format = "json"
    FormatYAML Format = "yaml"
)
```

`FormatAuto` is allowed only where a filename is available. It must infer:

- `.json` as JSON.
- `.yaml` and `.yml` as YAML.
- Extensions case-insensitively.
- Any missing or unsupported extension as an error.

Never detect a format by trying one parser and silently falling back to another.
Syntax errors must remain deterministic.

### 6.2 Limits

```go
type Limits struct {
    MaxInputBytes    int64
    MaxDepth         int
    MaxNodes         int
    MaxScalarBytes   int
    MaxAliases       int
    MaxExpandedNodes int
}

func DefaultLimits() Limits
```

Use conservative defaults:

```text
MaxInputBytes:    10 MiB
MaxDepth:         100
MaxNodes:         100,000
MaxScalarBytes:   1 MiB
MaxAliases:       100
MaxExpandedNodes: 100,000
```

The exact defaults may be lowered if tests demonstrate that lower limits cover
normal configuration files. Do not raise them without documenting the memory
impact.

Validate limits when constructing a codec. Negative values must return an
error. A zero value means "use the default," not "unlimited." Do not expose an
unlimited mode in the initial release.

### 6.3 Codec options

```go
type Option func(*settings) error

func WithLimits(limits Limits) Option
func WithAliases(enabled bool) Option
func WithMergeKeys(enabled bool) Option
func WithIndent(spaces int) Option
```

Required defaults:

- Aliases enabled, with strict limits.
- Merge keys disabled.
- YAML indentation is two spaces.
- Duplicate keys always rejected and cannot be enabled.
- Custom tags always rejected and cannot be enabled.
- Multiple documents always rejected and cannot be enabled.

`WithIndent` must accept a small safe range, such as 2 through 8. Invalid values
must produce a construction error rather than being silently corrected.

### 6.4 Codec

```go
type Codec struct {
    // unexported fields
}

func New(options ...Option) (*Codec, error)

func (c *Codec) YAMLToJSON(input []byte) (json.RawMessage, error)
func (c *Codec) JSONToYAML(input json.RawMessage) ([]byte, error)
```

Add convenience functions using default settings:

```go
func YAMLToJSON(input []byte) (json.RawMessage, error)
func JSONToYAML(input json.RawMessage) ([]byte, error)
```

All runtime handlers and helper tools must call these APIs. Do not maintain a
second conversion implementation in the CLI.

### 6.5 File handler options

```go
type HandlerOption func(*handlerSettings) error

func ReadOnly() HandlerOption
func RewriteOnSave() HandlerOption
func WithCodec(codec *Codec) HandlerOption
```

### 6.6 File handlers

```go
func NewFileHandler(
    filename string,
    defaultConfig bool,
    options ...HandlerOption,
) (sources.ConfigHandler, error)

func WithFileConfig(filename string, options ...HandlerOption) cfggo.Option
func WithDefaultFileConfig(filename string, options ...HandlerOption) cfggo.Option

func GroupWithFileConfig(filename string, options ...HandlerOption) cfggo.GroupOption
func GroupWithDefaultFileConfig(filename string, options ...HandlerOption) cfggo.GroupOption
```

Expected use:

```go
err := cfggo.Init(config,
    cfgyaml.WithFileConfig("config.yaml", cfgyaml.ReadOnly()),
)
```

And for generated YAML saves:

```go
err := cfggo.Init(config,
    cfgyaml.WithFileConfig("config.yaml", cfgyaml.RewriteOnSave()),
)
```

Group usage:

```go
group := cfggo.NewGroup(
    cfgyaml.GroupWithFileConfig("services.yaml", cfgyaml.ReadOnly()),
)
```

If the existing `GroupOption` construction API differs, adapt the signature to
the actual repository API without modifying unrelated group behavior.

## 7. YAML Profile

The decoder must accept a strict, JSON-compatible YAML 1.2-oriented profile.

### 7.1 Document requirements

- The stream must contain exactly one document.
- An empty stream is equivalent to an empty JSON object only if this matches
  existing cfggo empty-file behavior. Otherwise return a clear empty-document
  error. Pick one behavior and test it for standalone and group handlers.
- The root node must be a mapping.
- A second document marker followed by content is an error.
- Trailing comments and whitespace are allowed.

### 7.2 Mapping requirements

- Every mapping key must resolve to a string scalar.
- Complex keys are rejected.
- Sequence and mapping keys are rejected.
- Duplicate keys are rejected at every nesting level.
- Duplicate detection occurs before conversion to a Go map.
- Report the duplicate key and both useful source locations when available.
- Do not include the duplicate value in the error.

### 7.3 Accepted scalar values

Accept and normalize:

- String.
- Boolean.
- Signed integer.
- Unsigned integer when representable by the canonical path.
- Finite floating-point number.
- Null.

Reject:

- NaN.
- Positive and negative infinity.
- Binary values.
- Sets.
- Ordered maps.
- Arbitrary/custom tags.
- Values that cannot be represented safely in cfggo's canonical JSON path.

Explicit `!!str`, `!!bool`, `!!int`, `!!float`, and `!!null` tags may be
accepted when their value is valid. All non-core tags must be rejected.

### 7.4 Implicit scalar behavior

Use YAML 1.2/JSON-compatible expectations:

- `true` and `false` are booleans.
- `null` and `~` are null if supported by the selected parser's core schema.
- `yes`, `no`, `on`, and `off` must remain strings.
- Timestamp-looking values must remain strings or be rejected. They must not
  silently become `time.Time`.
- Quoting always forces a string.
- Leading-zero and octal behavior must be explicitly tested.

Use the parser node tag and original scalar text rather than unmarshalling
directly into `map[string]any`. Node traversal is required because generic
unmarshal can lose duplicate-key, tag, number, and source-location information.

### 7.5 Anchors and aliases

Aliases are enabled by default but strictly bounded.

- Resolve aliases to copied tree values.
- Do not preserve object identity.
- Reject cycles.
- Count alias references.
- Count nodes produced by expansion.
- Enforce `MaxAliases` and `MaxExpandedNodes` before allocating an unbounded
  result.
- Error messages must identify the alias or location without dumping aliased
  scalar contents.

If the selected parser expands aliases before the implementation can enforce
limits, configure the parser's own limits or disable aliases. Do not rely only
on input byte length to prevent expansion attacks.

### 7.6 Merge keys

Merge keys (`<<`) are disabled by default.

When disabled, encountering one must produce an explicit unsupported-feature
error. It must not be treated as an ordinary key.

If `WithMergeKeys(true)` is implemented:

- The merge value must be an alias to a mapping or a sequence of such aliases.
- Explicit mapping keys override merged keys.
- Duplicate explicit keys remain errors.
- Merge order must be documented and tested.
- The emitted YAML expands the resulting map; it does not reconstruct `<<`.
- Merge expansion counts toward node and alias limits.

It is acceptable to omit `WithMergeKeys` from the first implementation and
reject merge keys unconditionally. If omitted, remove it from the public API and
document the deliberate exclusion.

## 8. YAML-to-JSON Algorithm

Implement `Codec.YAMLToJSON` as follows:

1. Reject input larger than `MaxInputBytes` before parsing.
2. Decode one YAML document into a node tree.
3. Attempt to decode a second document.
4. Return an error if a second non-empty document exists.
5. Validate that the root is a mapping node.
6. Recursively validate and convert nodes into JSON-compatible values.
7. Track depth, node count, scalar byte count, alias count, and expanded node
   count during traversal.
8. Reject duplicate and non-string mapping keys before constructing a Go map.
9. Marshal the validated value to JSON.
10. Return compact canonical JSON as `json.RawMessage`.

Do not expose raw parser errors directly when they reproduce an entire source
line. Wrap errors into a package error containing:

- Operation, such as `decode YAML`.
- Line and column where available.
- A key path where available.
- Error category.
- No configuration value text.

Example acceptable error:

```text
decode YAML at database.password (line 8, column 3): duplicate mapping key
```

Example unacceptable error:

```text
line 8: password: super-secret-production-password
```

### 8.1 Numeric precision prerequisite

Before claiming exact `int64` and `uint64` support, fix cfggo's canonical JSON
decoder in `loadsave.go` to use `json.Decoder.UseNumber` instead of
`json.Unmarshal` into `map[string]interface{}`.

Required root-module work:

```go
decoder := json.NewDecoder(bytes.NewReader(data))
decoder.UseNumber()
if err := decoder.Decode(&rawConfig); err != nil {
    // preserve existing strict/lenient behavior
}
```

Also:

- Reject trailing non-whitespace JSON tokens.
- Extend `internal/convert.ConvertValue` to handle `json.Number` safely for all
  numeric target types.
- Never route integer conversion through `float64` when an exact integer target
  is available.
- Add tests for minimum and maximum signed integers, maximum unsigned integers,
  values above JavaScript's exact integer range, overflow, fractional-to-integer
  conversion, and invalid numbers.
- Preserve current error wrapping and secret redaction behavior.

This work improves JSON itself and must be committed and tested separately from
the YAML adapter where practical.

## 9. JSON-to-YAML Algorithm

Implement `Codec.JSONToYAML` as follows:

1. Reject input larger than `MaxInputBytes`.
2. Decode JSON with `json.Decoder.UseNumber`.
3. Reject trailing non-whitespace JSON values.
4. Require a root object.
5. Validate values recursively against the same JSON-compatible scalar model.
6. Build a YAML node tree explicitly.
7. Sort mapping keys lexicographically at every level.
8. Emit two-space indentation by default.
9. End generated files with exactly one newline.
10. Ensure output can be decoded by `YAMLToJSON` into equivalent values.

Generated YAML must be deterministic. The same JSON input and options must
produce byte-for-byte identical output across runs.

Quote strings when plain YAML could change their meaning, including strings
that resemble:

- Booleans.
- Null.
- Numbers.
- Dates or timestamps.
- YAML directives or document markers.
- Strings with leading/trailing whitespace.
- Strings containing syntax-significant `:` or `#` sequences.
- Empty strings.

It is acceptable to quote all strings in the initial implementation if doing so
substantially reduces ambiguity. Prefer readable output only after correctness
is established.

Do not emit anchors, aliases, merge keys, or custom tags.

## 10. Handler Implementation

### 10.1 Composition

The YAML handler should wrap `sources.HandlerFile` so it inherits current atomic
save and permission behavior.

Conceptually:

```go
type fileHandler struct {
    base     *sources.HandlerFile
    codec    *Codec
    readOnly bool
}

func (h *fileHandler) IsDefault() bool {
    return h.base.IsDefault()
}

func (h *fileHandler) LoadConfig() (json.RawMessage, error) {
    raw, err := h.base.LoadConfig()
    if err != nil {
        return nil, err
    }
    return h.codec.YAMLToJSON(raw)
}

func (h *fileHandler) SaveConfig(data json.RawMessage) error {
    if h.readOnly {
        return ErrReadOnly
    }
    raw, err := h.codec.JSONToYAML(data)
    if err != nil {
        return err
    }
    return h.base.SaveConfig(raw)
}
```

Although `HandlerFile.SaveConfig` accepts `json.RawMessage`, its implementation
is an atomic byte writer and does not parse the argument. Add a short comment in
the YAML handler explaining this deliberate transport composition. Do not
change `sources.ConfigHandler`'s public JSON contract.

### 10.2 Read-only default

YAML handlers must be read-only by default because cfggo cannot preserve YAML
comments and formatting.

`Save`, `SaveIfChanged`, and auto-save must return `ErrReadOnly` if save is
attempted. The error must clearly recommend `RewriteOnSave()`.

`RewriteOnSave()` opts into deterministic generated YAML and potential loss of
presentation details.

### 10.3 File existence behavior

Match the root options:

- `WithFileConfig` requires the file during option application.
- `WithDefaultFileConfig` tolerates a missing or inaccessible optional file in
  the same way as the existing root API.
- Required and optional handlers must report `IsDefault()` consistently.
- Do not silently create a missing required file during load.

Prefer composing the existing cfggo option behavior where possible. If the
root's private `withFileConfig` helper prevents reuse, reproduce only the small
existence/handler-selection behavior in the YAML module and cover it thoroughly
with tests.

### 10.4 Handler collision behavior

Do not use reflection or unsafe access to cfggo internals. The public
`cfggo.WithConfigHandler` API installs a handler unconditionally and does not
expose the root package's private collision checks. Therefore the optional
module cannot exactly reproduce those checks without a small additive root API.

Use one of these approaches, in order:

1. Add narrowly scoped root APIs that install a custom handler using the same
   collision/default-selection rules as the built-in file options. Reuse one
   private implementation for JSON and YAML-facing adapters. This is preferred
   if convenience functions named `WithFileConfig` are exposed by the YAML
   module.
2. Expose only `NewFileHandler` in the initial release and require callers to
   pass it to `cfggo.WithConfigHandler` or `cfggo.GroupWithConfigHandler`.
   Document that ordinary cfggo option ordering applies and the last custom
   handler option wins.

Do not claim collision behavior that the public APIs cannot enforce. Do not
modify `Structure` or `Group` fields from the optional module. Initialization
rollback must remain intact in either approach.

### 10.5 Groups

The same YAML handler must work with `GroupWithConfigHandler`. Group documents
remain a single root mapping whose sections correspond to namespaces.

No YAML-specific changes should be required in `group.go`. Add integration tests
for:

- Namespaced members.
- Root members.
- Required and optional group files.
- Group reload.
- Malformed YAML rollback.
- Deterministic group save when rewrite is enabled.

### 10.6 HTTP support

Do not wrap `sources.HandlerHTTP` in the first implementation. It forces
`Content-Type: application/json` on save, so blindly wrapping it would send YAML
with the wrong media type.

Document file support as the initial scope. A later YAML HTTP handler must:

- Send `Accept: application/yaml, application/x-yaml, text/yaml`.
- Accept documented YAML media types on load.
- Send `Content-Type: application/yaml` on save.
- Preserve all TLS, redirect, context, timeout, and response-size safeguards in
  `sources/http.go`.

Do not duplicate the HTTP security implementation merely to add YAML.

## 11. Errors

Define sentinel errors suitable for `errors.Is`:

```go
var (
    ErrInvalidYAML       = errors.New("invalid YAML configuration")
    ErrInvalidJSON       = errors.New("invalid JSON configuration")
    ErrUnsupported       = errors.New("unsupported YAML feature")
    ErrLimitExceeded     = errors.New("configuration limit exceeded")
    ErrDuplicateKey      = errors.New("duplicate YAML mapping key")
    ErrNonStringKey      = errors.New("YAML mapping key must be a string")
    ErrMultipleDocuments = errors.New("multiple YAML documents are not supported")
    ErrReadOnly          = errors.New("YAML handler is read-only")
)
```

Use a structured error type where useful:

```go
type Error struct {
    Op     string
    Path   string
    Line   int
    Column int
    Err    error
}

func (e *Error) Error() string
func (e *Error) Unwrap() error
```

Error requirements:

- Support `errors.Is` and `errors.As`.
- Include path and location when safe.
- Do not include scalar values.
- Do not include the complete source line.
- Do not log complete input documents.
- Let cfggo wrap handler errors using its existing `ErrSource` behavior.

## 12. Conversion Helper API

In addition to the codec methods, add a general conversion helper used by the
CLI and available to applications:

```go
type ConvertOptions struct {
    From   Format
    To     Format
    Indent int
    Limits Limits
}

func Convert(input []byte, options ConvertOptions) ([]byte, error)

func ConvertFile(inputPath, outputPath string, options ConvertOptions) error
```

Required behavior:

- `From` and `To` must differ.
- `FormatAuto` may infer from a corresponding filename in `ConvertFile`.
- In-memory `Convert` must reject `FormatAuto` because no filename exists.
- JSON-to-YAML uses `Codec.JSONToYAML`.
- YAML-to-JSON uses `Codec.YAMLToJSON` followed by pretty JSON formatting.
- JSON output uses two-space indentation by default and ends with one newline.
- YAML output is deterministic and ends with one newline.
- `ConvertFile` reads the whole input only after checking its size where
  possible.
- `ConvertFile` writes atomically in the destination directory.
- A new output file is mode `0600` because configuration may contain secrets.
- Existing output permissions are preserved.
- Input and output paths resolving to the same file must be rejected unless an
  explicit in-place API is added and implemented atomically.
- No partial output file may remain after failure.
- Errors must never contain configuration contents.

Avoid exporting an `Overwrite` boolean in the library API. The library helper
may atomically replace an explicitly supplied destination. Overwrite policy is a
CLI/user-interface concern.

## 13. Conversion CLI

Implement the command under the nested module. From the repository root run it
with Go's `-C` option because a parent module does not own packages inside a
nested module:

```text
go -C yaml run ./cmd/cfggo-convert
```

When running from inside the nested module:

```text
go run ./cmd/cfggo-convert
```

After installation:

```text
go install github.com/iqhive/cfggo/yaml/cmd/cfggo-convert@latest
```

### 13.1 CLI flags

Support:

```text
-in string
    Input path. Use "-" or omit for stdin.

-out string
    Output path. Use "-" or omit for stdout.

-from string
    Input format: auto, json, or yaml. Default: auto for file input.

-to string
    Output format: json or yaml. Required unless inferred from -out.

-indent int
    Output indentation. Default: 2.

-force
    Allow replacing an existing output file.

-check
    Parse and validate input without writing converted output.

-quiet
    Suppress non-error status messages. Never suppress errors.

-version
    Print tool version information.
```

Use the standard `flag` package. Do not introduce a CLI framework.

### 13.2 Format inference

Rules:

1. Explicit `-from` or `-to` wins.
2. Otherwise infer from the corresponding file extension.
3. Stdin has no extension, so `-from` is required for stdin.
4. Stdout has no extension, so `-to` is required for stdout.
5. `.yaml` and `.yml` mean YAML.
6. `.json` means JSON.
7. Unknown or absent extensions produce a usage error.
8. Do not inspect content to guess its format.

### 13.3 Output safety

- Refuse to overwrite an existing output file unless `-force` is supplied.
- Refuse input and output paths that identify the same file.
- Use atomic replacement when `-force` is supplied.
- New files use mode `0600`.
- Preserve existing mode when replacing a file.
- Write only converted document bytes to stdout.
- Write diagnostics to stderr.
- Do not print input or converted content in diagnostics.
- Handle broken stdout pipes as ordinary write failures without a panic.

### 13.4 Exit codes

Use:

```text
0  Success, including successful -check.
1  Input, parse, conversion, or output failure.
2  Invalid command-line usage.
```

### 13.5 Required CLI examples

Document these examples in the nested module package documentation and the root
README.

Convert a JSON file to YAML:

```sh
cfggo-convert -in config.json -out config.yaml
```

Convert YAML to pretty JSON:

```sh
cfggo-convert -in config.yaml -out config.json
```

Read YAML from stdin and write JSON to stdout:

```sh
cfggo-convert -from yaml -to json < config.yaml > config.json
```

Validate a YAML file without writing output:

```sh
cfggo-convert -in config.yaml -check
```

Replace an existing destination:

```sh
cfggo-convert -in config.json -out config.yaml -force
```

Do not show examples that pass secrets directly as command-line arguments.

## 14. Atomic File Helper

The nested module needs atomic writes for converted output. Prefer one of these
approaches in order:

1. Reuse `sources.HandlerFile.SaveConfig` internally when its behavior exactly
   matches the operation.
2. Add a narrowly scoped exported raw-byte atomic file helper to `sources` only
   if both core and the nested module can use it without weakening the JSON
   handler contract.
3. Implement an unexported equivalent inside the nested module as a last
   resort.

If adding a root helper, use an API such as:

```go
func WriteFileAtomic(filename string, data []byte) error
```

Then refactor `HandlerFile.SaveConfig` to call it. Preserve every current
security property from `sources/file.go`:

- Existing mode is preserved.
- New mode is `0600`.
- Temporary file is created in the destination directory.
- Temporary file is chmodded before writing.
- File contents are synced.
- File is closed before rename.
- Rename is atomic on the target filesystem.
- Destination directory is synced where supported.
- Temporary files are removed on every error path.

Any root helper must have root-module tests and must not add a dependency.

## 15. Testing Specification

Tests are part of the implementation, not follow-up work.

### 15.1 Root-module regression tests

Run and preserve:

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
golangci-lint run ./...
```

Add root tests for the `UseNumber` change and any atomic helper introduced.

### 15.2 YAML decoder tests

Add table-driven tests covering:

- Flat mappings.
- Nested mappings.
- Sequences.
- Sequences of mappings.
- Empty strings.
- Quoted ambiguous strings.
- Literal `true`, `false`, and null.
- `yes`, `no`, `on`, and `off` as strings.
- Integers at signed and unsigned boundaries.
- Integers larger than the exact `float64` range.
- Floats and exponents.
- Negative values.
- Null values.
- Unicode strings.
- Comments.
- Document start/end markers.
- Dotted keys as literal mapping keys.

Add rejection tests for:

- Duplicate keys at root and nested levels.
- Non-string keys.
- Complex keys.
- Multiple documents.
- Empty/non-mapping roots.
- NaN and infinity.
- Custom tags.
- Binary values.
- Sets.
- Invalid syntax.
- Excessive depth.
- Excessive node count.
- Oversized scalar.
- Oversized source.
- Excessive aliases.
- Alias cycles.
- Merge keys when disabled.

Every error test must assert the relevant sentinel with `errors.Is` where
applicable. Tests involving secret-looking values must assert that the error
does not contain those values.

### 15.3 YAML encoder tests

Test:

- Stable key sorting at all levels.
- Stable indentation.
- Exactly one trailing newline.
- Safe quoting of ambiguous strings.
- Root-object requirement.
- Invalid and trailing JSON rejection.
- Numeric precision.
- No anchors, aliases, custom tags, or merge keys in output.
- Byte-for-byte deterministic output over repeated calls.

For each accepted fixture, assert semantic round trip:

```go
json1 := validCanonicalJSON
yamlData := JSONToYAML(json1)
json2 := YAMLToJSON(yamlData)
assertJSONEquivalent(json1, json2)
```

Compare decoded values using `json.Decoder.UseNumber`; do not normalize through
`float64` in test helpers.

### 15.4 Handler integration tests

Create realistic cfggo structs and test:

- Required YAML file load.
- Optional missing YAML file.
- Nested configuration fields.
- Map-valued accessor fields.
- Slice-valued accessor fields.
- Null resetting a known field to typed zero.
- Environment overrides YAML.
- Flags override YAML and environment.
- Strict unknown keys.
- Lenient conversion failure behavior.
- Validation failure behavior.
- Secret conversion errors do not leak values.
- Reload after valid file change.
- Rollback after malformed YAML reload.
- Removed YAML keys revert to defaults on reload.
- Programmatic `Set` overrides survive reload.
- Read-only save returns `ErrReadOnly`.
- Rewrite save is loadable and deterministic.
- New saved files are `0600`.
- Existing file mode is preserved.
- Failed encoding does not replace the original file.
- Concurrent `Set`, reload, and save behavior remains race-free.

### 15.5 Group tests

Test:

- Two namespaced members from one YAML mapping.
- Root and namespaced member combination.
- Group environment and flag precedence.
- Group save and reload.
- Missing optional file.
- Malformed group YAML.
- Namespace value with the wrong shape.
- Duplicate namespace keys.
- Full rollback after reload failure.

### 15.6 CLI tests

Prefer testing a `run(args, stdin, stdout, stderr) int` function rather than
spawning a subprocess for every case. Keep `main` very small.

Test:

- JSON file to YAML file.
- YAML file to JSON file.
- Stdin to stdout.
- Format inference.
- Explicit format overriding extension.
- Missing format for stdin/stdout.
- Unknown extension.
- Existing destination without `-force`.
- Existing destination with `-force`.
- Same input/output file rejection.
- `-check` success and failure.
- Exit codes 0, 1, and 2.
- Diagnostics go to stderr.
- Document data goes only to stdout.
- New destination mode is `0600`.
- Existing mode is preserved.
- Failure leaves no partial destination.
- Error output does not contain secret fixture values.

### 15.7 Fuzzing

Add fuzz targets for:

```go
FuzzYAMLToJSON
FuzzJSONToYAML
FuzzRoundTrip
```

Properties:

- Never panic.
- Never hang indefinitely.
- Respect size and expansion limits.
- Successful YAML conversion produces valid single-value JSON.
- Successful JSON conversion produces YAML accepted by the strict decoder.
- Round-trip accepted JSON remains semantically equivalent.

Seed fuzzers with valid and invalid fixtures, including aliases, duplicate keys,
deep nesting, ambiguous scalars, and large integers.

### 15.8 External conformance

Select relevant cases from the language-independent YAML Test Suite. Record
unsupported cases as intentional profile exclusions. Do not silently skip cases
without an explanation.

Differentially test accepted-profile documents against at least one mature YAML
implementation where practical. Differences must be resolved or documented.

## 16. CI Changes

Extend `.github/workflows/ci.yml` with a separate nested-module job:

```yaml
yaml-module:
  name: YAML module
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@...
    - uses: actions/setup-go@...
      with:
        go-version: stable
        cache-dependency-path: |
          go.mod
          yaml/go.sum
    - name: Verify YAML module dependencies
      working-directory: yaml
      run: go mod tidy -diff
    - name: Test YAML module
      working-directory: yaml
      run: go test -count=1 ./...
    - name: Race test YAML module
      working-directory: yaml
      run: go test -race -count=1 ./...
    - name: Vet YAML module
      working-directory: yaml
      run: go vet ./...
```

Use the action versions already established by the repository rather than
copying the placeholders above.

Also make the root formatting check include nested-module Go files, which the
current `gofmt -l .` already does.

Run golangci-lint against the nested module either in its own invocation or with
a nested-module-specific configuration. Verify that the root dependency guard
still passes and that root `go.mod` remains unchanged.

## 17. Documentation Changes

Update the root README to cover:

- Root cfggo remains JSON-native and dependency-free.
- Official YAML support is opt-in through
  `github.com/iqhive/cfggo/yaml`.
- Basic standalone and group examples.
- Read-only default.
- `RewriteOnSave` and its presentation-loss warning.
- Strict profile exclusions.
- Security limits.
- Conversion CLI examples.
- The fact that `cfggo` tags, not `yaml` tags, define logical keys.

Replace the current FAQ snippet that loosely recommends unmarshalling YAML into
`map[string]any`. That approach is insufficient because it can lose duplicate
keys, tags, source locations, and numeric information.

Update `STRUCTURE.md` with the optional module boundary and conversion flow.

Add package examples that compile as tests:

```go
func ExampleWithFileConfig()
func ExampleGroupWithFileConfig()
func ExampleYAMLToJSON()
func ExampleJSONToYAML()
```

## 18. `.gitignore` Changes

The root `.gitignore` currently ignores `*.json` except selected fixture paths.
Add intentional exceptions for YAML module fixtures and generated golden files
as needed. Do not broadly unignore every JSON or YAML file in the repository.

For example:

```gitignore
!yaml/testdata/**/*.json
!yaml/testdata/**/*.yaml
!yaml/testdata/**/*.yml
```

If YAML files are not globally ignored, only the JSON exception may be needed.
Verify with `git status --ignored` before relying on fixtures in tests.

## 19. Performance Expectations

YAML parsing will be slower than JSON, but configuration loading is not a hot
read path. Optimize for correctness and bounded resource use before raw speed.

Add benchmarks for:

- Small YAML-to-JSON conversion.
- Large flat mapping conversion.
- Deep nested mapping conversion within limits.
- Alias-heavy conversion within limits.
- JSON-to-YAML generation.
- Full cfggo initialization from YAML.
- YAML reload.

Benchmarks must report allocations. Do not weaken validation or limits merely
to improve benchmark numbers.

## 20. Work Sequence

Implement in this order:

1. Add `UseNumber` to the root JSON decoder and exact `json.Number` conversion.
2. Add root regression tests and verify all root CI commands.
3. Add/refactor the atomic byte-write helper if needed.
4. Create the nested YAML module.
5. Implement structured errors and validated settings.
6. Implement strict YAML node traversal and YAML-to-JSON conversion.
7. Implement deterministic JSON-to-YAML conversion.
8. Add codec unit tests and fuzz seeds.
9. Implement read-only and rewrite-capable file handlers.
10. Add standalone cfggo integration tests.
11. Add group integration tests.
12. Implement the shared conversion helper API.
13. Implement the `cfggo-convert` CLI over the shared API.
14. Add CLI tests.
15. Add nested-module CI.
16. Update README, package docs, and `STRUCTURE.md`.
17. Run all verification commands in both modules.

Keep root numeric changes separate from YAML code in reviewable commits if the
user asks for commits. Do not create commits unless explicitly requested.

## 21. Verification Commands

From the repository root:

```sh
gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
golangci-lint run ./...
```

From `yaml/`:

```sh
go mod tidy -diff
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
golangci-lint run ./...
```

Exercise the conversion tool manually from the repository root using temporary
files outside the repository or fixtures under `yaml/testdata`:

```sh
go -C yaml run ./cmd/cfggo-convert \
  -in testdata/valid/basic.json \
  -out /tmp/basic.yaml

go -C yaml run ./cmd/cfggo-convert \
  -in /tmp/basic.yaml \
  -out /tmp/basic.json

go -C yaml run ./cmd/cfggo-convert \
  -in /tmp/basic.yaml \
  -check
```

Compare semantic JSON values after the round trip using a JSON parser, not a
text diff. Also verify output permissions on Unix-like CI systems.

## 22. Acceptance Criteria

The work is complete only when all of the following are true:

- Root `go.mod` has no third-party requirements.
- Existing root APIs retain their current JSON behavior.
- YAML support is available through an explicit optional module.
- YAML decoding uses node-level validation.
- Duplicate keys are rejected.
- Non-string keys are rejected.
- Multiple documents are rejected.
- Custom tags and non-JSON values are rejected.
- Alias expansion and nesting are bounded.
- Large integers are not silently converted through `float64`.
- Runtime YAML loading preserves cfggo precedence, validation, redaction,
  reload, and rollback behavior.
- YAML file handlers are read-only by default.
- Rewrite mode produces deterministic, semantically equivalent YAML.
- New output files use mode `0600` and writes are atomic.
- Standalone and group configurations are covered by integration tests.
- JSON/YAML conversion is available as both a Go API and `cfggo-convert` CLI.
- CLI format inference is extension-based and deterministic.
- CLI does not overwrite files without `-force`.
- Error messages do not expose configuration values.
- Root and nested-module tests, race tests, vet, formatting, and lint all pass.
- README and package documentation explain limitations honestly.

## 23. Explicit Non-Goals

Do not implement these in the initial version:

- Automatic YAML detection in existing root file options.
- Comment-preserving YAML edits.
- Arbitrary YAML tags or constructors.
- Unbounded aliases or graph identity.
- Multiple YAML documents.
- Complex mapping keys.
- A YAML-specific struct-tag naming scheme.
- A global codec registry.
- Blank-import plugins.
- Build-tag-selected parser dependencies.
- A vendored YAML parser.
- YAML-over-HTTP unless transport content types and existing security behavior
  are addressed deliberately.

These exclusions keep the feature predictable, secure, and compatible with
cfggo's dependency-free core.
