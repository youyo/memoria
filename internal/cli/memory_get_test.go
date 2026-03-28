package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryGet_NotFound(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "get", "nonexistent-id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// not found の場合は "not found" を含む出力
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory get should be implemented, got: %s", stdout)
	}
}

func TestMemoryGet_JSONFormat_NotFound(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"--format", "json", "memory", "get", "nonexistent-id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
}
