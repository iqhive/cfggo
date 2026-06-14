package validcfg

import "testing"

func TestValidatorsHandleNilWithoutPanic(t *testing.T) {
	validators := map[string]Validator{
		"MinLength": MinLength(1),
		"MaxLength": MaxLength(1),
		"Range":     Range(0, 10),
	}

	for name, validator := range validators {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("validator panicked on nil: %v", r)
				}
			}()
			if err := validator(nil); err == nil {
				t.Fatal("validator(nil): expected error, got nil")
			}
		})
	}
}
