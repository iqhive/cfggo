package cfggo

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// NewFlag creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	if c.parent == nil {
		// Logger.Infof("NewFlag InitSelf %s", c.name)
		c.InitSelf()
	}

	configMutex.Lock()
	defer configMutex.Unlock()
	// Logger.Infof("NewFlag start %s", c.name)

	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	if c.FlagSet.Lookup(configVarName) != nil {
		Logger.Errorf("Flag (%s) is already set, skipping...\n", configVarName)
		return
	}

	// Special handling for boolean flags
	if boolVal, isBool := defaultValue.(bool); isBool {
		// Store the original value before we set up the flag
		c.configData[configVarName] = boolVal

		// Define a callback function that will be called after flag parsing
		boolCallback := func(parsedValue bool) {
			// The callback will be invoked by parseFlags after flags are parsed
			if parsedValue != boolVal {
				// Only update if the value changed
				configMutex.Lock()
				c.configData[configVarName] = parsedValue
				c.changed = true
				configMutex.Unlock()
			}
		}

		// Register the callback in a map to be called later
		if c.boolCallbacks == nil {
			c.boolCallbacks = make(map[string]func(bool))
		}
		c.boolCallbacks[configVarName] = boolCallback

		// Use BoolVar for the flag, which properly handles both --flag and --flag=true formats
		c.FlagSet.BoolVar(new(bool), configVarName, boolVal, configDescription)

		// Logger.Infof("NewFlag start 5z %s", c.name)
		return
	}

	// Logger.Infof("NewFlag start 6 %s", c.name)

	// Logger.Infof("NewFlag start 7 %s", c.name)
	// Only register with the dedicated FlagSet, not the global one
	if c.configData[configVarName] == nil {
		// Logger.Infof("NewFlag start 7a %s", c.name)
		Logger.Warnf("configData[%s] is not set, using default value for type", configVarName)
		// Create a safe copy of the data we need rather than keeping a reference to c
		typeName := configVarName
		targetType := reflect.TypeOf(defaultValue)

		// Store the value now while we have the lock
		c.configData[configVarName] = defaultValue

		// Create a var without holding a reference to c
		dvar := &safeVar{
			name:   typeName,
			want:   targetType,
			setter: c.createSetter(configVarName),
		}
		c.FlagSet.Var(dvar, configVarName, configDescription)
		// Logger.Infof("NewFlag start 7b %s", c.name)
	} else {
		// Logger.Infof("NewFlag start 8a %s", c.name)
		// Create a safe copy of the data we need rather than keeping a reference to c
		typeName := configVarName
		targetType := reflect.TypeOf(c.configData[configVarName])

		// Create a var without holding a reference to c
		dvar := &safeVar{
			name:   typeName,
			want:   targetType,
			setter: c.createSetter(configVarName),
		}
		c.FlagSet.Var(dvar, configVarName, configDescription)
		// Logger.Infof("NewFlag start 8b %s", c.name)
	}

	// Logger.Infof("NewFlag end %s", c.name)
}

// Creates a safe setter function that doesn't hold a reference to the Structure
func (c *Structure) createSetter(key string) func(interface{}) error {
	return func(value interface{}) error {
		// Acquire a new lock when setting the value
		configMutex.Lock()
		defer configMutex.Unlock()
		c.changed = true
		return c.set(key, value)
	}
}

// safeVar is a replacement for dynamicVar that doesn't hold a direct reference to Structure
type safeVar struct {
	name   string
	want   reflect.Type
	setter func(interface{}) error
}

// Set implements the flag.Value interface
func (d *safeVar) Set(s string) error {
	if d.want == nil {
		return fmt.Errorf("safeVar has nil type")
	}

	// All the conversion logic from dynamicVar.Set...
	var value = reflect.New(d.want).Elem()

	// The rest of the type conversion logic is the same as in dynamicVar.Set
	// Interface handling
	if d.want.Kind() == reflect.Interface {
		// Try to parse as number first
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			// Check if it's actually an integer
			if float64(int(f)) == f {
				return d.setter(int(f))
			}
			return d.setter(f)
		}

		// Try to parse as bool
		if b, err := strconv.ParseBool(s); err == nil {
			return d.setter(b)
		}

		// Default to string
		return d.setter(s)
	}

	// Basic type conversion for common types
	switch d.want.Kind() {
	case reflect.Bool:
		if b, err := strconv.ParseBool(s); err == nil {
			value.SetBool(b)
		} else {
			return fmt.Errorf("invalid bool value: %s", s)
		}
	case reflect.String:
		value.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i, err := strconv.ParseInt(s, 0, 64); err == nil {
			value.SetInt(i)
		} else {
			return fmt.Errorf("invalid int value: %s", s)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if i, err := strconv.ParseUint(s, 0, 64); err == nil {
			value.SetUint(i)
		} else {
			return fmt.Errorf("invalid uint value: %s", s)
		}
	case reflect.Float32, reflect.Float64:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			value.SetFloat(f)
		} else {
			return fmt.Errorf("invalid float value: %s", s)
		}
	case reflect.Slice:
		// Handle empty string case for slices
		if s == "" {
			value.Set(reflect.MakeSlice(d.want, 0, 0))
			return d.setter(value.Interface())
		}

		// Check if the input looks like a JSON array
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			// Try to parse as JSON for string slices
			if d.want.Elem().Kind() == reflect.String {
				var stringSlice []string
				if err := json.Unmarshal([]byte(s), &stringSlice); err == nil {
					value.Set(reflect.MakeSlice(d.want, len(stringSlice), len(stringSlice)))
					for i, v := range stringSlice {
						value.Index(i).SetString(v)
					}
					return d.setter(value.Interface())
				}
				// If JSON parsing fails, fall back to comma-separated format
			}
		}

		split := strings.Split(s, ",")
		value.Set(reflect.MakeSlice(d.want, len(split), len(split)))

		for i, v := range split {
			v = strings.TrimSpace(v)
			elemValue := value.Index(i)

			// Handle different element types
			switch elemValue.Kind() {
			case reflect.String:
				elemValue.SetString(v)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intVal, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid int in slice at position %d: %s", i, v)
				}
				elemValue.SetInt(intVal)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				uintVal, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid uint in slice at position %d: %s", i, v)
				}
				elemValue.SetUint(uintVal)
			case reflect.Float32, reflect.Float64:
				floatVal, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return fmt.Errorf("invalid float in slice at position %d: %s", i, v)
				}
				elemValue.SetFloat(floatVal)
			case reflect.Bool:
				boolVal, err := strconv.ParseBool(v)
				if err != nil {
					return fmt.Errorf("invalid bool in slice at position %d: %s", i, v)
				}
				elemValue.SetBool(boolVal)
			default:
				return fmt.Errorf("unsupported slice element type: %s", elemValue.Kind())
			}
		}
		return d.setter(value.Interface())
	default:
		// For more complex types, we'll just pass the string
		return d.setter(s)
	}

	return d.setter(value.Interface())
}

// String implements the flag.Value interface
func (d *safeVar) String() string {
	// We don't have direct access to the value, so just return empty string
	return ""
}

// Helper method to wait for flags to be parsed
func (c *Structure) waitForFlagParsed() {
	// Create a ticker to periodically check if flags have been parsed
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// Watch for up to 2 seconds to avoid infinite wait but allow more time for complex applications
	timeout := time.After(2 * time.Second)

	// Use a mutex to protect access to FlagSet
	for {
		select {
		case <-ticker.C:
			configMutex.RLock()
			parsed := c.FlagSet != nil && c.FlagSet.Parsed()
			configMutex.RUnlock()
			if parsed {
				return
			}
		case <-timeout:
			// Don't wait forever, just return
			Logger.Debug("Timeout waiting for flags to be parsed")
			return
		}
	}
}

func (c *Structure) parseFlags() {
	// Logger.Infof("parseFlags start 1 %s", c.name)

	// First check if flags are already parsed under a read lock
	configMutex.RLock()
	alreadyParsed := c.FlagSet.Parsed()
	configMutex.RUnlock()

	// Short circuit if already parsed
	if alreadyParsed {
		Logger.Infof("parseFlags: flags already parsed %s", c.name)
		return
	}

	// Logger.Infof("parseFlags start 3 %s", c.name)
	// Filter out Go test flags
	args := filterTestFlags(os.Args[1:])
	// Logger.Infof("parseFlags start 4 %s", c.name)

	// Parse flags without holding the mutex
	// This allows the Set() methods to acquire the mutex as needed
	var parseErr error
	if parseErr = c.FlagSet.Parse(args); parseErr != nil {
		Logger.Errorf("error parsing flags: %v", parseErr)
	}
	// Logger.Infof("parseFlags start 5 %s", c.name)

	// After parsing, process any boolean flags
	// We need to do this here rather than relying on Set() for bool flags
	// because standalone boolean flags (--flag vs --flag=true) don't call Set()
	if c.boolCallbacks != nil && parseErr == nil {
		// Visit all flags that were set (explicitly or via defaults)
		// Logger.Infof("parseFlags start 6 %s", c.name)
		c.FlagSet.Visit(func(f *flag.Flag) {
			// Logger.Infof("parseFlags start 7 %s", c.name)
			// Check if we have a callback for this flag
			if callback, exists := c.boolCallbacks[f.Name]; exists {
				// Get the flag value
				value, err := strconv.ParseBool(f.Value.String())
				if err == nil {
					// Call the callback with the parsed value
					callback(value)
				} else {
					Logger.Errorf("Failed to parse bool flag %s: %v", f.Name, err)
				}
			}
		})
	}
	// Logger.Infof("parseFlags start 8 %s", c.name)

	// Now acquire the lock to update the changed flag
	configMutex.Lock()
	defer configMutex.Unlock()

	if parseErr == nil {
		// Mark that configuration has changed if flags were successfully parsed
		c.changed = true
	}
	// Logger.Infof("parseFlags start 9 %s", c.name)
}

// GetFlagSet returns the FlagSet used by this configuration
// This allows applications to register the FlagSet with their own flag parsing system
func (c *Structure) GetFlagSet() *flag.FlagSet {
	if c.parent == nil {
		c.InitSelf()
	}

	return c.FlagSet
}

// filterTestFlags removes Go test flags (starting with -test.) from arguments
func filterTestFlags(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-test.") {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}
