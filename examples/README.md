# cfggo Examples

This directory contains runnable examples that demonstrate key cfggo features.
Each example can be run from its own subdirectory with `go run main.go`.

---

## Examples

### `demo/` - Hot reload GIF demo

Demonstrates:
- Loading config from a JSON file.
- `OnChange` callbacks when config values change.
- Typed accessors returning updated values after reload.

```sh
cd demo && cp config.initial.json config.json && go run main.go
```

---

### `basic/` - Getting started

Demonstrates:
- Embedding `cfggo.Structure` and defining typed accessor functions.
- Default values via `cfggo.DefaultValue(...)` and `default:"..."` struct tags.
- Per-instance loggers (`cfggo.WithLogger`).
- Multiple configuration instances with different names.

```sh
cd basic && go run main.go
```

Sample `config.json` is included to show how file-based values are loaded.

---

### `advanced/` - Custom loggers and error wrappers

Demonstrates:
- Implementing a fully custom `cfglogger.Logger`.
- Custom `errwrapper.ErrorWrapper` for rich error formatting.
- Running multiple independent service configs in one process.
- `WrapError` for error wrapping, then logging the returned error explicitly.

```sh
cd advanced && go run main.go
```

---

### `validation/` - Built-in and custom validators

Demonstrates the full validator API:

| Constructor                       | What it checks                              |
|-----------------------------------|---------------------------------------------|
| `cfggo.Required()`                | Value is non-nil and non-empty              |
| `cfggo.Range(min, max)`           | Numeric value within `[min, max]`           |
| `cfggo.MinLength(n)`              | String/slice has at least *n* elements      |
| `cfggo.MaxLength(n)`              | String/slice has at most *n* elements       |
| `cfggo.OneOf(opts...)`            | Value equals one of the listed options      |
| `cfggo.Regex(pattern)`            | String matches the regular expression       |
| `cfggo.Email()`                   | Valid email address                         |
| `cfggo.URL()`                     | Valid URL with scheme and host              |
| `cfggo.All(validators...)`        | All validators must pass                    |
| `cfggo.Any(validators...)`        | At least one validator must pass            |
| `cfggo.Custom(func(any) error)`   | User-supplied validation function           |

```sh
cd validation && go run main.go
```

Sample `config.json` is included with valid values so the example runs from a
clean checkout.

---

### `debugging/` - Startup checks and failure modes

Demonstrates:
- The recommended startup/debug pattern using `WithStrictKeys` and `Report`.
- Bad JSON, wrong type, unknown key, and failed validator errors.
- Environment and flag override provenance.
- Secret redaction in diagnostic output.

```sh
cd debugging && go run main.go
```

---

## Key patterns

### `cfggo.Init` vs method call

Both are equivalent:

```go
// Convenience function (added in Phase 2):
cfggo.Init(cfg, cfggo.WithDefaultFileConfig("config.json"))

// Equivalent method call:
cfg.Init(cfg, cfggo.WithDefaultFileConfig("config.json"))
```

### Per-instance vs global logger

```go
// Use the global logger (default):
cfg.Init(cfg)

// Use a custom logger for this instance only:
cfg.Init(cfg, cfggo.WithLogger(myLogger))

// Change the global log level:
cfggo.SetLogLevel(cfggo.LogLevelDebug)
```

### Configuration source precedence (lowest to highest)

1. `cfggo.DefaultValue(...)` set at struct literal time
2. `default:"..."` struct tag
3. JSON configuration file
4. Environment variables (`KEY` -> `KEY`, `nested.key` -> `NESTED_KEY`)
5. Command-line flags (`--key=value`)
