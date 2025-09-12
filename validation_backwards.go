package cfggo

import "github.com/iqhive/cfggo/validcfg"

func Required() validcfg.Validator {
	return validcfg.Required()
}

func MinLength(min int) validcfg.Validator {
	return validcfg.MinLength(min)
}

func MaxLength(max int) validcfg.Validator {
	return validcfg.MaxLength(max)
}

func Range(min, max float64) validcfg.Validator {
	return validcfg.Range(min, max)
}

func OneOf(min, max float64) validcfg.Validator {
	return validcfg.Range(min, max)
}

func Regex(pattern string) validcfg.Validator {
	return validcfg.Regex(pattern)
}

func Email() validcfg.Validator {
	return validcfg.Email()
}

func URL() validcfg.Validator {
	return validcfg.URL()
}

// All returns a validator that checks if all validators pass
func All(validators ...validcfg.Validator) validcfg.Validator {
	return validcfg.All(validators...)
}

// Any returns a validator that checks if any validator passes
func Any(validators ...validcfg.Validator) validcfg.Validator {
	return validcfg.Any(validators...)
}

// Custom returns a validator that uses a custom function
func Custom(fn func(interface{}) error) validcfg.Validator {
	return validcfg.Custom(fn)
}
