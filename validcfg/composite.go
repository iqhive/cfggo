package validcfg

import "fmt"

// All returns a validator that checks if all validators pass
func All(validators ...Validator) Validator {
	return func(value interface{}) error {
		for _, validator := range validators {
			if err := validator(value); err != nil {
				return err
			}
		}

		return nil
	}
}

// Any returns a validator that checks if any validator passes
func Any(validators ...Validator) Validator {
	return func(value interface{}) error {
		var errors []error

		for _, validator := range validators {
			if err := validator(value); err == nil {
				return nil
			} else {
				errors = append(errors, err)
			}
		}

		return fmt.Errorf("none of the validators passed: %v", errors)
	}
}
