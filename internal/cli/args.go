package cli

import "strings"

// NormalizeArgs supports the requested -rps spelling alongside Cobra's --rps flag.
func NormalizeArgs(args []string) []string {
	normalized := make([]string, len(args))
	afterDash := false
	for i, arg := range args {
		if arg == "--" {
			afterDash = true
		}
		if !afterDash && (arg == "-rps" || strings.HasPrefix(arg, "-rps=")) {
			arg = "-" + arg
		}
		normalized[i] = arg
	}
	return normalized
}
