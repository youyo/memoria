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

// runHookSessionStart はテスト用に HookSessionStartCmd.RunWithReader を実行するヘルパー。
func runHookSessionStart(t *testing.T, stdinContent string) (string, error) {
	t.Helper()
	sqlDB := testutil.OpenTestDB(t)
	cmd := &cli.HookSessionStartCmd{}
	globals := &cli.Globals{}

	var buf strings.Builder
	err := cmd.RunWithReader(globals, &buf, strings.NewReader(stdinContent), sqlDB, nil)
	return buf.String(), err
}

func TestHookSessionStart_InvalidJSON(t *testing.T) {
	// invalid JSON → exit 0
	_, err := runHookSessionStart(t, `{invalid`)
	if err != nil {
		t.Errorf("Run should return nil for invalid JSON, got: %v", err)
	}
}

func TestHookSessionStart_EmptyStdin(t *testing.T) {
	// 空 stdin → exit 0
	_, err := runHookSessionStart(t, ``)
	if err != nil {
		t.Errorf("Run should return nil for empty stdin, got: %v", err)
	}
}

func TestHookSessionStart_ValidJSON_EmptyDB(t *testing.T) {
	// chunks なし → 空の additionalContext を返す
	input := `{"session_id":"s1","cwd":"/tmp","transcript_path":"","source":"startup"}`
	output, err := runHookSessionStart(t, input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// JSON 出力を検証
	var resp cli.HookOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v\noutput: %q", err, output)
	}
	if resp.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want %q", resp.HookSpecificOutput.HookEventName, "SessionStart")
	}
}

func TestHookSessionStart_OutputFormat(t *testing.T) {
	// 出力が正しい JSON フォーマットであることを確認
	input := `{"session_id":"s1","cwd":"/tmp","source":"startup"}`
	output, err := runHookSessionStart(t, input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Fatal("expected non-empty output")
	}

	if !strings.HasPrefix(trimmed, "{") {
		t.Errorf("expected JSON output starting with '{', got: %s", trimmed)
	}
}

func TestHookSessionStart_WithChunks(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	// Resolver を通じて project を登録し、正しい project_id を取得する
	resolver := project.NewResolver(sqlDB)
	projectID, err := resolver.Resolve(ctx, "/test/project")
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	// chunk を挿入
	_, err = sqlDB.Exec(`
INSERT INTO chunks (chunk_id, project_id, content, summary, kind, importance, scope, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"chunk1", projectID, "Use WAL mode for SQLite performance", "WAL mode", "decision", 0.9, "project", "hash1",
	)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	cmd := &cli.HookSessionStartCmd{}
	globals := &cli.Globals{}
	var buf strings.Builder

	input := `{"session_id":"s1","cwd":"/test/project","source":"startup"}`
	if err := cmd.RunWithReader(globals, &buf, strings.NewReader(input), sqlDB, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	var resp cli.HookOutput
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("expected valid JSON, got: %v\noutput: %q", err, output)
	}
	if resp.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want 'SessionStart'", resp.HookSpecificOutput.HookEventName)
	}
}
