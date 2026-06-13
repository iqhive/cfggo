# cfggo

> **Type-safe, hot-reloadable configuration for Go. No untyped lookups, no boilerplate.**

[![Go Reference](https://pkg.go.dev/badge/github.com/iqhive/cfggo.svg)](https://pkg.go.dev/github.com/iqhive/cfggo)
[![Go Report Card](https://goreportcard.com/badge/github.com/iqhive/cfggo)](https://goreportcard.com/report/github.com/iqhive/cfggo)
[![CI](https://github.com/iqhive/cfggo/actions/workflows/ci.yml/badge.svg)](https://github.com/iqhive/cfggo/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/iqhive/cfggo)](https://github.com/iqhive/cfggo/releases)

`cfggo` simplifies configuration management in Go. It loads config from files,
environment variables, command-line flags, and HTTP endpoints — and gives you
**compile-time type safety** and **runtime hot reloading** without the
stringly-typed key lookups you get from most config libraries.

```go
// No config.GetInt("server_port") — just call the typed function:
port := config.ServerPort() // returns int, checked at compile time
```

> ℹ️ **Used in production at [iQ Hive](https://github.com/iqhive).**

---

## Why cfggo?

The Go config space is crowded. Here's where cfggo fits:

| | **cfggo** | Viper | envconfig | koanf |
|---|:---:|:---:|:---:|:---:|
| Compile-time type-safe access | ✅ | ❌ (`Get*` by string key) | ✅ | ❌ |
| Hot reload at runtime | ✅ | ⚠️ (file watch only) | ❌ | ⚠️ (manual) |
| Files / Env / Flags / HTTP | ✅ | ✅ | Env only | ✅ |
| Built-in validators | ✅ | ❌ | ⚠️ (`required`) | ❌ |
| Plays nicely with stdlib `flag` | ✅ | ⚠️ | ❌ | ⚠️ |
| Dependencies | Minimal | Many | None | Minimal |

**Pick cfggo** if you want strongly-typed config access *and* live reloading from one small dependency.
**Reach for Viper** if you need its huge format ecosystem (YAML/TOML/HCL/etc.) and don't mind untyped lookups.

---

## Table of Contents

- [Installation](#installation)
- [How It Works](#how-it-works)
- [Key Features](#key-features)
- [Basic Example](#basic-example)
- [Struct Tags and Field Naming](#struct-tags-and-field-naming)
- [Configuration Sources and Precedence](#configuration-sources-and-precedence)
- [Supported Types](#supported-types)
- [Environment Variables](#environment-variables)
- [Command-Line Flags](#command-line-flags)
- [Reading and Setting Values at Runtime](#reading-and-setting-values-at-runtime)
- [Saving Configuration](#saving-configuration)
- [Hot Reloading](#hot-reloading)
- [Validation](#validation)
- [Debugging and Error Handling](#debugging-where-did-this-value-come-from)
- [Advanced Features](#advanced-features)
- [Thread Safety](#thread-safety)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)

---

## Installation

```sh
go get github.com/iqhive/cfggo
```

Requires Go 1.21+.

## How It Works

`cfggo` uses a unique approach to configuration management:

1. **Function-based access**: Instead of accessing configuration values directly, you define functions that return the values.
2. **Embedded structure**: Your configuration struct embeds the `cfggo.Structure` type.
3. **Type safety**: The return type of each function defines the type of the configuration value.
4. **Hot reloading**: Configuration can be reloaded at runtime, and all functions will return the updated values.

## Key Features

- **Type-safe configuration**: All configuration values are accessed through strongly-typed functions.
- **Hot reloadable**: Configuration can be reloaded at runtime without restarting your application.
- **Function-based approach**: Configuration values are accessed through functions, not direct field access.
- **Multiple configuration sources**: Load from JSON files, environment variables, command-line flags, and HTTP endpoints — or plug in your own.
- **Command-line flag integration**: Seamless integration with Go's `flag` package, including coexisting with `flag.CommandLine`.
- **Thread-safe**: All operations are protected by per-instance mutexes for concurrent access.
- **Validation support**: Register per-key validators, with a rich set of built-in validators included.
- **Default values**: Specify defaults via struct tags or in code.
- **Automatic type conversion**: Strings, numbers, booleans, slices, maps, `time.Duration`, `time.Time`, and any `encoding.TextUnmarshaler` are converted automatically.
- **Change notifications**: Subscribe to value changes with `OnChange` callbacks.
- **Provenance tracking**: Ask where any value came from (default, file, env, flag, ...) via `Source`/`Explain`.
- **Sentinel errors**: Branch on `errors.Is` against exported sentinels instead of matching strings.

## Basic Example

Here's a simple example of how to use `cfggo`. Defaults are declared once, using
struct tags:

```go
package main

import (
    "fmt"

    "github.com/iqhive/cfggo"
)

// Define your configuration struct
type MyConfig struct {
    cfggo.Structure // Embed the Structure type

    // Define configuration items as functions that return the desired type.
    // The `default` tag supplies the fallback value when nothing else sets it.
    ServerPort   func() int             `cfggo:"server_port" default:"8080" help:"Port for the server to listen on"`
    DatabaseURL  func() string          `cfggo:"db_url" default:"postgres://localhost:5432/mydb" help:"Database connection URL"`
    FeatureFlags func() map[string]bool `cfggo:"features" default:"{\"new_ui\": false, \"analytics\": true}" help:"Feature flags"`
    LogLevel     func() string          `cfggo:"log_level" default:"info" help:"Logging level"`
}

func main() {
    config := &MyConfig{}

    // Initialize the configuration.
    // This loads values from a JSON file, environment variables, and command-line flags.
    cfggo.Init(config, cfggo.WithFileConfig("config.json"))

    // Access configuration values through the functions
    fmt.Printf("Server will listen on port %d\n", config.ServerPort())
    fmt.Printf("Database URL: %s\n", config.DatabaseURL())
    fmt.Printf("Log level: %s\n", config.LogLevel())

    // Feature flags are accessed as a map
    if config.FeatureFlags()["new_ui"] {
        fmt.Println("New UI is enabled")
    }
}
```

> **Setting defaults in code instead of tags:** if you prefer not to use struct
> tags, you can supply defaults directly with the `cfggo.DefaultValue` helper:
>
> ```go
> config := &MyConfig{
>     ServerPort:  cfggo.DefaultValue(8080),
>     DatabaseURL: cfggo.DefaultValue("postgres://localhost:5432/mydb"),
>     LogLevel:    cfggo.DefaultValue("info"),
> }
> ```

## Struct Tags and Field Naming

cfggo recognises three struct tags on your config fields:

| Tag | Purpose |
|---|---|
| `cfggo:"name"` | The configuration key (used for file keys, env vars, and flags). |
| `default:"value"` | The fallback value, parsed into the field's type. |
| `help:"text"` | Description shown in `--help` output, `String()`, and `Explain()`. |

**Choosing the config key.** If no `cfggo` tag is present, cfggo falls back —
in order — to the `cfg`, `config`, and `json` tags, and finally to the Go field
name. Comma options (e.g. `json:"name,omitempty"`) are stripped, so existing
JSON-tagged structs work without changes:

```go
type MyConfig struct {
    cfggo.Structure
    Port    func() int    `cfggo:"port"`           // key: "port"
    Host    func() string `json:"host,omitempty"`  // key: "host"
    Timeout func() int                              // key: "Timeout" (field name)
}
```

**Ignoring a field.** Tag a field with `-` to exclude it from configuration
entirely (no key, env var, or flag is created):

```go
Internal func() string `cfggo:"-"` // never loaded or exposed as config
```

## Configuration Sources and Precedence

`cfggo` loads configuration from multiple sources in the following order (later sources override earlier ones):

1. **Struct field defaults**: Set in code using `cfggo.DefaultValue()`.
2. **`default` struct tags**: The `default:"..."` tag value.
3. **Configuration files**: JSON files loaded via options (or any custom `ConfigHandler`).
4. **Environment variables**: Automatically mapped from config keys (e.g., `server_port` → `SERVER_PORT`).
5. **Command-line flags**: Automatically registered based on your struct fields.

## Supported Types

A config field can be a `func() T` for any of the following `T`. cfggo converts
incoming values (JSON, environment strings, flag strings, and `default` tags)
into the field's declared type automatically:

| Category | Types |
|---|---|
| Scalars | `string`, `bool`, all sized `int`/`uint`, `float32`, `float64` |
| Time | `time.Duration` (e.g. `"1h30m"`), `time.Time` (RFC 3339) |
| Slices | `[]string`, `[]int`, `[]bool`, `[]float32`, `[]float64`, ... |
| Maps | `map[string]string`, `map[string]interface{}`, ... |
| Custom | Any type implementing `encoding.TextUnmarshaler`; otherwise JSON-decoded |

**String formats for collections.** When a value arrives as a single string
(common for flags and environment variables), slices accept a JSON array, a
comma-separated list, or a single value; maps accept a JSON object:

```
--tags '["a","b"]'      # JSON array
--tags a,b,c            # comma-separated
--tags single           # single value -> ["single"]
--limits '{"cpu":2}'    # JSON object for a map field
```

**Flexible booleans.** Boolean values accept `true/false`, `t/f`, `yes/no`,
`y/n`, and `1/0` (case-insensitive), from any source.

## Environment Variables

Environment variables are automatically mapped from your configuration keys. The mapping follows these rules:

- Keys are converted to uppercase.
- Dots (`.`) are replaced with underscores (`_`).

For example:

- `server_port` → `SERVER_PORT`
- `db.url` → `DB_URL`

## Command-Line Flags

Command-line flags are automatically registered based on your struct fields. The flag name is the same as the configuration key.

For example:

```
--server_port=8080
--db_url="postgres://localhost:5432/mydb"
--log_level=debug
```

Boolean flags can be used with or without a value:

```
--feature_enabled        # Sets to true
--feature_enabled=true   # Sets to true
--feature_enabled=false  # Sets to false
```

> **Important:** As with the standard `flag` package, boolean flags do **not**
> consume a following space-separated token. `--feature_enabled true` enables the
> flag from its *presence* and leaves `true` as a stray positional argument. To
> set a boolean explicitly always use the `--feature_enabled=value` form. cfggo
> logs a warning when unexpected positional arguments remain after parsing, since
> that almost always indicates a `--bool value` mistake.

### Parsing model and interop with the standard `flag` package

By default, cfggo creates its own private `*flag.FlagSet` and parses
`os.Args` for you inside `Init()`. In this mode:

- You do **not** need to (and should **not**) call `flag.Parse()` yourself.
- A flag that cfggo doesn't define (e.g. a typo, or one registered by another
  library on `flag.CommandLine`) prints usage and terminates the process
  (`flag.ExitOnError`). Pass `cfggo.WithIgnoreUnknownVars()` to instead ignore
  unrecognized flags (and their separate values) and continue:

  ```go
  cfggo.Init(config, cfggo.WithIgnoreUnknownVars())
  ```

If you want cfggo to coexist with the idiomatic "register flags, call
`flag.Parse()` once" pattern, register cfggo's flags on a flag set you control
and own the single parse call yourself:

```go
config := &MyConfig{ /* ... */ }

// Register cfggo's flags on the process-global flag.CommandLine.
cfggo.Init(config, cfggo.WithStandardFlags())
// (equivalent: cfggo.Init(config, cfggo.WithFlagSet(flag.CommandLine)))

// The host performs the one canonical parse; cfggo flags AND any other
// library's flags (glog/klog, OpenTelemetry, testing, etc.) resolve together.
flag.Parse()
```

When an external flag set is supplied via `WithFlagSet`/`WithStandardFlags`,
cfggo registers its flags but does **not** parse during `Init()` — the host owns
the single `Parse()` call. cfggo flag values still propagate automatically as the
host parses, because each flag writes directly into the config map.

## Reading and Setting Values at Runtime

The typed accessor functions (`config.ServerPort()`) are the primary way to read
values, but cfggo also exposes a dynamic key/value API for code that works with
keys at runtime (admin endpoints, tooling, tests):

```go
// Read a value by key (second return reports whether the key exists).
if v, ok := config.Get("server_port"); ok {
    fmt.Println("port is", v)
}

// Set a value by key. The value is converted to the field's type, the change
// is recorded with SourceSet provenance, and any OnChange callbacks fire.
if err := config.Set("server_port", 9090); err != nil {
    log.Fatal(err)
}

// The change is visible through the typed accessor immediately:
fmt.Println(config.ServerPort()) // 9090
```

## Saving Configuration

When a `ConfigHandler` is configured (e.g. via `WithFileConfig`), you can persist
the current configuration back to it:

```go
// Always write the current configuration through the handler.
if err := config.Save(); err != nil {
    log.Fatal(err)
}

// Write only if something changed since the last load/save (clears the dirty flag).
if err := config.SaveIfChanged(); err != nil {
    log.Fatal(err)
}
```

You can also inspect the configuration without a handler:

```go
raw := config.GetJSONBytes()   // []byte JSON snapshot of all values
fmt.Println(config.String())   // aligned key: value listing with help text
```

To save automatically on shutdown, see [`WithAutoSave`](#configuration-options).

## Hot Reloading

One of the key features of `cfggo` is the ability to reload configuration at runtime. This is useful for applications that need to change configuration without restarting.

```go
// Reload configuration from all sources
if err := config.ReloadConfig(); err != nil {
    log.Fatalf("Failed to reload configuration: %v", err)
}

// After reloading, all function calls will return the updated values
fmt.Printf("Updated server port: %d\n", config.ServerPort())
```

### Reacting to changes (OnChange)

Register a callback to be notified when configuration values change. Callbacks
fire on `Set` (with the single changed key) and on `Reload`/`ReloadConfig` (with
the aggregate set of keys whose values changed). They do not fire for values
applied during `Init`.

```go
cancel := config.OnChange(func(changedKeys []string) {
    log.Printf("config changed: %v", changedKeys)
    // e.g. re-dial a database, adjust a log level, ...
})
defer cancel() // unregister when you no longer care
```

Callbacks run synchronously after cfggo releases its internal lock, so it is
safe to call `Get`/`Set` from within a callback (avoid infinite `Set` loops).

## Validation

You can register validators for configuration values to ensure they meet your requirements.
Use `RegisterValidator` (or its alias `AddValidator`) to attach a validator to a key:

```go
// Register a validator for the "server_port" configuration key
config.RegisterValidator("server_port", func(value interface{}) error {
    port, ok := value.(int)
    if !ok {
        return errors.New("server_port must be an integer")
    }
    if port < 1024 || port > 65535 {
        return errors.New("server_port must be between 1024 and 65535")
    }
    return nil
})

// Validate the entire configuration
if err := config.Validate(); err != nil {
    log.Fatalf("Configuration validation failed: %v", err)
}

// Validate a specific configuration key
if err := config.ValidateKey("server_port"); err != nil {
    log.Fatalf("Validation of server_port failed: %v", err)
}
```

### Detecting validation errors

All validation failures match the `validcfg.ErrValidation` sentinel via
`errors.Is`, so you can branch on validation errors without depending on the
concrete error types:

```go
import "github.com/iqhive/cfggo/validcfg" // adjust to your actual import path

if err := config.Validate(); err != nil {
    if errors.Is(err, validcfg.ErrValidation) {
        // One or more values failed validation.
        log.Fatalf("invalid configuration: %v", err)
    }
    // Some other error (I/O, parsing, etc.)
    log.Fatalf("could not validate configuration: %v", err)
}
```

## Debugging: where did this value come from?

cfggo tracks the provenance of every value, which is usually the fastest way to
answer "why is this value what it is?"

```go
// Human-readable, source-annotated dump of the whole config:
fmt.Println(config.Explain())
// features:
//   name: prod   (from env)   // the name
//   port: 8080   (from flag)

// Or query a single key:
if src, ok := config.Source("server_port"); ok {
    fmt.Printf("server_port came from %s\n", src) // default | file | http | env | flag | set
}
```

Turn up logging to trace loading decisions:

```go
cfggo.SetLogLevel(cfggo.LogLevelDebug)
```

### Inspecting errors

Errors returned by cfggo wrap sentinel values, so you can branch on them with
`errors.Is` and recover the underlying cause with `errors.As`:

```go
if err := config.ReloadConfig(); err != nil {
    switch {
    case errors.Is(err, cfggo.ErrSource):     // source (file/http/env) failure
    case errors.Is(err, cfggo.ErrNoHandler):  // no source configured
    case errors.Is(err, cfggo.ErrUnknownKey): // unknown key
    case errors.Is(err, cfggo.ErrValidation): // validation failure
    }
    // Application-defined code (if any):
    code := cfggo.ErrorCode(err)
    _ = code
}
```

### Initialising without exiting the process

`Init` (and `InitSelf` / `InitMyParent`) returns an error instead of terminating
the process, so you can handle initialisation failures yourself:

```go
if err := config.Init(config, cfggo.WithFileConfig("config.json")); err != nil {
    return fmt.Errorf("load config: %w", err)
}
```

## Advanced Features

### Custom Configuration Sources

A configuration source is anything that implements the `sources.ConfigHandler`
interface, which exchanges JSON with cfggo:

```go
import "github.com/iqhive/cfggo/sources"

type ConfigHandler interface {
    // IsDefault reports whether startup may continue if LoadConfig fails
    // (true means a missing/unreadable source is tolerated).
    IsDefault() bool
    // LoadConfig returns the JSON representation of the configuration.
    LoadConfig() (json.RawMessage, error)
    // SaveConfig persists the JSON representation of the configuration.
    SaveConfig(json.RawMessage) error
}
```

cfggo ships with handlers for files (`WithFileConfig` / `WithDefaultFileConfig`),
HTTP endpoints (`WithHTTPConfig`), and environment variables (`WithEnvConfig`).
Plug in your own implementation with `WithConfigHandler`:

```go
cfggo.Init(config, cfggo.WithConfigHandler(myHandler))
```

### Nested Configuration

You can use nested structures for more complex configuration:

```go
type DatabaseConfig struct {
    Host     func() string `cfggo:"host" default:"localhost" help:"Database host"`
    Port     func() int    `cfggo:"port" default:"5432" help:"Database port"`
    Username func() string `cfggo:"username" default:"user" help:"Database username"`
    Password func() string `cfggo:"password" default:"pass" help:"Database password"`
}

type MyConfig struct {
    cfggo.Structure
    ServerPort func() int     `cfggo:"server_port" help:"Server port"`
    Database   DatabaseConfig `cfggo:"database" help:"Database configuration"`
}
```

Nested fields are flattened into dotted keys (`database.host`, `database.port`),
which is also how they map to environment variables (`DATABASE_HOST`) and flags
(`--database.host`). Pointer sub-structs (`Database *DatabaseConfig`) work too —
cfggo allocates a nil pointer sub-struct during `Init` so its accessor functions
are wired up and safe to call.

### Detecting Unrecognized Configuration

To catch typos or stale keys (for example, leftover entries in a config file),
ask cfggo to warn about keys that don't correspond to a struct field:

```go
config.CheckUnrecognizedItems(config)
```

Use `cfggo.IgnoreFlags("key1", "key2")` to exempt command-line-only keys from
that check.

### Enhanced Validation

The package provides a rich set of built-in validators for common validation scenarios:

```go
// Add validators to your configuration
config.AddValidator("server_port", cfggo.Range(1024, 65535))
config.AddValidator("email", cfggo.Email())
config.AddValidator("username", cfggo.All(
    cfggo.Required(),
    cfggo.MinLength(3),
    cfggo.MaxLength(20),
))
config.AddValidator("api_key", cfggo.Regex(`^[A-Za-z0-9]{32}$`))
config.AddValidator("log_level", cfggo.OneOf("debug", "info", "warn", "error"))
config.AddValidator("website", cfggo.URL())

// Validate the entire configuration
if err := config.Validate(); err != nil {
    // ValidationErrors provides detailed information about all validation failures
    log.Fatalf("Configuration validation failed: %v", err)
}
```

Available validators include:

- `Required()` — Ensures a value is not nil or empty.
- `MinLength(min)` — Checks if a string, slice, or map has at least `min` elements.
- `MaxLength(max)` — Checks if a string, slice, or map has at most `max` elements.
- `Range(min, max)` — Ensures a numeric value is within the specified range.
- `OneOf(options...)` — Checks if a value is one of the provided options.
- `Regex(pattern)` — Validates a string against a regular expression.
- `Email()` — Validates that a string is a valid email address.
- `URL()` — Validates that a string is a valid URL.
- `Custom(func)` — Creates a validator from a custom function.
- `All(validators...)` — Ensures all validators pass.
- `Any(validators...)` — Ensures at least one validator passes.

### Logging

The package includes a configurable logging system that can be adjusted based on your application's needs:

```go
// Set the log level
cfggo.SetLogLevel(cfggo.LogLevelDebug)

// Parse log level from string
level, _ := cfggo.ParseLogLevel("info")
cfggo.SetLogLevel(level)

// Redirect logs to a file
file, _ := os.OpenFile("config.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
cfggo.SetLogOutput(file)
```

Available log levels:

- `LogLevelDebug` — Detailed debugging information.
- `LogLevelInfo` — General information about normal operation.
- `LogLevelWarn` — Warnings that don't affect normal operation.
- `LogLevelError` — Errors that affect operation but don't cause termination.
- `LogLevelFatal` — Errors that cause termination.
- `LogLevelNone` — Disables all logging.

`SetLogLevel` and `SetLogOutput` configure the built-in global logger. To give a
single configuration instance its own logger, implement `cfglogger.Logger` and
supply it with `WithLogger` (or `config.SetLogger(...)`):

```go
cfggo.Init(config, cfggo.WithLogger(myLogger))
```

Similarly, `WithErrorWrapper` lets you customise how cfggo formats the errors it
returns (for example, to attach your own error codes or context).

### Configuration Options

`cfggo` provides several options for initializing configuration:

```go
// Name the configuration instance (used in logs and Explain() output)
cfggo.Init(config, cfggo.WithName("server"))

// Load configuration from a file (warns if the file is missing)
cfggo.Init(config, cfggo.WithFileConfig("config.json"))

// Load configuration from a file, silently tolerating a missing file
cfggo.Init(config, cfggo.WithDefaultFileConfig("config.json"))

// Load configuration from a file specified by a command-line flag
cfggo.Init(config, cfggo.WithFileConfigParamName("config"))

// Load configuration from HTTP endpoints
cfggo.Init(config, cfggo.WithHTTPConfig(httpLoader, httpSaver))

// Use environment variables as a config source, optionally filtered by prefix
cfggo.Init(config, cfggo.WithEnvConfig("MYAPP_"))

// Plug in a custom sources.ConfigHandler
cfggo.Init(config, cfggo.WithConfigHandler(myHandler))

// Skip loading from environment variables
cfggo.Init(config, cfggo.WithSkipEnvironment())

// Save the configuration when a context is cancelled (e.g. on shutdown).
// cfggo does not install signal handlers or call os.Exit; you own the context.
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
cfggo.Init(config, cfggo.WithAutoSave(ctx))
// Or save explicitly from your own shutdown path:
//   err := config.SaveIfChanged()

// Register cfggo's flags on a custom FlagSet (host owns the Parse() call)
cfggo.Init(config, cfggo.WithFlagSet(myFlagSet))

// Register cfggo's flags on the global flag.CommandLine, then call flag.Parse()
cfggo.Init(config, cfggo.WithStandardFlags())

// Ignore (instead of exiting on) command-line flags cfggo doesn't define
cfggo.Init(config, cfggo.WithIgnoreUnknownVars())

// Add a validator during initialization
cfggo.Init(config, cfggo.WithValidation("server_port", portValidator))

// Use a custom logger or error wrapper for this instance
cfggo.Init(config, cfggo.WithLogger(myLogger))
cfggo.Init(config, cfggo.WithErrorWrapper(myWrapper))
```

> **Note:** Options can be combined in a single `Init` call, e.g.
> `cfggo.Init(config, cfggo.WithName("api"), cfggo.WithFileConfig("config.json"), cfggo.WithAutoSave(ctx))`.

### Deprecations

A few symbols are retained for backward compatibility and will be removed in a
future version. Prefer the replacements:

| Deprecated | Use instead |
|---|---|
| `CleanupSignalHandler()` (now a no-op) | `WithAutoSave(ctx)` or call `Save`/`SaveIfChanged` from your own shutdown path |
| `WithErrorWrapperWithLogger` / `WrapErrorWithLogging` | `WithErrorWrapper` and log the returned error yourself |
| `convert.ConvertValue` / `cfggo.ConvertValue` | `config.Set(key, value)` |

## Thread Safety

All operations in `cfggo` are protected by per-instance mutexes, making it safe
to use in concurrent applications. You can access and update configuration values
from multiple goroutines — including from within `OnChange` callbacks — without
worrying about race conditions. Independent `Structure` instances never contend
on a shared lock.

## Examples

Runnable examples live in the [`examples/`](examples) directory:

- [`examples/basic`](examples/basic) — minimal setup with file, env, and flag loading.
- [`examples/advanced`](examples/advanced) — multiple instances with custom loggers and error wrappers.
- [`examples/validation`](examples/validation) — built-in and custom validators.

## Contributing

Contributions are welcome! If you find any issues or have suggestions for new features, please open an issue or submit a pull request on the [GitHub repository](https://github.com/iqhive/cfggo).

## License

`cfggo` is licensed under the [MIT License](LICENSE).

## Acknowledgments

`cfggo` is inspired by the [Viper](https://github.com/spf13/viper) configuration package. It aims to provide a more lightweight and flexible alternative with additional features like type safety and custom configuration handlers.
