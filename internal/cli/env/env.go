// Package env provides helper functions for looking up environment variables
// with support for multiple fallback keys and type conversion.
package env

import (
	"os"
	"strconv"
	"strings"
)

// LookupEnv searches for environment variables by the given keys in order.
// It returns the first non-empty trimmed value found and true, or empty string
// and false if no matching non-empty variable is found.
func LookupEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed, true
			}
		}
	}
	return "", false
}

// LookupEnvInt searches for environment variables by the given keys and
// attempts to parse the value as an integer. It returns the parsed integer
// and true if successful, or 0 and false if no valid integer is found.
func LookupEnvInt(keys ...string) (int, bool) {
	if value, ok := LookupEnv(keys...); ok {
		if n, err := strconv.Atoi(value); err == nil {
			return n, true
		}
	}
	return 0, false
}

// LookupEnvBool searches for environment variables by the given keys and
// interprets the value as a boolean. Values "true", "1", and "yes" (case-insensitive)
// are considered true. It returns the boolean value and true if found,
// or false and false if no matching variable is found.
func LookupEnvBool(keys ...string) (bool, bool) {
	if value, ok := LookupEnv(keys...); ok {
		v := strings.ToLower(value)
		return v == "true" || v == "1" || v == "yes", true
	}
	return false, false
}
