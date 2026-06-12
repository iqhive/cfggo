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
// # Extension points
//
//   - [WithLogger] / [SetLogLevel] / [SetLogOutput]: control logging.
//   - [WithErrorWrapper]: customise error formatting.
//   - [WithValidation] / [AddValidator]: attach validators to individual keys.
//   - [WithConfigHandler]: plug in a custom [sources.ConfigHandler].
package cfggo
