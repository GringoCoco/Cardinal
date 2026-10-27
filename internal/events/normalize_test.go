package events

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		// Claude Code — канон, проходят как есть
		"Stop":              Stop,
		"Notification":      Notification,
		"PermissionRequest": PermissionRequ,
		// Gemini/Antigravity
		"BeforeTool":  PreToolUse,
		"AfterModel":  PostToolUse,
		"BeforeAgent": UserPromptSubmit,
		"AfterAgent":  Stop,
		"startup":     SessionStart,
		"exit":        SessionEnd,
		// OpenCode
		"session.idle":  Stop,
		"session.error": StopFailure,
		// неизвестное — проходит без изменений
		"MysteryEvent": "MysteryEvent",
	}
	for raw, want := range cases {
		if got := Normalize(raw); got != want {
			t.Errorf("Normalize(%q) = %q, ждали %q", raw, got, want)
		}
	}
}

func TestHookHelpers(t *testing.T) {
	h := Hook{
		Event:     "BeforeTool",
		Agent:     "opencode",
		CWD:       "/Users/artem/Projects/atlas",
		Tool:      "Bash",
		ToolInput: map[string]any{"command": "npm install"},
	}
	if h.Canonical() != PreToolUse {
		t.Errorf("Canonical = %s, ждали %s", h.Canonical(), PreToolUse)
	}
	if h.NamedAgent() != "opencode" {
		t.Errorf("NamedAgent = %s", h.NamedAgent())
	}
	if got := h.Project(); got != "atlas" {
		t.Errorf("Project = %s", got)
	}
	if got := h.Summary(); got != "Bash: npm install" {
		t.Errorf("Summary = %s", got)
	}

	// Пустой coucou_agent = Claude Code
	none := Hook{}
	if none.NamedAgent() != "Claude Code" {
		t.Errorf("NamedAgent дефолт = %s", none.NamedAgent())
	}
}
