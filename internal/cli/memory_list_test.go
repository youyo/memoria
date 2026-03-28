package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryList_EmptyDB(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory list should be implemented, got: %s", stdout)
	}
}

func TestMemoryList_JSONFormat(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"--format", "json", "memory", "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results interface{}
	if jsonErr := json.Unmarshal([]byte(stdout), &results); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
}

func TestMemoryList_WithLimit(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "list", "--limit", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory list should be implemented, got: %s", stdout)
	}
}

func TestMemoryList_WithKind(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "list", "--kind", "decision"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory list should be implemented, got: %s", stdout)
	}
}

func TestMemoryList_WithProject(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "list", "--project", "proj-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory list should be implemented, got: %s", stdout)
	}
}
