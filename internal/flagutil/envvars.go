package flagutil

import "strings"

// EnvVarFlags is a custom flag type for collecting repeated KEY=VALUE environment
// variable arguments. It implements flag.Value, so it can be used with flag.Var
// and passed multiple times on the command line.
type EnvVarFlags []string

func (e *EnvVarFlags) String() string {
	return strings.Join(*e, ",")
}

func (e *EnvVarFlags) Set(value string) error {
	*e = append(*e, value)
	return nil
}

// ParseEnvVarFlags converts a slice of "KEY=VALUE" strings into a map.
// Entries that do not contain "=" are silently ignored.
func ParseEnvVarFlags(flags []string) map[string]string {
	result := make(map[string]string, len(flags))
	for _, kv := range flags {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
