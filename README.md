# cfggo

`cfggo` is a Go package designed to simplify configuration management in Go applications. It provides a flexible and powerful way to handle configuration through various sources such as files, environment variables, and HTTP endpoints. The package supports default values, dynamic configuration updates, and command-line flags.

## Key Features

- **Type-safe configuration**: All configuration values are accessed through strongly-typed functions
- **Hot reloadable**: Configuration can be reloaded at runtime without restarting your application
- **Function-based approach**: Configuration values are accessed through functions, not direct field access
- **Multiple configuration sources**: Load from JSON files, environment variables, and HTTP endpoints
- **Command-line flag integration**: Seamless integration with Go's flag package
- **Thread-safe**: All operations are protected by mutexes for concurrent access
- **Validation support**: Register validators for configuration values
- **Default values**: Specify default values for configuration items
- **Automatic type conversion**: Values are automatically converted to the correct type

## Installation

To install the package, run:

```sh
go get github.com/iqhive/cfggo
```

## How It Works

`cfggo` uses a unique approach to configuration management:

1. **Function-based access**: Instead of accessing configuration values directly, you define functions that return the values
2. **Embedded Structure**: Your configuration struct embeds the `cfggo.Structure` type
3. **Type safety**: The return type of each function defines the type of the configuration value
4. **Hot reloading**: Configuration can be reloaded at runtime, and all functions will return the updated values

## Basic Example

Here's a simple example of how to use `cfggo`:

```go
package main

import (
    "fmt"
    "github.com/iqhive/cfggo"
)

// Define your configuration struct
type MyConfig struct {
    cfggo.Structure // Embed the Structure type
    
    // Define configuration items as functions that return the desired type
    ServerPort    func() int           `cfg:"server_port" default:"8080" help:"Port for the server to listen on"`
    DatabaseURL   func() string        `cfg:"db_url" default:"postgres://localhost:5432/mydb" help:"Database connection URL"`
    FeatureFlags  func() map[string]bool `cfg:"features" default:"{\"new_ui\": false, \"analytics\": true}" help:"Feature flags"`
    LogLevel      func() string        `cfg:"log_level" default:"info" help:"Logging level"`
}

func main() {
    // Create a new configuration instance
    config := &MyConfig{
        // Set default values using the DefaultValue helper
        ServerPort:   cfggo.DefaultValue(8080),
        DatabaseURL:  cfggo.DefaultValue("postgres://localhost:5432/mydb"),
        FeatureFlags: cfggo.DefaultValue(map[string]bool{"new_ui": false, "analytics": true}),
        LogLevel:     cfggo.DefaultValue("info"),
    }
    
    // Initialize the configuration
    // This will load values from a JSON file, environment variables, and command-line flags
    cfggo.Init(config, cfggo.WithDefaultFileConfig("config.json"))
    
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

## Configuration Sources and Precedence

`cfggo` loads configuration from multiple sources in the following order (later sources override earlier ones):

1. **Default values**: Set in code using `cfggo.DefaultValue()` or via struct tags
2. **Configuration files**: JSON files loaded via options
3. **Environment variables**: Automatically mapped from config keys (e.g., `server_port` → `SERVER_PORT`)
4. **Command-line flags**: Automatically registered based on your struct fields

## Environment Variables

Environment variables are automatically mapped from your configuration keys. The mapping follows these rules:

- Keys are converted to uppercase
- Dots (`.`) are replaced with underscores (`_`)

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

## Validation

You can register validators for configuration values to ensure they meet your requirements:

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

## Advanced Features

### Custom Configuration Sources

You can implement custom configuration sources by implementing the `configHandler` interface:

```go
type configHandler interface {
    LoadConfig() ([]byte, error)
    SaveConfig([]byte) error
}
```

### Nested Configuration

You can use nested structures for more complex configuration:

```go
type DatabaseConfig struct {
    Host     func() string `cfg:"host" default:"localhost" help:"Database host"`
    Port     func() int    `cfg:"port" default:"5432" help:"Database port"`
    Username func() string `cfg:"username" default:"user" help:"Database username"`
    Password func() string `cfg:"password" default:"pass" help:"Database password"`
}

type MyConfig struct {
    cfggo.Structure
    ServerPort func() int           `cfg:"server_port" help:"Server port"`
    Database   DatabaseConfig       `cfg:"database" help:"Database configuration"`
}
```

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
- `Required()` - Ensures a value is not nil or empty
- `MinLength(min)` - Checks if a string, slice, or map has at least `min` elements
- `MaxLength(max)` - Checks if a string, slice, or map has at most `max` elements
- `Range(min, max)` - Ensures a numeric value is within the specified range
- `OneOf(options...)` - Checks if a value is one of the provided options
- `Regex(pattern)` - Validates a string against a regular expression
- `Email()` - Validates that a string is a valid email address
- `URL()` - Validates that a string is a valid URL
- `Custom(func)` - Creates a validator from a custom function
- `All(validators...)` - Ensures all validators pass
- `Any(validators...)` - Ensures at least one validator passes

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
- `LogLevelDebug` - Detailed debugging information
- `LogLevelInfo` - General information about normal operation
- `LogLevelWarn` - Warnings that don't affect normal operation
- `LogLevelError` - Errors that affect operation but don't cause termination
- `LogLevelFatal` - Errors that cause termination
- `LogLevelNone` - Disables all logging

### Configuration Options

`cfggo` provides several options for initializing configuration:

```go
// Load configuration from a file
cfggo.Init(config, cfggo.WithFileConfig("config.json"))

// Load configuration from a file specified by a command-line flag
cfggo.Init(config, cfggo.WithFileConfigParamName("config"))

// Load configuration from HTTP endpoints
cfggo.Init(config, cfggo.WithHTTPConfig(httpLoader, httpSaver))

// Skip loading from environment variables
cfggo.Init(config, cfggo.WithSkipEnvironment())

// Enable automatic saving of configuration on program exit
cfggo.Init(config, cfggo.WithAutoSave())

// Use a custom FlagSet
cfggo.Init(config, cfggo.WithFlagSet(myFlagSet))

// Add a validator during initialization
cfggo.Init(config, cfggo.WithValidation("server_port", portValidator))
```

## Thread Safety

All operations in `cfggo` are protected by mutexes, making it safe to use in concurrent applications. You can access and update configuration values from multiple goroutines without worrying about race conditions.

## Contributing

Contributions are welcome! If you find any issues or have suggestions for new features, please open an issue or submit a pull request on the [GitHub repository](https://github.com/iqhive/cfggo).

## License

`cfggo` is licensed under the [MIT License](LICENSE).

## Acknowledgments

`cfggo` is inspired by the [viper](https://github.com/spf13/viper) configuration package. It aims to provide a more lightweight and flexible alternative with additional features like type safety and custom configuration handlers.
