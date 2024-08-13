package cfggo

import (
	"flag"
	"fmt"
	"time"

	"bitbucket.org/iqhive/iqlog/v3"
)

// NewVar creates a new configuration item, using the type of the defaultValue
func (c *GenericConfig) NewVar(configVarName string, defaultValue interface{}, configDescription string) {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}
	c.configData[configVarName] = defaultValue

	if flag.Lookup(configVarName) != nil {
		iqlog.Errorf("Flag %s is already set, skipping...\n", configVarName)
		return
	}

	switch val := defaultValue.(type) {
	case string:
		flag.String(configVarName, val, configDescription)
	case bool:
		flag.Bool(configVarName, val, configDescription)
	case float64:
		flag.Float64(configVarName, val, configDescription)
	case float32:
		flag.Float64(configVarName, float64(val), configDescription)
	case int64:
		flag.Int64(configVarName, val, configDescription)
	case int:
		flag.Int(configVarName, val, configDescription)
	case int8:
		flag.Int(configVarName, int(val), configDescription)
	case int16:
		flag.Int(configVarName, int(val), configDescription)
	case int32:
		flag.Int(configVarName, int(val), configDescription)
	case []int:
		flag.Var(newIntSliceValue(val), configVarName, configDescription)
	case []string:
		flag.Var(newStringSliceValue(val), configVarName, configDescription)
	case []bool:
		flag.Var(newBoolSliceValue(val), configVarName, configDescription)
	case []float64:
		flag.Var(newFloat64SliceValue(val), configVarName, configDescription)
	case []float32:
		flag.Var(newFloat32SliceValue(val), configVarName, configDescription)
	case []int64:
		flag.Var(newInt64SliceValue(val), configVarName, configDescription)
	case time.Duration:
		flag.Duration(configVarName, val, configDescription)
	case time.Time:
		flag.String(configVarName, val.Format(time.RFC3339), configDescription)
	case map[string]string:
		flag.Var(newStringMapValue(val), configVarName, configDescription)
	case map[string]interface{}:
		flag.Var(newInterfaceMapValue(val), configVarName, configDescription)
	case uint64:
		flag.Uint64(configVarName, val, configDescription)
	case uint:
		flag.Uint(configVarName, val, configDescription)
	case uint8:
		flag.Uint(configVarName, uint(val), configDescription)
	case uint16:
		flag.Uint(configVarName, uint(val), configDescription)
	case uint32:
		flag.Uint(configVarName, uint(val), configDescription)
	default:
		_ = val
		panic(fmt.Sprintf("Unknown/unsupported type (%T) for config variable %s", val, configVarName))
	}
}
