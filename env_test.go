package cfggo

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	// Setup test cases
	tests := []struct {
		name          string
		envVars       map[string]string
		configData    map[string]interface{}
		skipEnv       bool
		expected      map[string]interface{}
		expectedTypes map[string]reflect.Type
	}{
		{
			name: "Load string environment variable",
			envVars: map[string]string{
				"TEST_KEY": "test_value",
			},
			configData: map[string]interface{}{
				"test.key": "",
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.key": "test_value",
			},
			expectedTypes: map[string]reflect.Type{
				"test.key": reflect.TypeOf(""),
			},
		},
		{
			name: "Load int environment variable",
			envVars: map[string]string{
				"TEST_INT": "42",
			},
			configData: map[string]interface{}{
				"test.int": 0,
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.int": 42,
			},
			expectedTypes: map[string]reflect.Type{
				"test.int": reflect.TypeOf(0),
			},
		},
		{
			name: "Load int64 environment variable",
			envVars: map[string]string{
				"TEST_INT64": "64",
			},
			configData: map[string]interface{}{
				"test.int64": int64(0),
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.int64": int64(64),
			},
			expectedTypes: map[string]reflect.Type{
				"test.int64": reflect.TypeOf(int64(0)),
			},
		},
		{
			name: "Load time.Time environment variable",
			envVars: map[string]string{
				"TEST_TIME": "2023-10-01T15:04:05Z",
			},
			configData: map[string]interface{}{
				"test.time": time.Time{},
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.time": time.Date(2023, 10, 1, 15, 4, 5, 0, time.UTC),
			},
			expectedTypes: map[string]reflect.Type{
				"test.time": reflect.TypeOf(time.Time{}),
			},
		},
		{
			name: "Load time.Duration environment variable",
			envVars: map[string]string{
				"TEST_DURATION": "1h30m",
			},
			configData: map[string]interface{}{
				"test.duration": time.Duration(0),
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.duration": time.Hour + 30*time.Minute,
			},
			expectedTypes: map[string]reflect.Type{
				"test.duration": reflect.TypeOf(time.Duration(0)),
			},
		},
		{
			name: "Load bool environment variable",
			envVars: map[string]string{
				"TEST_BOOL": "true",
			},
			configData: map[string]interface{}{
				"test.bool": false,
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.bool": true,
			},
			expectedTypes: map[string]reflect.Type{
				"test.bool": reflect.TypeOf(false),
			},
		},
		{
			name: "Load []string environment variable",
			envVars: map[string]string{
				"TEST_STRINGS": "a,b,c",
			},
			configData: map[string]interface{}{
				"test.strings": []string{},
			},
			skipEnv: false,
			expected: map[string]interface{}{
				"test.strings": []string{"a", "b", "c"},
			},
			expectedTypes: map[string]reflect.Type{
				"test.strings": reflect.TypeOf([]string{}),
			},
		},
		{
			name: "Skip environment variables",
			envVars: map[string]string{
				"TEST_KEY": "test_value",
			},
			configData: map[string]interface{}{
				"test.key": "",
			},
			skipEnv: true,
			expected: map[string]interface{}{
				"test.key": "",
			},
			expectedTypes: map[string]reflect.Type{
				"test.key": reflect.TypeOf(""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			// Create a Structure instance
			c := &Structure{
				configData: tt.configData,
				skipEnv:    tt.skipEnv,
			}

			// Call loadFromEnv
			c.loadFromEnv()

			// Check if the configData matches the expected result and types
			for key, expectedValue := range tt.expected {
				if !reflect.DeepEqual(c.configData[key], expectedValue) {
					t.Errorf("For key %s, expected value %v, got %v", key, expectedValue, c.configData[key])
				}
				if reflect.TypeOf(c.configData[key]) != tt.expectedTypes[key] {
					t.Errorf("For key %s, expected type %v, got %v", key, tt.expectedTypes[key], reflect.TypeOf(c.configData[key]))
				}
			}
		})
	}
}
