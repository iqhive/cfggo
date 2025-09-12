# CFGGO Examples

This directory contains examples demonstrating how to use cfggo's new error handling and logging features.

## New Features

### Per-Instance Loggers and Error Wrappers

cfggo now supports instance-specific loggers and error wrappers while maintaining full backwards compatibility.

## Examples

### Basic Example (`basic/`)

Demonstrates:
- Basic usage (backwards compatible)
- Custom logger per instance
- Error wrapping with logging
- Multiple instances with different loggers

Run: `cd basic && go run main.go`

### Advanced Example (`advanced/`)

Demonstrates:
- Custom logger implementations
- Custom error wrappers
- Multiple services with different error handling
- Type conversion errors with instance-specific handling
- Backwards compatibility verification

Run: `cd advanced && go run main.go`

## Key Features Demonstrated

### 1. Instance-Specific Loggers

```go
// Each configuration instance can have its own logger
config := &MyConfig{}
config.Init(config, cfggo.WithLogger(customLogger))
```

### 2. Instance-Specific Error Wrappers

```go
// Custom error wrapper for this instance only
config.Init(config, cfggo.WithErrorWrapper(customErrorWrapper))
```

### 3. Error Wrapping with Automatic Logging

```go
// Wrap errors and automatically log them
err := config.WrapErrorWithLogging(nil, 500, "Something went wrong")
```

### 4. Multiple Configuration Instances

```go
// Different services can have different loggers and error handling
serviceA := &Config{}
serviceA.Init(serviceA, cfggo.WithLogger(serviceALogger))

serviceB := &Config{}
serviceB.Init(serviceB, cfggo.WithLogger(serviceBLogger))
```

### 5. Backwards Compatibility

```go
// Existing code continues to work unchanged
config := &MyConfig{}
config.Init(config)  // Uses global logger and error wrapper
```

## Migration Guide

### For Existing Users

No changes required! Your existing code will continue to work exactly as before. The global `cfggo.Logger` and `cfggo.ErrorWrapper` are still available and work the same way.

### For New Features

To use the new per-instance features:

1. **Custom Logger**: Use `cfggo.WithLogger(yourLogger)` as an option when calling `Init()`
2. **Custom Error Wrapper**: Use `cfggo.WithErrorWrapper(yourWrapper)` as an option
3. **Error Logging**: Call `config.WrapErrorWithLogging()` instead of the global `ErrorWrapper`

## Directory Structure

The conversion functions have been moved to their own package for better organization:

- `convert/` - Type conversion utilities (moved from `convert.go`)
- The original `convert.go` now provides backwards-compatible wrappers

This refactoring improves code organization while maintaining full backwards compatibility.
