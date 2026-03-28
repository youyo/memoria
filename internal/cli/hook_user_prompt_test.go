package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/cli"
	"github.com/youyo/memoria/internal/project"
	"github.com/youyo/memoria/internal/testutil"
)

// runHookUserPrompt はテスト用に HookUserPromptCmd.RunWithReader を実行するヘルパー。
func runHookUserPrompt(t *testing.T, stdinContent string) (string, error) {
	t.Helper()
	sqlDB := testutil.OpenTestDB(t)
	cmd := &cli.HookUserPromptCmd{}
	globals := &cli.Globals{}

	var buf strings.Builder
	err := cmd.RunWithReader(globals, &buf, strings.NewReader(stdinContent), sqlDB, nil)
	return buf.String(), err
}

func TestHookUserPrompt_InvalidJSON(t *testing.T) {
	_, err := runHookUserPrompt(t, `{invalid`)
	if err != nil {
		t.Errorf("Run should return nil for invalid JSON, got: %v", err)
	}
}

func TestHookUserPrompt_EmptyStdin(t *testing.T) {
	_, err := runHookUserPrompt(t, ``)
	if err != nil {
		t.Errorf("Run should return nil for empty stdin, got: %v", err)
	}
}

func TestHookUserPrompt_ValidJSON_EmptyDB(t *testing.T) {
	input := `{"session_id":"s1","cwd":"/tmp","prompt":"how do I use SQLite WAL mode?"}`
	output, err := runHookUserPrompt(t, input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var resp cli.HookOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput: %q", err, output)
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName = %q, want %q", resp.HookSpecificOutput.HookEventName, "UserPromptSubmit")
	}
}

func TestHookUserPrompt_EmptyPrompt(t *testing.T) {
	input := `{"session_id":"s1","cwd":"/tmp","prompt":""}`
	output, err := runHookUserPrompt(t, input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var resp cli.HookOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput: %q", err, output)
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName = %q, want 'UserPromptSubmit'", resp.HookSpecificOutput.HookEventName)
	}
}

func TestHookUserPrompt_WithMatchingChunk(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	// Resolver を通じて project を登録し、正しい project_id を取得する
	resolver := project.NewResolver(sqlDB)
	projectID, err := resolver.Resolve(ctx, "/test/project")
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	_, err = sqlDB.Exec(`
INSERT INTO chunks (chunk_id, project_id, content, summary, kind, importance, scope, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"chunk1", projectID, "Use context.WithTimeout for database queries to avoid blocking", "context timeout", "pattern", 0.8, "project", "hash1",
	)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	cmd := &cli.HookUserPromptCmd{}
	globals := &cli.Globals{}
	var buf strings.Builder

	input := `{"session_id":"s1","cwd":"/test/project","prompt":"how to handle database timeout"}`
	if err := cmd.RunWithReader(globals, &buf, strings.NewReader(input), sqlDB, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	var resp cli.HookOutput
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("expected valid JSON, got: %v\noutput: %q", err, output)
	}
	if resp.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName = %q, want 'UserPromptSubmit'", resp.HookSpecificOutput.HookEventName)
	}
}
