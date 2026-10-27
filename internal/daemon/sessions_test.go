package daemon

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GringoCoco/cardinal/internal/config"
	"github.com/GringoCoco/cardinal/internal/events"
)

func TestSessionStateTransitions(t *testing.T) {
	tests := []struct {
		event string
		state string
	}{
		{events.SessionStart, "running"},
		{events.PermissionRequ, "running"},
		{events.Notification, "waiting"},
		{events.Stop, "completed"},
		{events.StopFailure, "error"},
		{events.StopInterrupted, "stopped"},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			state, _ := stateAndActivity(&events.Hook{Event: tt.event})
			if state != tt.state {
				t.Fatalf("state = %q, want %q", state, tt.state)
			}
		})
	}
}

func TestExternalSessionIsNotControllable(t *testing.T) {
	if canControlSession(nil) || canControlSession(&managedProcess{}) {
		t.Fatal("external session must not be controllable")
	}
}

func TestPermissionWithoutDecisionChannelFallsBackImmediately(t *testing.T) {
	d := New(&config.Config{DecisionWaitSec: 1})
	req := httptest.NewRequest("POST", "/hook", strings.NewReader(`{
		"hook_event_name":"PermissionRequest",
		"session_id":"s1",
		"coucou_agent":"opencode",
		"tool_name":"shell",
		"tool_input":{"command":"go test ./..."}
	}`))
	w := httptest.NewRecorder()
	started := time.Now()
	d.handleHook(w, req)
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("fallback took %v; agent would be blocked", elapsed)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "{}" {
		t.Fatalf("response = %q, want {}", got)
	}
}
