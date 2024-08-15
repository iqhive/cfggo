package cfggo

import (
	"flag"
	"os"
	"reflect"
	"testing"
	"time"
)

var timeNow = time.Now()

type TestConfig struct {
	Structure
	StringEmptyField  func() string                 `json:"string_empty_field" help:"Test field"`
	StringField       func() string                 `json:"string_field" help:"Test field"`
	IntZeroField      func() int                    `json:"int_zero_field" help:"Test field"`
	IntNegField       func() int                    `json:"int_neg_field" help:"Test field"`
	IntPosField       func() int                    `json:"int_pos_field" help:"Test field"`
	BoolFalseField    func() bool                   `json:"bool_false_field" help:"Test field"`
	BoolTrueField     func() bool                   `json:"bool_true_field" help:"Test field"`
	StringSlice       func() []string               `json:"string_slice" help:"Test field"`
	EmptyStringSlice  func() []string               `json:"empty_string_slice" help:"Test field"`
	NilStringSlice    func() []string               `json:"nil_string_slice" help:"Test field"`
	IntSlice          func() []int                  `json:"int_slice" help:"Test field"`
	EmptyIntSlice     func() []int                  `json:"empty_int_slice" help:"Test field"`
	NilIntSlice       func() []int                  `json:"nil_int_slice" help:"Test field"`
	BoolSlice         func() []bool                 `json:"bool_slice" help:"Test field"`
	EmptyBoolSlice    func() []bool                 `json:"empty_bool_slice" help:"Test field"`
	NilBoolSlice      func() []bool                 `json:"nil_bool_slice" help:"Test field"`
	Float64Slice      func() []float64              `json:"float64_slice" help:"Test field"`
	EmptyFloat64Slice func() []float64              `json:"empty_float64_slice" help:"Test field"`
	NilFloat64Slice   func() []float64              `json:"nil_float64_slice" help:"Test field"`
	Float32Slice      func() []float32              `json:"float32_slice" help:"Test field"`
	EmptyFloat32Slice func() []float32              `json:"empty_float32_slice" help:"Test field"`
	NilFloat32Slice   func() []float32              `json:"nil_float32_slice" help:"Test field"`
	StringMap         func() map[string]string      `json:"string_map" help:"Test field"`
	EmptyStringMap    func() map[string]string      `json:"empty_string_map" help:"Test field"`
	NilStringMap      func() map[string]string      `json:"nil_string_map" help:"Test field"`
	InterfaceMap      func() map[string]interface{} `json:"interface_map" help:"Test field"`
	EmptyInterfaceMap func() map[string]interface{} `json:"empty_interface_map" help:"Test field"`
	NilInterfaceMap   func() map[string]interface{} `json:"nil_interface_map" help:"Test field"`
	DurationField     func() time.Duration          `json:"duration_field" help:"Test field"`
	TimeField         func() time.Time              `json:"time_field" help:"Test field"`
}

func NewTestConfig() *TestConfig {
	if timeNow.IsZero() {
		timeNow = time.Now()
	}
	return &TestConfig{
		StringEmptyField:  DefaultValue(""),
		StringField:       DefaultValue("default_string"),
		IntZeroField:      DefaultValue(0),
		IntNegField:       DefaultValue(-1),
		IntPosField:       DefaultValue(1),
		BoolFalseField:    DefaultValue(false),
		BoolTrueField:     DefaultValue(true),
		StringSlice:       DefaultValue([]string{"default"}),
		EmptyStringSlice:  DefaultValue([]string{}),
		NilStringSlice:    DefaultValue([]string(nil)),
		IntSlice:          DefaultValue([]int{1}),
		EmptyIntSlice:     DefaultValue([]int{}),
		NilIntSlice:       DefaultValue([]int(nil)),
		BoolSlice:         DefaultValue([]bool{true}),
		EmptyBoolSlice:    DefaultValue([]bool{}),
		NilBoolSlice:      DefaultValue([]bool(nil)),
		Float64Slice:      DefaultValue([]float64{1.0}),
		EmptyFloat64Slice: DefaultValue([]float64{}),
		NilFloat64Slice:   DefaultValue([]float64(nil)),
		Float32Slice:      DefaultValue([]float32{1.0}),
		EmptyFloat32Slice: DefaultValue([]float32{}),
		NilFloat32Slice:   DefaultValue([]float32(nil)),
		StringMap:         DefaultValue(map[string]string{"key": "value"}),
		EmptyStringMap:    DefaultValue(map[string]string{}),
		NilStringMap:      DefaultValue(map[string]string(nil)),
		InterfaceMap:      DefaultValue(map[string]interface{}{"key": "value"}),
		EmptyInterfaceMap: DefaultValue(map[string]interface{}{}),
		NilInterfaceMap:   DefaultValue(map[string]interface{}(nil)),
		DurationField:     DefaultValue(time.Duration(0)),
		TimeField:         DefaultValue(timeNow),
	}
}

func NewMutatedTestConfig() *TestConfig {
	config := NewTestConfig()

	config.StringField = DefaultValue("mutated_string")
	config.IntNegField = DefaultValue(-2)
	config.IntPosField = DefaultValue(2)
	config.BoolTrueField = DefaultValue(false)
	config.StringSlice = DefaultValue([]string{"mutated"})
	config.IntSlice = DefaultValue([]int{2})
	config.BoolSlice = DefaultValue([]bool{false})
	config.Float64Slice = DefaultValue([]float64{2.0})
	config.Float32Slice = DefaultValue([]float32{2.0})
	config.StringMap = DefaultValue(map[string]string{"mutated_key": "mutated_value"})
	config.InterfaceMap = DefaultValue(map[string]interface{}{"mutated_key": "mutated_value"})
	config.DurationField = DefaultValue(time.Duration(1 * time.Hour))
	config.TimeField = DefaultValue(timeNow.Add(24 * time.Hour))

	return config
}

func TestDefaultValue(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected interface{}
	}{
		{"test", "test"},
		{123, 123},
		{true, true},
		{[]string{"a", "b"}, []string{"a", "b"}},
		{map[string]string{"key": "value"}, map[string]string{"key": "value"}},
		{time.Duration(5 * time.Second), time.Duration(5 * time.Second)},
		{timeNow.Format(time.RFC3339), timeNow.Format(time.RFC3339)},
	}

	for _, test := range tests {
		result := reflect.ValueOf(DefaultValue(test.input)).Call(nil)[0].Interface()
		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("Expected %v, but got %v", test.expected, result)
		}
	}
}
func TestInit(t *testing.T) {
	timeNow = time.Now()
	config := NewTestConfig()
	config.Init(config)

	if config.name != "TestConfig" {
		t.Errorf("Expected name to be 'TestConfig', but got %v", config.name)
	}
	if config.parent != config {
		t.Errorf("Expected parent to be config, but got %v", config.parent)
	}

	if config.StringField() != "default_string" {
		t.Errorf("Expected 'default_string', but got %v", config.StringField())
	}
	if config.IntZeroField() != 0 {
		t.Errorf("Expected 0, but got %v", config.IntZeroField())
	}
	if config.IntNegField() != -1 {
		t.Errorf("Expected -1, but got %v", config.IntNegField())
	}
	if config.IntPosField() != 1 {
		t.Errorf("Expected 1, but got %v", config.IntPosField())
	}
	if config.BoolFalseField() != false {
		t.Errorf("Expected false, but got %v", config.BoolFalseField())
	}
	if config.BoolTrueField() != true {
		t.Errorf("Expected true, but got %v", config.BoolTrueField())
	}
	if len(config.StringSlice()) != 1 || config.StringSlice()[0] != "default" {
		t.Errorf("Expected slice with 'default', but got %v", config.StringSlice())
	}
	if len(config.StringMap()) != 1 || config.StringMap()["key"] != "value" {
		t.Errorf("Expected map with 'key': 'value', but got %v", config.StringMap())
	}
	if len(config.InterfaceMap()) != 1 || config.InterfaceMap()["key"] != "value" {
		t.Errorf("Expected map with 'key': 'value', but got %v", config.InterfaceMap())
	}
	if config.DurationField() != 0 {
		t.Errorf("Expected 0 duration, but got %v", config.DurationField())
	}
	if config.TimeField() != timeNow {
		t.Errorf("Expected %v, but got %v", timeNow, config.TimeField())
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	config := NewTestConfig()
	config.Init(config, WithFileConfig("test.json"))

	// Set a value to be saved
	config.Set("string_field", "test_value")
	config.configData["string_field"] = "test_value"

	// Save the configuration
	err := config.saveConfig()
	if err != nil {
		t.Errorf("Expected no error during save, but got %v", err)
	}

	// Load the configuration
	err = config.loadConfig()
	if err != nil {
		t.Errorf("Expected no error during load, but got %v", err)
	}

	// Verify the loaded value
	if config.StringField() != "test_value" {
		t.Errorf("Expected 'test_value', but got %v", config.StringField())
	}
}

func TestEnvironmentVariables(t *testing.T) {
	os.Setenv("STRING_FIELD", "env_value")
	defer os.Unsetenv("STRING_FIELD")

	config := NewTestConfig()
	config.Init(config, WithFileConfig("test.json"))

	if config.StringField() != "env_value" {
		t.Errorf("Expected 'env_value', but got %v", config.StringField())
	}
}

func TestCommandLineFlags(t *testing.T) {
	flag.String("string_field", "flag_value", "string field")
	flag.Parse()

	config := NewTestConfig()
	config.Init(config, WithFileConfig("test.json"))
	// Reset flags for other tests
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Modify the TestEnvironmentVariables test
	t.Run("TestEnvironmentVariables", func(t *testing.T) {
		os.Setenv("STRING_FIELD", "env_value")
		defer os.Unsetenv("STRING_FIELD")

		config := NewTestConfig()
		config.Init(config, WithFileConfig("test.json"))

		if config.StringField() != "env_value" {
			t.Errorf("Expected 'env_value', but got %v", config.StringField())
		}
	})

	// Modify the TestSaveAndLoadConfig test
	t.Run("TestSaveAndLoadConfig", func(t *testing.T) {
		config := NewTestConfig()
		config.Init(config, WithFileConfig("test_save_load.json"))
		defer os.Remove("test_save_load.json")

		// Set a value to be saved
		config.StringField = func() string { return "test_value" }
		config.configData["string_field"] = "test_value"

		// Save the configuration
		err := config.saveConfig()
		if err != nil {
			t.Errorf("Expected no error during save, but got %v", err)
		}

		// Create a new config instance to load the saved data
		newConfig := NewTestConfig()
		newConfig.Init(newConfig, WithFileConfig("test_save_load.json"))
		if err != nil {
			t.Errorf("Expected no error during load, but got %v", err)
		}

		// Verify the loaded value
		if newConfig.StringField() != "test_value" {
			t.Errorf("Expected 'test_value', but got %v", newConfig.StringField())
		}
	})

	if config.StringField() != "test_value" {
		t.Errorf("Expected 'test_value', but got %v", config.StringField())
	}
}
