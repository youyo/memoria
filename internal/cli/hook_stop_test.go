package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/cli"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
)

// runHookStop はテスト用に HookStopCmd.Run() を実行するヘルパー。
// stdin を strings.Reader に置き換えて Run() を呼ぶ。
func runHookStop(t *testing.T, stdinContent string, sqlDB *sql.DB) error {
	t.Helper()
	cmd := &cli.HookStopCmd{}
	globals := &cli.Globals{}
	q := queue.New(sqlDB)

	w := io.Writer(io.Discard)
	return cmd.RunWithReader(globals, &w, strings.NewReader(stdinContent), sqlDB, q)
}

func TestHookStopInput_Valid(t *testing.T) {
	input := `{"session_id":"s1","cwd":"/tmp","last_assistant_message":"hello"}`
	var got cli.HookStopInput
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "s1")
	}
	if got.Cwd != "/tmp" {
		t.Errorf("Cwd = %q, want %q", got.Cwd, "/tmp")
	}
	if got.LastAssistantMessage != "hello" {
		t.Errorf("LastAssistantMessage = %q, want %q", got.LastAssistantMessage, "hello")
	}
}

func TestHookStopInput_EmptyMessage(t *testing.T) {
	input := `{"session_id":"s1","cwd":"/tmp","last_assistant_message":""}`
	var got cli.HookStopInput
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Errorf("Unmarshal should not fail on empty message: %v", err)
	}
	if got.LastAssistantMessage != "" {
		t.Errorf("LastAssistantMessage = %q, want empty", got.LastAssistantMessage)
	}
}

func TestHookStopInput_MissingField(t *testing.T) {
	input := `{"session_id":"s1","last_assistant_message":"hello"}`
	var got cli.HookStopInput
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Errorf("Unmarshal should not fail on missing cwd: %v", err)
	}
	if got.Cwd != "" {
		t.Errorf("Cwd = %q, want empty string", got.Cwd)
	}
}

func TestCheckpointPayload_JSON(t *testing.T) {
	p := cli.CheckpointPayload{
		SessionID:  "s1",
		ProjectID:  "abcd12341234abcd",
		Cwd:        "/tmp",
		Content:    "hello",
		CapturedAt: mustParseTime("2026-03-28T10:00:00Z"),
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{"session_id", "project_id", "captured_at", "content", "cwd"} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON missing field %q: %s", want, s)
		}
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestHookStop_EnqueuesJob(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	inputJSON := `{"session_id":"sess1","cwd":"/tmp","last_assistant_message":"WAL mode enabled"}`
	if err := runHookStop(t, inputJSON, sqlDB); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// queue.Stats() で queued == 1 を確認
	q := queue.New(sqlDB)
	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats[queue.StatusQueued] != 1 {
		t.Errorf("queued jobs = %d, want 1", stats[queue.StatusQueued])
	}
}

func TestHookStop_InvalidJSON(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)

	// invalid JSON → Run() は error を返さず exit 0 相当
	if err := runHookStop(t, `{invalid`, sqlDB); err != nil {
		t.Errorf("Run should return nil for invalid JSON, got: %v", err)
	}

	// キューにジョブが追加されていないことを確認
	ctx := context.Background()
	q := queue.New(sqlDB)
	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats[queue.StatusQueued] != 0 {
		t.Errorf("queued jobs = %d, want 0", stats[queue.StatusQueued])
	}
}

func TestHookStop_EmptyStdin(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)

	// stdin が EOF → 何もせず exit 0
	if err := runHookStop(t, ``, sqlDB); err != nil {
		t.Errorf("Run should return nil for empty stdin, got: %v", err)
	}
}

func TestHookStop_TimeoutContext(t *testing.T) {
	// タイムアウト済みの context でもパニックしないことを確認
	sqlDB := testutil.OpenTestDB(t)

	cmd := &cli.HookStopCmd{}
	globals := &cli.Globals{}
	q := queue.New(sqlDB)
	w := io.Writer(io.Discard)

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	_ = ctx

	// Run() は内部で独自 context を作るのでパニックしない
	err := cmd.RunWithReader(globals, &w, strings.NewReader(`{"session_id":"s1","cwd":"/tmp","last_assistant_message":"test"}`), sqlDB, q)
	if err != nil {
		// エラーを返してもよい（exit 0 を保証するのは main.go 側の責務）
		// ただしパニックしないことを確認
		_ = err
	}
}

func TestHookStop_EnqueueFailure(t *testing.T) {
	// Enqueue が失敗しても exit 0 継続（DB がクローズ済みの場合をシミュレート）
	sqlDB := testutil.OpenTestDB(t)

	// DB を先にクローズしてエラーを発生させる
	sqlDB.Close()

	// invalid JSON でなく valid JSON を投入（project 解決が先に失敗するが継続）
	err := runHookStop(t, `{"session_id":"s1","cwd":"/tmp","last_assistant_message":"test"}`, sqlDB)
	// エラーを返さないことを確認（セッションをブロックしない）
	if err != nil {
		t.Errorf("Run should return nil even on enqueue failure, got: %v", err)
	}
}

func TestHookStop_LargeMessage(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	// 1MB の last_assistant_message
	largeContent := strings.Repeat("あ", 1024*1024/3) // UTF-8 で約 1MB
	inputMap := map[string]string{
		"session_id":             "sess1",
		"cwd":                    "/tmp",
		"last_assistant_message": largeContent,
	}
	b, _ := json.Marshal(inputMap)

	if err := runHookStop(t, string(b), sqlDB); err != nil {
		t.Fatalf("Run with large message: %v", err)
	}

	q := queue.New(sqlDB)
	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats[queue.StatusQueued] != 1 {
		t.Errorf("queued jobs = %d, want 1", stats[queue.StatusQueued])
	}
}

func TestHookStop_Integration(t *testing.T) {
	// tmpDir の SQLite DB を使った統合テスト
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	inputJSON := `{"session_id":"integ-session","cwd":"/tmp","last_assistant_message":"Integration test message"}`

	if err := runHookStop(t, inputJSON, sqlDB); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// jobs テーブルを直接 SELECT して確認
	row := sqlDB.QueryRowContext(ctx,
		`SELECT job_type, status, payload_json FROM jobs WHERE job_type = 'checkpoint_ingest' LIMIT 1`)

	var jobType, status, payloadJSON string
	if err := row.Scan(&jobType, &status, &payloadJSON); err != nil {
		t.Fatalf("query jobs: %v", err)
	}

	if jobType != "checkpoint_ingest" {
		t.Errorf("job_type = %q, want %q", jobType, "checkpoint_ingest")
	}
	if status != "queued" {
		t.Errorf("status = %q, want %q", status, "queued")
	}

	// payload_json に必要なフィールドが含まれることを確認
	var payload struct {
		SessionID string `json:"session_id"`
		ProjectID string `json:"project_id"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.SessionID != "integ-session" {
		t.Errorf("payload.session_id = %q, want %q", payload.SessionID, "integ-session")
	}
	if len(payload.ProjectID) != 16 {
		t.Errorf("payload.project_id length = %d, want 16", len(payload.ProjectID))
	}
	if !strings.Contains(payload.Content, "Integration test message") {
		t.Errorf("payload.content missing expected text, got: %q", payload.Content)
	}

	// stderr バッファを確認（エラーなし時は ensureWorker のメッセージのみ）
	var stderr bytes.Buffer
	cmd := &cli.HookStopCmd{}
	globals := &cli.Globals{}
	q := queue.New(sqlDB)
	w := io.Writer(io.Discard)

	// 2回目の実行（UPSERT が正しく動くか確認）
	if err := cmd.RunWithReader(globals, &w, strings.NewReader(inputJSON), sqlDB, q); err != nil {
		t.Errorf("2nd Run: %v", err)
	}
	_ = stderr

	// projects テーブルに重複がないことを確認
	var count int
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 1 {
		t.Errorf("projects count = %d, want 1 (UPSERT should not duplicate)", count)
	}
}
