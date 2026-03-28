package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigPrintHook_OutputIsValidJSON(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"config", "print-hook"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(stdout)
	if !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("expected JSON output, got: %s", trimmed)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("expected valid JSON, got error: %v\noutput: %q", err, trimmed)
	}
}

func TestConfigPrintHook_ContainsHooks(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"config", "print-hook"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, hookName := range []string{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd"} {
		if !strings.Contains(stdout, hookName) {
			t.Errorf("expected output to contain %q, got: %s", hookName, stdout)
		}
	}
}

func TestConfigPrintHook_ContainsCommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"config", "print-hook"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, cmd := range []string{"session-start", "user-prompt", "hook stop", "session-end"} {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("expected output to contain %q, got: %s", cmd, stdout)
		}
	}
}
