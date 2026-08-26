package ai_test

import (
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestPiMessagesResponseErrorIsAnInertPublicCarrier(t *testing.T) {
	code := "rate_limit"
	details := map[string]any{"request_id": "req-1"}
	err := &ai.PiMessagesResponseError{
		Message:           "request rejected",
		Code:              &code,
		DiagnosticDetails: details,
	}
	if got := err.Error(); got != "request rejected" {
		t.Fatalf("Error() = %q, want request rejected", got)
	}
	if err.Code == nil || *err.Code != code || !reflect.DeepEqual(err.DiagnosticDetails, details) {
		t.Fatalf("error carrier = %#v, want preserved code and diagnostic details", err)
	}
	var nilError *ai.PiMessagesResponseError
	if got := nilError.Error(); got != "" {
		t.Fatalf("nil Error() = %q, want empty string", got)
	}
}
