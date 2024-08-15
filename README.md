# cfggo

`cfggo` is a Go package designed to simplify configuration management in Go applications. It provides a flexible and powerful way to handle configuration through various sources such as files, environment variables, and HTTP endpoints. The package supports default values, dynamic configuration updates, and command-line flags.

## Features

- Load configuration from JSON files, environment variables, and HTTP endpoints.
- Support for default values and dynamic configuration updates.
- Command-line flag integration.
- Thread-safe configuration access and updates.
- Customizable configuration handlers.

## Installation

To install the package, run:

```sh
go get github.com/iqhive/cfggo
```

## Usage

### Basic Example

Here's a basic example of how to use `cfggo`:

```go
package main

import (
    "fmt"
    "github.com/iqhive/cfggo"
)

type MyConfig struct {
	cfggo.Structure
	StringField       func() string                 `json:"string_field" help:"My string config item"`
	IntField          func() int                    `json:"int_pos_field" help:"My int config item"`
	BoolField         func() bool                   `json:"bool_true_field" help:"My bool config item"`
	StringSlice       func() []string               `json:"string_slice" help:"My string slice config item"`
	IntSlice          func() []int                  `json:"int_slice" help:"My int slice config item"`
	BoolSlice         func() []bool                 `json:"bool_slice" help:"My bool slice config item"`
	Float64Slice      func() []float64              `json:"float64_slice" help:"My float64 slice config item"`
	Float32Slice      func() []float32              `json:"float32_slice" help:"My float32 slice config item"`
	StringMap         func() map[string]string      `json:"string_map" help:"My string map config item"`
	InterfaceMap      func() map[string]interface{} `json:"interface_map" help:"My interface map config item"`
	DurationField     func() time.Duration          `json:"duration_field" help:"My duration config item"`
	TimeField         func() time.Time              `json:"time_field" help:"My time config item"`
}

func main() {
    mycfg := &MyConfig{}
    cfggo.Init(mycfg, cfggo.WithFileConfig("myconfig.json"))

    fmt.Println("StringField:", mycfg.StringField())
    fmt.Println("IntField:", mycfg.IntField())
    fmt.Println("BoolField:", mycfg.BoolField())
    fmt.Println("StringSlice:", mycfg.StringSlice())
    fmt.Println("IntSlice:", mycfg.IntSlice())
    fmt.Println("BoolSlice:", mycfg.BoolSlice())
    fmt.Println("Float64Slice:", mycfg.Float64Slice())
    fmt.Println("Float32Slice:", mycfg.Float32Slice())
    fmt.Println("StringMap:", mycfg.StringMap())
    fmt.Println("InterfaceMap:", mycfg.InterfaceMap())
    fmt.Println("DurationField:", mycfg.DurationField())
    fmt.Println("TimeField:", mycfg.TimeField())
}
```

### Supported Options

`cfggo` supports the following options:

- `WithFileConfig(filename string) Option`: Sets the config source/dest to a filename.
- `WithHTTPConfig(httpLoader *http.Request, httpSaver *http.Request) Option`: Sets the config source/dest to HTTP requests.
- `WithSkipEnvironment() Option`: Skips loading from environment variables.
- `WithName(name string) Option`: Sets the name of the configuration.


### Command-Line Flag Integration

`cfggo` supports command-line flag integration using the `flag` package. 


### Default Values

`cfggo` supports default values for configuration keys. Here's an example of how to use it:
```go
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

type MyConfig struct {
	StringField       func() string                 `json:"string_field" help:"My string config item"`
	IntField          func() int                    `json:"int_pos_field" help:"My int config item"`
	BoolField         func() bool                   `json:"bool_true_field" help:"My bool config item"`
	StringSlice       func() []string               `json:"string_slice" help:"My string slice config item"`
	IntSlice          func() []int                  `json:"int_slice" help:"My int slice config item"`
	BoolSlice         func() []bool                 `json:"bool_slice" help:"My bool slice config item"`
	Float64Slice      func() []float64              `json:"float64_slice" help:"My float64 slice config item"`
	Float32Slice      func() []float32              `json:"float32_slice" help:"My float32 slice config item"`
	StringMap         func() map[string]string      `json:"string_map" help:"My string map config item"`
	InterfaceMap      func() map[string]interface{} `json:"interface_map" help:"My interface map config item"`
	DurationField     func() time.Duration          `json:"duration_field" help:"My duration config item"`
	TimeField         func() time.Time              `json:"time_field" help:"My time config item"`
}

func main() {
	mycfg := &MyConfig{
        StringField: cfggo.DefaultValue("default_value1"),
        IntField:    cfggo.DefaultValue(123),
        BoolField:  cfggo.DefaultValue(true),
    }
	cfggo.Init(mycfg, cfggo.WithFileConfig("myconfig.json"))

	fmt.Println("StringField:", mycfg.StringField())
	fmt.Println("IntField:", mycfg.IntField())
	fmt.Println("BoolField:", mycfg.BoolField())
	fmt.Println("StringSlice:", mycfg.StringSlice())
	fmt.Println("IntSlice:", mycfg.IntSlice())
	fmt.Println("BoolSlice:", mycfg.BoolSlice())
	fmt.Println("Float64Slice:", mycfg.Float64Slice())
	fmt.Println("Float32Slice:", mycfg.Float32Slice())
	fmt.Println("StringMap:", mycfg.StringMap())
	fmt.Println("InterfaceMap:", mycfg.InterfaceMap())
	fmt.Println("DurationField:", mycfg.DurationField())
	fmt.Println("TimeField:", mycfg.TimeField())
}

### Thread-Safety

`cfggo` is thread-safe, meaning you can access and update the configuration from multiple goroutines concurrently without worrying about data races.
```go
package main

import (
    "fmt"
    "github.com/iqhive/cfggo"
)

type MyConfig struct {
	StringField       func() string                 `json:"string_field" help:"My string config item"`
	IntField          func() int                    `json:"int_pos_field" help:"My int config item"`
}

func main() {
    mycfg := &MyConfig{
        StringField: cfggo.DefaultValue("default_value1"),
        IntField:    cfggo.DefaultValue(123),
    }
    cfggo.Init(mycfg, cfggo.WithFileConfig("myconfig.json"))

    // Start multiple goroutines to access and update the configuration
    for i := 0; i < 10; i++ {
        go func(i int) {
            // Access and update the configuration
            value := mycfg.StringField()
            fmt.Println("Value:", value)

            err := mycfg.Set("string_field", fmt.Sprintf("new_value_%d", i))
            if err != nil {
                fmt.Println(err)
                return
            }
        }(i)
    }

    // Wait for goroutines to finish (for demonstration purposes)
    time.Sleep(2 * time.Second)
}

## Contributing

Contributions are welcome! If you find any issues or have suggestions for new features, please open an issue or submit a pull request on the [GitHub repository](https://github.com/iqhive/cfggo).

## License

`cfggo` is licensed under the [MIT License](LICENSE).

## Acknowledgments

`cfggo` is inspired by the [viper](https://github.com/spf13/viper) configuration package. It aims to provide a more lightweight and flexible alternative with additional features like type safety and custom configuration handlers.