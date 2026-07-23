// Package config provides shared environment-variable helpers used by
// cep-updater and other CLI entry points. Keeping them here avoids
// duplication across cmd/ packages without creating a generic "util"
// package.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// EnvDefault returns the environment variable value if set, otherwise def.
func EnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ParseEnvDuration reads key from environment and parses it as a duration.
// Returns the parsed value and nil error when the variable is set and valid.
// Returns zero and nil error when the variable is unset or empty.
// Returns an error when the variable is set but has an invalid format.
func ParseEnvDuration(key string) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s value %q: %w", key, v, err)
	}
	return d, nil
}

// EnvBool returns the parsed boolean from an environment variable if set,
// otherwise def. "1", "true", "yes" are true; everything else is false.
func EnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		return err == nil && b
	}
	return def
}
