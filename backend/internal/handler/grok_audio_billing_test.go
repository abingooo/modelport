//go:build unit

package handler

import (
	"testing"
	"time"

	coderws "github.com/coder/websocket"
)

func TestIsExpectedGrokRealtimeClose(t *testing.T) {
	for _, status := range []coderws.StatusCode{
		coderws.StatusNormalClosure,
		coderws.StatusGoingAway,
		coderws.StatusNoStatusRcvd,
		coderws.StatusAbnormalClosure,
	} {
		if !isExpectedGrokRealtimeClose(coderws.CloseError{Code: status}) {
			t.Fatalf("status %v should be treated as an expected session close", status)
		}
	}
	if isExpectedGrokRealtimeClose(coderws.CloseError{Code: coderws.StatusPolicyViolation}) {
		t.Fatal("policy violations must not be treated as billable normal closes")
	}
}

func TestNewGrokRealtimeUsageResult_BillsElapsedSession(t *testing.T) {
	requireNil := newGrokRealtimeUsageResult("alias", "upstream", 0)
	if requireNil != nil {
		t.Fatal("zero-duration realtime session must not create usage")
	}

	result := newGrokRealtimeUsageResult("alias", "upstream", 90*time.Second)
	if result == nil || result.AudioUsage == nil {
		t.Fatal("elapsed realtime session must create usage even when the proxy later fails")
	}
	if result.Model != "alias" || result.UpstreamModel != "upstream" {
		t.Fatalf("unexpected models: %q / %q", result.Model, result.UpstreamModel)
	}
	if result.AudioUsage.DurationOrUnits != 1.5 {
		t.Fatalf("unexpected billed minutes: %v", result.AudioUsage.DurationOrUnits)
	}
}

func TestGrokRealtimeAccountSlotDoesNotRewriteOwnershipBinding(t *testing.T) {
	if grokRealtimeAccountSlotSessionHash != "" {
		t.Fatal("realtime account-slot admission must not receive the ownership session hash")
	}
}
