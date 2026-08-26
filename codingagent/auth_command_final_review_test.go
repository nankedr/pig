package codingagent

import (
	"errors"
	"testing"
)

func TestParseAuthCommandAcceptsPinnedMinExpiryUnits(t *testing.T) {
	validDurations := []string{"30M", "1H", "45S", "250MS"}
	for _, duration := range validDurations {
		t.Run(duration, func(t *testing.T) {
			operation, err := parseAuthCommand([]string{
				"auth", "print-bearer-token",
				"--provider", "openai",
				"--min-expiry", duration,
			})
			if err != nil {
				t.Fatalf("parseAuthCommand(--min-expiry %q) error = %v, want nil", duration, err)
			}
			if operation != "command.auth.print-bearer-token" {
				t.Fatalf("parseAuthCommand(--min-expiry %q) operation = %q, want %q", duration, operation, "command.auth.print-bearer-token")
			}
		})
	}

	t.Run("unsupported unit", func(t *testing.T) {
		_, err := parseAuthCommand([]string{
			"auth", "print-bearer-token",
			"--provider", "openai",
			"--min-expiry", "30D",
		})
		var argumentError *CLIArgumentError
		if !errors.As(err, &argumentError) {
			t.Fatalf("parseAuthCommand(--min-expiry 30D) error = %T (%v), want *CLIArgumentError", err, err)
		}
		const want = "--min-expiry must use a duration such as 30m or 1h"
		if argumentError.Message != want {
			t.Fatalf("CLIArgumentError.Message = %q, want %q", argumentError.Message, want)
		}
	})
}
