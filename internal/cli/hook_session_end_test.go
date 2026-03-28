package cli_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/cli"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
)

// runHookSessionEnd はテスト用に HookSessionEndCmd.RunWithReader() を実行するヘルパー。
func runHookSessionEnd(t *testing.T, stdinContent string, sqlDB *sql.DB) error {
	t.Helper()
	cmd := &cli.HookSessionEndCmd{}
	globals := &cli.Globals{}
	q := queue.New(sqlDB)

	w := io.Writer(io.Discard)
	return cmd.RunWithReader(globals, &w, strings.NewReader(stdinContent), sqlDB, q)
}

func TestHookSessionEndInput_Valid(t *testing.T) {
	input := `{"session_id":"s1","cwd":"/tmp","transcript_path":"/tmp/abc123.jsonl","reason":"exit"}`
	var got cli.HookSessionEndInput
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "s1")
	}
	if got.Cwd != "/tmp" {
		t.Errorf("Cwd = %q, want %q", got.Cwd, "/tmp")
	}
	if got.TranscriptPath != "/tmp/abc123.jsonl" {
		t.Errorf("TranscriptPath = %q, want %q", got.TranscriptPath, "/tmp/abc123.jsonl")
	}
	if got.Reason != "exit" {
		t.Errorf("Reason = %q, want %q", got.Reason, "exit")
	}
}

func TestHookSessionEndInput_MissingOptionalFields(t *testing.T) {
	// reason は任意フィールド
	input := `{"session_id":"s1","cwd":"/tmp","transcript_path":"/tmp/abc123.jsonl"}`
	var got cli.HookSessionEndInput
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Errorf("Unmarshal should not fail on missing reason: %v", err)
	}
	if got.Reason != "" {
		t.Errorf("Reason = %q, want empty string", got.Reason)
	}
}

func TestSessionEndPayload_JSON(t *testing.T) {
	p := cli.SessionEndPayload{
		SessionID:      "s1",
		ProjectID:      "abcd12341234abcd",
		Cwd:            "/tmp",
		TranscriptPath: "/tmp/abc123.jsonl",
		Reason:         "exit",
		EnqueuedAt:     mustParseTime("2026-03-28T10:00:00Z"),
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{"session_id", "project_id", "cwd", "transcript_path", "reason", "enqueued_at"} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON missing field %q: %s", want, s)
		}
	}
}

func TestHookSessionEnd_EnqueuesJob(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	inputJSON := `{"session_id":"sess1","cwd":"/tmp","transcript_path":"/tmp/abc123.jsonl","reason":"exit"}`
	if err := runHookSessionEnd(t, inputJSON, sqlDB); err != nil {
		t.Fatalf("Run: %v", err)
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

func TestHookSessionEnd_InvalidJSON(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)

	// invalid JSON → Run() は error を返さず exit 0 相当
	if err := runHookSessionEnd(t, `{invalid`, sqlDB); err != nil {
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

func TestHookSessionEnd_EmptyStdin(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)

	// stdin が EOF → 何もせず exit 0
	if err := runHookSessionEnd(t, ``, sqlDB); err != nil {
		t.Errorf("Run should return nil for empty stdin, got: %v", err)
	}
}

func TestHookSessionEnd_EnqueueFailure(t *testing.T) {
	// DB がクローズ済みの場合でも exit 0 継続
	sqlDB := testutil.OpenTestDB(t)
	sqlDB.Close()

	err := runHookSessionEnd(t, `{"session_id":"s1","cwd":"/tmp","transcript_path":"/tmp/abc.jsonl","reason":"exit"}`, sqlDB)
	if err != nil {
		t.Errorf("Run should return nil even on enqueue failure, got: %v", err)
	}
}

func TestHookSessionEnd_Integration(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	inputJSON := `{"session_id":"integ-session","cwd":"/tmp","transcript_path":"/tmp/integ.jsonl","reason":"exit"}`

	if err := runHookSessionEnd(t, inputJSON, sqlDB); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// jobs テーブルを直接 SELECT して確認
	row := sqlDB.QueryRowContext(ctx,
		`SELECT job_type, status, payload_json FROM jobs WHERE job_type = 'session_end_ingest' LIMIT 1`)

	var jobType, status, payloadJSON string
	if err := row.Scan(&jobType, &status, &payloadJSON); err != nil {
		t.Fatalf("query jobs: %v", err)
	}

	if jobType != "session_end_ingest" {
		t.Errorf("job_type = %q, want %q", jobType, "session_end_ingest")
	}
	if status != "queued" {
		t.Errorf("status = %q, want %q", status, "queued")
	}

	// payload_json に必要なフィールドが含まれることを確認
	var payload struct {
		SessionID      string `json:"session_id"`
		ProjectID      string `json:"project_id"`
		TranscriptPath string `json:"transcript_path"`
		Reason         string `json:"reason"`
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
	if payload.TranscriptPath != "/tmp/integ.jsonl" {
		t.Errorf("payload.transcript_path = %q, want %q", payload.TranscriptPath, "/tmp/integ.jsonl")
	}
	if payload.Reason != "exit" {
		t.Errorf("payload.reason = %q, want %q", payload.Reason, "exit")
	}

	// 2回目の実行（UPSERT が正しく動くか確認）
	if err := runHookSessionEnd(t, inputJSON, sqlDB); err != nil {
		t.Errorf("2nd Run: %v", err)
	}

	// projects テーブルに重複がないことを確認
	var count int
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if count != 1 {
		t.Errorf("projects count = %d, want 1 (UPSERT should not duplicate)", count)
	}
}

func TestHookSessionEnd_NoTranscriptRead(t *testing.T) {
	// transcript_path が存在しないファイルでも enqueue は成功する
	// （ファイル読み込みはしないため）
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	inputJSON := `{"session_id":"s1","cwd":"/tmp","transcript_path":"/nonexistent/path/abc.jsonl","reason":"crash"}`
	if err := runHookSessionEnd(t, inputJSON, sqlDB); err != nil {
		t.Fatalf("Run should not fail even if transcript file doesn't exist: %v", err)
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
