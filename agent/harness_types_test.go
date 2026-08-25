package agent_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
)

func TestHarnessResultPreservesZeroValueSuccessAndFailure(t *testing.T) {
	success := agent.OK(0)
	if !success.OK || success.Value != 0 || success.Error != nil {
		t.Fatalf("OK(0) = %#v", success)
	}
	if value, err := agent.GetOrThrow(success); value != 0 || err != nil {
		t.Fatalf("GetOrThrow(OK(0)) = (%d, %v)", value, err)
	}

	wantErr := errors.New("failed")
	failure := agent.Err[int](wantErr)
	if failure.OK || !errors.Is(failure.Error, wantErr) {
		t.Fatalf("Err(failed) = %#v", failure)
	}
	if _, err := agent.GetOrThrow(failure); !errors.Is(err, wantErr) {
		t.Fatalf("GetOrThrow(Err(failed)) error = %v", err)
	}
}
