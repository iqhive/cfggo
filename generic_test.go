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
	GenericConfig
	StringEmptyField  func() string                 `json:"string_empty_field" config:"string_empty_field"`
	StringField       func() string                 `json:"string_field" config:"string_field"`
	IntZeroField      func() int                    `json:"int_zero_field" config:"int_zero_field"`
	IntNegField       func() int                    `json:"int_neg_field" config:"int_neg_field"`
	IntPosField       func() int                    `json:"int_pos_field" config:"int_pos_field"`
	BoolFalseField    func() bool                   `json:"bool_false_field" config:"bool_false_field"`
	BoolTrueField     func() bool                   `json:"bool_true_field" config:"bool_true_field"`
	StringSlice       func() []string               `json:"string_slice" config:"string_slice"`
	EmptyStringSlice  func() []string               `json:"empty_string_slice" config:"empty_string_slice"`
	NilStringSlice    func() []string               `json:"nil_string_slice" config:"nil_string_slice"`
	IntSlice          func() []int                  `json:"int_slice" config:"int_slice"`
	EmptyIntSlice     func() []int                  `json:"empty_int_slice" config:"empty_int_slice"`
	NilIntSlice       func() []int                  `json:"nil_int_slice" config:"nil_int_slice"`
	BoolSlice         func() []bool                 `json:"bool_slice" config:"bool_slice"`
	EmptyBoolSlice    func() []bool                 `json:"empty_bool_slice" config:"empty_bool_slice"`
	NilBoolSlice      func() []bool                 `json:"nil_bool_slice" config:"nil_bool_slice"`
	Float64Slice      func() []float64              `json:"float64_slice" config:"float64_slice"`
	EmptyFloat64Slice func() []float64              `json:"empty_float64_slice" config:"empty_float64_slice"`
	NilFloat64Slice   func() []float64              `json:"nil_float64_slice" config:"nil_float64_slice"`
	Float32Slice      func() []float32              `json:"float32_slice" config:"float32_slice"`
	EmptyFloat32Slice func() []float32              `json:"empty_float32_slice" config:"empty_float32_slice"`
	NilFloat32Slice   func() []float32              `json:"nil_float32_slice" config:"nil_float32_slice"`
	StringMap         func() map[string]string      `json:"string_map" config:"string_map"`
	EmptyStringMap    func() map[string]string      `json:"empty_string_map" config:"empty_string_map"`
	NilStringMap      func() map[string]string      `json:"nil_string_map" config:"nil_string_map"`
	InterfaceMap      func() map[string]interface{} `json:"interface_map" config:"interface_map"`
	EmptyInterfaceMap func() map[string]interface{} `json:"empty_interface_map" config:"empty_interface_map"`
	NilInterfaceMap   func() map[string]interface{} `json:"nil_interface_map" config:"nil_interface_map"`
	DurationField     func() time.Duration          `json:"duration_field" config:"duration_field"`
	TimeField         func() time.Time              `json:"time_field" config:"time_field"`
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
	config.Init(config, "test", "")

	if config.name != "test" {
		t.Errorf("Expected name to be 'test', but got %v", config.name)
	}
	if config.filename != "" {
		t.Errorf("Expected filename to be '', but got %v", config.filename)
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
	config.Init(config, "test", "test.json")

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
	config.Init(config, "test", "test.json")

	if config.StringField() != "env_value" {
		t.Errorf("Expected 'env_value', but got %v", config.StringField())
	}
}

func TestCommandLineFlags(t *testing.T) {
	flag.String("string_field", "flag_value", "string field")
	flag.Parse()

	config := NewTestConfig()
	config.Init(config, "test", "test.json")
	// Reset flags for other tests
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Modify the TestEnvironmentVariables test
	t.Run("TestEnvironmentVariables", func(t *testing.T) {
		os.Setenv("STRING_FIELD", "env_value")
		defer os.Unsetenv("STRING_FIELD")

		config := NewTestConfig()
		config.Init(config, "test", "test.json")

		if config.StringField() != "env_value" {
			t.Errorf("Expected 'env_value', but got %v", config.StringField())
		}
	})

	// Modify the TestSaveAndLoadConfig test
	t.Run("TestSaveAndLoadConfig", func(t *testing.T) {
		config := NewTestConfig()
		config.Init(config, "test", "test_save_load.json")
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
		newConfig.Init(newConfig, "test", "test_save_load.json")
		if err != nil {
			t.Errorf("Expected no error during load, but got %v", err)
		}

		// Verify the loaded value
		if newConfig.StringField() != "test_value" {
			t.Errorf("Expected 'test_value', but got %v", newConfig.StringField())
		}
	})

	if config.StringField() != "flag_value" {
		t.Errorf("Expected 'flag_value', but got %v", config.StringField())
	}
}
