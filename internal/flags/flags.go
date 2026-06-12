package flags

import "strings"

// FilterTestFlags removes Go test flags (those starting with "-test.") from
// a slice of command-line arguments so they do not interfere with application
// flag parsing.
func FilterTestFlags(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-test.") {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}
