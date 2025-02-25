# cfggo

`cfggo` is a Go package designed to simplify configuration management in Go applications. It provides a type-safe, flexible, and powerful way to handle configuration through various sources such as files, environment variables, and command-line flags.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
  - [Configuration Structure](#configuration-structure)
  - [Configuration Loading Order](#configuration-loading-order)
  - [Type Safety](#type-safety)
  - [Default Values](#default-values)
- [Configuration Sources](#configuration-sources)
  - [JSON Files](#json-files)
  - [Environment Variables](#environment-variables)
  - [Command-Line Flags](#command-line-flags)
  - [HTTP Endpoints](#http-endpoints)
- [Advanced Features](#advanced-features)
  - [Configuration Validation](#configuration-validation)
  - [Configuration Reloading](#configuration-reloading)
  - [Thread Safety](#thread-safety)
  - [Automatic Saving](#automatic-saving)
  - [Custom FlagSets](#custom-flagsets)
- [Best Practices](#best-practices)
- [API Reference](#api-reference)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Type-safe configuration**: Access configuration values with proper Go types
- **Multiple configuration sources**: Load from JSON files, environment variables, command-line flags, and HTTP endpoints
- **Configuration validation**: Validate configuration values against custom rules
- **Dynamic reloading**: Update configuration without restarting your application
- **Thread-safe access**: Safely access and update configuration from multiple goroutines
- **Default values**: Specify default values for configuration keys
- **Customizable**: Extend with your own configuration handlers
- **Minimal dependencies**: Only relies on the Go standard library

## Installation

To install the package, run:

```sh
go get github.com/iqhive/cfggo
```

## Quick Start

Here's a simple example to get you started:

```go
package main

import (
    "fmt"
    "log"
    "github.com/iqhive/cfggo"
)

// Define your configuration structure
type AppConfig struct {
    cfggo.Structure                // Embed the cfggo.Structure
    ServerPort func() int          `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string       `cfg:"log_level" help:"Logging level (debug, info, warn, error)"`
    Features   func() []string     `cfg:"features" help:"Enabled features"`
    Database   func() DatabaseConfig `cfg:"database" help:"Database configuration"`
}

type DatabaseConfig struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`
    Username string `json:"username"`
    Password string `json:"password"`
}

func main() {
    // Create a new configuration with default values
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
        Features:   cfggo.DefaultValue([]string{"auth", "api", "metrics"}),
    }
    
    // Initialize the configuration
    config.Init(config, 
        cfggo.WithDefaultFileConfig("config.json"),
        cfggo.WithFileConfigParamName("config"),
    )
    
    // Access configuration values
    fmt.Printf("Server will start on port %d\n", config.ServerPort())
    fmt.Printf("Log level: %s\n", config.LogLevel())
    fmt.Printf("Enabled features: %v\n", config.Features())
    
    // You can also update configuration values
    if err := config.Set("server_port", 9000); err != nil {
        log.Fatalf("Failed to update server port: %v", err)
    }
    
    fmt.Printf("Server port updated to %d\n", config.ServerPort())
}
```

## Core Concepts

### Configuration Structure

In cfggo, you define your configuration as a Go struct with function fields. Each function returns a specific type and is tagged with a configuration key:

```go
type MyConfig struct {
    cfggo.Structure                // Embed the cfggo.Structure
    ServerPort func() int          `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string       `cfg:"log_level" help:"Logging level"`
}
```

The `cfg` tag specifies the configuration key name, and the `help` tag provides documentation for command-line help.

### Configuration Loading Order

cfggo loads configuration in the following order, with later sources overriding earlier ones:

1. Default values specified in code
2. Configuration files
3. Environment variables
4. Command-line flags

This means command-line flags have the highest precedence, followed by environment variables, then configuration files, and finally default values.

### Type Safety

One of the key features of cfggo is type safety. Configuration values are accessed through typed functions, ensuring you always get the correct type:

```go
// This returns an int, not an interface{} or string that needs conversion
port := config.ServerPort()

// This returns a string
level := config.LogLevel()

// This returns a slice of strings
features := config.Features()
```

### Default Values

You can specify default values for configuration keys when creating your configuration struct:

```go
config := &MyConfig{
    ServerPort: cfggo.DefaultValue(8080),
    LogLevel:   cfggo.DefaultValue("info"),
}
```

## Configuration Sources

### JSON Files

cfggo can load configuration from JSON files:

```go
// Load from a specific file (error if file doesn't exist)
config.Init(config, cfggo.WithFileConfig("config.json"))

// Load from a file if it exists, but don't error if it doesn't
config.Init(config, cfggo.WithDefaultFileConfig("config.json"))

// Load from a file specified by a command-line flag
config.Init(config, cfggo.WithFileConfigParamName("config"))
```

The JSON structure should match your configuration keys:

```json
{
    "server_port": 8080,
    "log_level": "debug",
    "features": ["auth", "api", "metrics"],
    "database": {
        "host": "localhost",
        "port": 5432,
        "username": "user",
        "password": "password"
    }
}
```

### Environment Variables

cfggo automatically loads configuration from environment variables. The environment variable names are derived from the configuration keys by converting to uppercase and replacing dots with underscores:

```
SERVER_PORT=9000
LOG_LEVEL=debug
FEATURES=auth,api,metrics
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_USERNAME=user
DATABASE_PASSWORD=password
```

You can disable environment variable loading with:

```go
config.Init(config, cfggo.WithSkipEnvironment())
```

### Command-Line Flags

cfggo automatically creates command-line flags for each configuration key:

```
./myapp --server_port=9000 --log_level=debug --features=auth,api,metrics
```

### HTTP Endpoints

cfggo can load and save configuration from HTTP endpoints:

```go
// Create HTTP requests for loading and saving
loadReq, _ := http.NewRequest("GET", "https://config-server.example.com/config", nil)
saveReq, _ := http.NewRequest("POST", "https://config-server.example.com/config", nil)

// Initialize with HTTP configuration
config.Init(config, cfggo.WithHTTPConfig(loadReq, saveReq))
```

## Advanced Features

### Configuration Validation

cfggo supports validation of configuration values. You can register validators for specific configuration keys:

```go
// Define a validator function
portValidator := func(value interface{}) error {
    port, ok := value.(int)
    if !ok {
        return errors.New("port must be an integer")
    }
    if port < 1024 || port > 65535 {
        return errors.New("port must be between 1024 and 65535")
    }
    return nil
}

// Register the validator during initialization
config.Init(config, cfggo.WithValidation("server_port", portValidator))

// Or register it after initialization
config.RegisterValidator("server_port", portValidator)

// Validate the entire configuration
if err := config.Validate(); err != nil {
    log.Fatalf("Configuration validation failed: %v", err)
}

// Validate a specific key
if err := config.ValidateKey("server_port"); err != nil {
    log.Fatalf("Server port validation failed: %v", err)
}
```

### Configuration Reloading

cfggo supports reloading configuration from all sources without restarting your application:

```go
// Reload configuration from all sources
if err := config.ReloadConfig(); err != nil {
    log.Fatalf("Failed to reload configuration: %v", err)
}
```

This is useful for applications that need to update their configuration dynamically, such as when a configuration file changes.

### Thread Safety

cfggo is thread-safe, meaning you can access and update configuration from multiple goroutines concurrently:

```go
// Access configuration from multiple goroutines
for i := 0; i < 10; i++ {
    go func() {
        port := config.ServerPort()
        fmt.Printf("Server port: %d\n", port)
    }()
}

// Update configuration from multiple goroutines
for i := 0; i < 10; i++ {
    go func(i int) {
        err := config.Set("server_port", 8080 + i)
        if err != nil {
            fmt.Printf("Failed to update port: %v\n", err)
        }
    }(i)
}
```

### Automatic Saving

cfggo can automatically save configuration changes to the configured destination (file or HTTP endpoint):

```go
// Enable automatic saving
config.Init(config, cfggo.WithAutoSave())
```

When automatic saving is enabled, any changes made to the configuration using the `Set` method will be saved to the configured destination.

### Custom FlagSets

cfggo supports using custom FlagSets instead of the global flag.CommandLine:

```go
// Create a custom FlagSet
customFlags := flag.NewFlagSet("myapp", flag.ExitOnError)

// Use the custom FlagSet
config.Init(config, cfggo.WithFlagSet(customFlags))

// Parse the custom FlagSet
customFlags.Parse(os.Args[1:])
```

This is useful when you need to use multiple FlagSets in your application or when writing tests.

## Best Practices

1. **Define a clear configuration structure**: Organize your configuration logically with nested structures when appropriate.

2. **Use descriptive help tags**: Provide clear descriptions in the `help` tags to make your command-line interface user-friendly.

3. **Set sensible defaults**: Always provide default values for configuration keys that make sense for most users.

4. **Validate critical configuration**: Use validators for configuration keys that have specific requirements or constraints.

5. **Handle errors gracefully**: Always check for errors when initializing configuration or updating values.

6. **Use environment variables for sensitive information**: Avoid storing sensitive information like passwords in configuration files.

7. **Document your configuration**: Provide documentation for users on available configuration options and their meanings.

8. **Test your configuration**: Write tests to ensure your configuration behaves as expected in different scenarios.

## API Reference

### Structure Methods

- `Init(parent interface{}, options ...Option) error`: Initialize the configuration structure
- `Set(key string, value interface{}) error`: Set a configuration value
- `Get(key string) (interface{}, bool)`: Get a configuration value and whether it exists
- `RegisterValidator(key string, validator Validator) error`: Register a validator for a configuration key
- `Validate() error`: Validate the entire configuration
- `ValidateKey(key string) error`: Validate a specific configuration key
- `ReloadConfig() error`: Reload configuration from all sources

### Options

- `WithName(name string) Option`: Set the name of the configuration
- `WithFileConfig(filename string) Option`: Set the configuration file (error if file doesn't exist)
- `WithDefaultFileConfig(filename string) Option`: Set the configuration file (no error if file doesn't exist)
- `WithFileConfigParamName(argName string) Option`: Set the configuration file from a command-line flag
- `WithHTTPConfig(httpLoader *http.Request, httpSaver *http.Request) Option`: Set HTTP endpoints for loading and saving
- `WithSkipEnvironment() Option`: Skip loading from environment variables
- `WithAutoSave() Option`: Enable automatic saving of configuration changes
- `WithFlagSet(fs *flag.FlagSet) Option`: Use a custom FlagSet
- `WithValidation(key string, validator Validator) Option`: Register a validator for a configuration key

## Getting Started Guide

### Step 1: Define Your Configuration Structure

Start by defining a struct that embeds `cfggo.Structure` and includes function fields for your configuration values:

```go
package main

import (
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure                // Embed the cfggo.Structure
    ServerPort func() int          `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string       `cfg:"log_level" help:"Logging level"`
    Debug      func() bool         `cfg:"debug" help:"Enable debug mode"`
}
```

### Step 2: Create and Initialize Your Configuration

Create an instance of your configuration struct, set default values, and initialize it:

```go
func main() {
    // Create a new configuration with default values
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
        Debug:      cfggo.DefaultValue(false),
    }
    
    // Initialize the configuration
    config.Init(config, 
        cfggo.WithDefaultFileConfig("config.json"),
        cfggo.WithFileConfigParamName("config"),
    )
    
    // Now you can use your configuration
    startServer(config)
}
```

### Step 3: Access Configuration Values

Access your configuration values using the function fields:

```go
func startServer(config *AppConfig) {
    port := config.ServerPort()
    logLevel := config.LogLevel()
    debug := config.Debug()
    
    fmt.Printf("Starting server on port %d with log level %s\n", port, logLevel)
    if debug {
        fmt.Println("Debug mode enabled")
    }
    
    // Your server code here...
}
```

### Step 4: Update Configuration Values

You can update configuration values at runtime:

```go
func updateConfig(config *AppConfig) {
    // Update a configuration value
    if err := config.Set("server_port", 9000); err != nil {
        log.Fatalf("Failed to update server port: %v", err)
    }
    
    // Get the updated value
    newPort := config.ServerPort()
    fmt.Printf("Server port updated to %d\n", newPort)
}
```

### Step 5: Save Configuration Changes

If you want to save configuration changes to a file:

```go
func saveConfig(config *AppConfig) {
    // Save the current configuration to a file
    if err := config.SaveConfig(); err != nil {
        log.Fatalf("Failed to save configuration: %v", err)
    }
    
    fmt.Println("Configuration saved successfully")
}
```

## Examples

### Basic Configuration

```go
package main

import (
    "fmt"
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure
    ServerPort func() int    `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string `cfg:"log_level" help:"Logging level"`
}

func main() {
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
    }
    
    config.Init(config, cfggo.WithDefaultFileConfig("config.json"))
    
    fmt.Printf("Server port: %d\n", config.ServerPort())
    fmt.Printf("Log level: %s\n", config.LogLevel())
}
```

### Nested Configuration

```go
package main

import (
    "fmt"
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure
    Server   func() ServerConfig   `cfg:"server" help:"Server configuration"`
    Database func() DatabaseConfig `cfg:"database" help:"Database configuration"`
}

type ServerConfig struct {
    Host string `json:"host"`
    Port int    `json:"port"`
}

type DatabaseConfig struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`
    Username string `json:"username"`
    Password string `json:"password"`
}

func main() {
    config := &AppConfig{
        Server: cfggo.DefaultValue(ServerConfig{
            Host: "localhost",
            Port: 8080,
        }),
        Database: cfggo.DefaultValue(DatabaseConfig{
            Host:     "localhost",
            Port:     5432,
            Username: "user",
            Password: "password",
        }),
    }
    
    config.Init(config, cfggo.WithDefaultFileConfig("config.json"))
    
    server := config.Server()
    fmt.Printf("Server: %s:%d\n", server.Host, server.Port)
    
    db := config.Database()
    fmt.Printf("Database: %s:%d (user: %s)\n", db.Host, db.Port, db.Username)
}
```

### Configuration Validation

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure
    ServerPort func() int    `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string `cfg:"log_level" help:"Logging level"`
}

func main() {
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
    }
    
    // Define validators
    portValidator := func(value interface{}) error {
        port, ok := value.(int)
        if !ok {
            return errors.New("port must be an integer")
        }
        if port < 1024 || port > 65535 {
            return errors.New("port must be between 1024 and 65535")
        }
        return nil
    }
    
    logLevelValidator := func(value interface{}) error {
        level, ok := value.(string)
        if !ok {
            return errors.New("log level must be a string")
        }
        validLevels := map[string]bool{
            "debug": true,
            "info":  true,
            "warn":  true,
            "error": true,
        }
        if !validLevels[level] {
            return errors.New("log level must be one of: debug, info, warn, error")
        }
        return nil
    }
    
    // Initialize with validators
    config.Init(config,
        cfggo.WithDefaultFileConfig("config.json"),
        cfggo.WithValidation("server_port", portValidator),
        cfggo.WithValidation("log_level", logLevelValidator),
    )
    
    // Validate the configuration
    if err := config.Validate(); err != nil {
        log.Fatalf("Configuration validation failed: %v", err)
    }
    
    fmt.Printf("Server port: %d\n", config.ServerPort())
    fmt.Printf("Log level: %s\n", config.LogLevel())
}
```

### Configuration Reloading

```go
package main

import (
    "fmt"
    "log"
    "os"
    "time"
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure
    ServerPort func() int    `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string `cfg:"log_level" help:"Logging level"`
}

func main() {
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
    }
    
    // Initialize with a configuration file
    config.Init(config, cfggo.WithFileConfig("config.json"))
    
    // Print initial configuration
    fmt.Printf("Initial config - Port: %d, Log Level: %s\n", 
        config.ServerPort(), config.LogLevel())
    
    // Simulate a configuration file change
    go func() {
        time.Sleep(2 * time.Second)
        
        // Create a new configuration file with updated values
        newConfig := []byte(`{
            "server_port": 9000,
            "log_level": "debug"
        }`)
        
        if err := os.WriteFile("config.json", newConfig, 0644); err != nil {
            log.Fatalf("Failed to write new config file: %v", err)
        }
        
        fmt.Println("Configuration file updated")
        
        // Reload the configuration
        if err := config.ReloadConfig(); err != nil {
            log.Fatalf("Failed to reload configuration: %v", err)
        }
        
        // Print updated configuration
        fmt.Printf("Updated config - Port: %d, Log Level: %s\n", 
            config.ServerPort(), config.LogLevel())
    }()
    
    // Wait for the configuration to be reloaded
    time.Sleep(5 * time.Second)
}
```

### Environment Variables and Command-Line Flags

```go
package main

import (
    "fmt"
    "os"
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure
    ServerPort func() int    `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string `cfg:"log_level" help:"Logging level"`
}

func main() {
    // Set environment variables
    os.Setenv("SERVER_PORT", "9000")
    
    // Simulate command-line arguments
    os.Args = []string{"myapp", "--log_level=debug"}
    
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
    }
    
    // Initialize the configuration
    config.Init(config)
    
    // Print the configuration values
    fmt.Printf("Server port: %d (from environment variable)\n", config.ServerPort())
    fmt.Printf("Log level: %s (from command-line flag)\n", config.LogLevel())
}
```

### Custom FlagSet Example

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "github.com/iqhive/cfggo"
)

type AppConfig struct {
    cfggo.Structure
    ServerPort func() int    `cfg:"server_port" help:"Port for the server to listen on"`
    LogLevel   func() string `cfg:"log_level" help:"Logging level"`
}

func main() {
    // Create a custom FlagSet
    customFlags := flag.NewFlagSet("myapp", flag.ExitOnError)
    
    config := &AppConfig{
        ServerPort: cfggo.DefaultValue(8080),
        LogLevel:   cfggo.DefaultValue("info"),
    }
    
    // Initialize with the custom FlagSet
    config.Init(config, cfggo.WithFlagSet(customFlags))
    
    // Parse the custom FlagSet with some arguments
    customFlags.Parse([]string{"--server_port=9000", "--log_level=debug"})
    
    // Print the configuration values
    fmt.Printf("Server port: %d\n", config.ServerPort())
    fmt.Printf("Log level: %s\n", config.LogLevel())
}
```

## Contributing

Contributions are welcome! If you find any issues or have suggestions for new features, please open an issue or submit a pull request on the [GitHub repository](https://github.com/iqhive/cfggo).

## License

`cfggo` is licensed under the [MIT License](LICENSE).
