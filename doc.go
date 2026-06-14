// Package cfggo provides type-safe, hot-reloadable configuration management
// for Go applications.
//
// # Quick start
//
// Define a configuration struct by embedding [Structure] and declaring each
// configuration field as a zero-argument function that returns the desired type:
//
//	type AppConfig struct {
//	    cfggo.Structure
//	    Port    func() int    `cfggo:"port"     default:"8080"      help:"HTTP listen port"`
//	    DSN     func() string `cfggo:"db_dsn"   default:""          help:"Database DSN"`
//	    Debug   func() bool   `cfggo:"debug"    default:"false"     help:"Enable debug logging"`
//	}
//
// Initialise the config once at program startup:
//
//	cfg := &AppConfig{}
//	cfggo.Init(cfg, cfggo.WithDefaultFileConfig("config.json"))
//
// Read values by calling the accessor functions:
//
//	fmt.Println("listening on port", cfg.Port())
//
// # Why fields are functions
//
// Each configuration field is declared as a func() T rather than a plain T so
// that reads always observe the current value. cfggo replaces each field with a
// closure that reads the live config map under a lock, which is what makes hot
// reloading transparent: after [Structure.ReloadConfig] every accessor returns
// the updated value with no pointer-swapping or re-reading by the caller, and
// concurrent reads are race-free. The trade-off is the small syntactic overhead
// of calling cfg.Port() instead of cfg.Port. Set a default in code with
// [DefaultValue] (e.g. Port: cfggo.DefaultValue(8080)) or via the `default`
// tag.
//
// # Pitfalls
//
// Because each field is a func() T, two mistakes are worth calling out:
//
//   - Calling an accessor before Init panics. cfggo installs the reading
//     closures during Init, so a field left at its nil zero value (no
//     [DefaultValue] assigned) is a nil func and cfg.Port() panics. Always call
//     Init/InitSelf at startup before reading any value. Assigning
//     Port: cfggo.DefaultValue(8080) makes the field safe to read even before
//     Init, since it already holds a real function.
//   - A field declared as a plain T instead of func() T is silently ignored.
//     If you write Port int with a cfggo tag, it never backs a config value.
//     Init logs a warning for any cfggo/cfg/config-tagged field that is not a
//     func() T so this shows up instead of failing mysteriously at runtime.
//
// # Configuration sources (in precedence order, lowest to highest)
//
//  1. Struct field defaults: set via [DefaultValue] at declaration time.
//  2. Tag defaults: `default:"..."` struct tag.
//  3. Configuration file: JSON file via [WithFileConfig] or [WithDefaultFileConfig].
//  4. Environment variables: keys are upper-cased and dots become underscores
//     (e.g. "db.host" becomes "DB_HOST").
//  5. Command-line flags: registered automatically; name equals the config key.
//
// # Hot reloading
//
//	if err := cfg.ReloadConfig(); err != nil {
//	    log.Println("reload failed:", err)
//	}
//
// After a reload every subsequent call to an accessor function returns the
// updated value. All accessors are safe for concurrent use.
//
// # Strictness
//
// By default Init is strict: a malformed configuration file, a value that
// cannot be coerced to its field type, or a value that fails a registered
// validator causes Init to return an error instead of silently starting with
// partial or default values. Use [WithLenientLoad] for best-effort loading
// (failures become logged warnings), and [WithStrictKeys] to turn unrecognized
// configuration keys (typos) into errors rather than warnings.
//
// # Reading values
//
// Accessor functions return their value directly. For dynamic keys (or when you
// prefer not to go through the struct) use the type-safe generic helpers:
//
//	port, ok := cfggo.Value[int](&cfg.Structure, "port")
//	dsn := cfggo.MustValue[string](&cfg.Structure, "db_dsn")
//
// # Secrets
//
// Tag a field `secret:"true"` to mark it sensitive. Its value is masked in all
// human-readable / diagnostic output ([Structure.Explain], [Structure.String],
// [Structure.Diagnose], [Structure.ConfigReference], [Structure.Report]) so a
// config dump pasted into a log or bug report does not leak credentials. Saving
// (Save / GetJSONBytes) still writes the real value so round-tripping works.
//
// # Debugging
//
//   - [Structure.Explain]: human-readable, source-annotated dump of all values,
//     including the full override chain (eg "[default->file->env]") for any
//     value set by more than one source.
//   - [Structure.SourceChain]: the ordered list of sources that contributed to
//     a key's current value, the programmatic form of Explain's chain.
//   - [Structure.DiagnoseData]: the canonical structured snapshot (value,
//     source, chain, type, default, validation) that every built-in report is
//     rendered from. Use it to build your own formatting or a "--config-check".
//   - [Structure.Diagnose]: alias for DiagnoseData for read-only call sites.
//   - [Structure.ConfigReference]: reference table of every field (key, type,
//     env var, default, help) generated from the struct definition.
//   - [Structure.Report]: one-stop dump (reference + resolved state + validation)
//     with secrets masked, ideal for attaching to a bug report.
//
// # Extension points
//
//   - [WithLogger] / [SetLogLevel] / [SetLogOutput]: control logging. The global
//     logger and error wrapper are accessed via the race-safe [GlobalLogger] /
//     [SetGlobalLogger] and [GlobalErrorWrapper] / [SetGlobalErrorWrapper].
//   - [WithErrorWrapper]: customise error formatting.
//   - [WithValidation] / [AddValidator]: attach validators to individual keys.
//   - [WithConfigHandler]: plug in a custom [sources.ConfigHandler].
package cfggo
