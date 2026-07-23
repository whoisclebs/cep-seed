package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/whoisclebs/cep-seed/internal/config"
)

func TestEnvDefault(test *testing.T) {
	test.Run("returns_env_value_when_set", func(subtest *testing.T) {
		os.Setenv("TEST_ENV_DEFAULT_CO", "from-env")
		defer os.Unsetenv("TEST_ENV_DEFAULT_CO")
		if got := config.EnvDefault("TEST_ENV_DEFAULT_CO", "fallback"); got != "from-env" {
			subtest.Errorf("EnvDefault = %q, want %q", got, "from-env")
		}
	})

	test.Run("returns_fallback_when_env_unset", func(subtest *testing.T) {
		os.Unsetenv("TEST_ENV_UNSET_CO")
		if got := config.EnvDefault("TEST_ENV_UNSET_CO", "fallback"); got != "fallback" {
			subtest.Errorf("EnvDefault = %q, want %q", got, "fallback")
		}
	})

	test.Run("returns_fallback_when_env_empty", func(subtest *testing.T) {
		os.Setenv("TEST_ENV_EMPTY_CO", "")
		defer os.Unsetenv("TEST_ENV_EMPTY_CO")
		if got := config.EnvDefault("TEST_ENV_EMPTY_CO", "fallback"); got != "fallback" {
			subtest.Errorf("EnvDefault = %q, want %q", got, "fallback")
		}
	})
}

func TestParseEnvDuration(test *testing.T) {
	test.Run("returns_env_value_when_valid", func(subtest *testing.T) {
		os.Setenv("TEST_DUR_CO", "30s")
		defer os.Unsetenv("TEST_DUR_CO")
		got, err := config.ParseEnvDuration("TEST_DUR_CO")
		if err != nil {
			subtest.Fatalf("ParseEnvDuration: %v", err)
		}
		if got != 30*time.Second {
			subtest.Errorf("ParseEnvDuration = %v, want %v", got, 30*time.Second)
		}
	})

	test.Run("returns_error_on_invalid_format", func(subtest *testing.T) {
		os.Setenv("TEST_DUR_BAD_CO", "not-a-duration")
		defer os.Unsetenv("TEST_DUR_BAD_CO")
		_, err := config.ParseEnvDuration("TEST_DUR_BAD_CO")
		if err == nil {
			subtest.Fatal("expected error for invalid duration")
		}
	})

	test.Run("returns_zero_when_env_unset", func(subtest *testing.T) {
		os.Unsetenv("TEST_DUR_UNSET_CO")
		got, err := config.ParseEnvDuration("TEST_DUR_UNSET_CO")
		if err != nil {
			subtest.Fatalf("ParseEnvDuration: %v", err)
		}
		if got != 0 {
			subtest.Errorf("ParseEnvDuration = %v, want 0", got)
		}
	})

	test.Run("returns_zero_when_env_empty", func(subtest *testing.T) {
		os.Setenv("TEST_DUR_EMPTY_CO", "")
		defer os.Unsetenv("TEST_DUR_EMPTY_CO")
		got, err := config.ParseEnvDuration("TEST_DUR_EMPTY_CO")
		if err != nil {
			subtest.Fatalf("ParseEnvDuration: %v", err)
		}
		if got != 0 {
			subtest.Errorf("ParseEnvDuration = %v, want 0", got)
		}
	})
}

func TestEnvBool(test *testing.T) {
	testCases := []struct {
		envVal   string
		def      bool
		expected bool
	}{
		{"true", false, true},
		{"1", false, true},
		{"false", true, false},
		{"0", true, false},
		{"", true, true},
		{"", false, false},
		{"invalid", true, false},
	}
	for _, testCase := range testCases {
		name := "env=" + testCase.envVal + "_def=" + map[bool]string{true: "true", false: "false"}[testCase.def]
		test.Run(name, func(subtest *testing.T) {
			if testCase.envVal != "" {
				os.Setenv("TEST_BOOL_CO", testCase.envVal)
				defer os.Unsetenv("TEST_BOOL_CO")
			} else {
				os.Unsetenv("TEST_BOOL_CO")
			}
			if got := config.EnvBool("TEST_BOOL_CO", testCase.def); got != testCase.expected {
				subtest.Errorf("EnvBool = %v, want %v", got, testCase.expected)
			}
		})
	}
}
