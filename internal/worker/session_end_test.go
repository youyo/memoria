package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
	"github.com/youyo/memoria/internal/worker"
)

// sessionEndPayloadTest はテスト用の session_end payload 構造体。
type sessionEndPayloadTest struct {
	SessionID      string    `json:"session_id"`
	ProjectID      string    `json:"project_id"`
	Cwd            string    `json:"cwd"`
	TranscriptPath string    `json:"transcript_path"`
	Reason         string    `json:"reason"`
	EnqueuedAt     time.Time `json:"enqueued_at"`
}

func makeSessionEndPayload(t *testing.T, sessionID, projectID, transcriptPath string) string {
	t.Helper()
	payload := sessionEndPayloadTest{
		SessionID:      sessionID,
		ProjectID:      projectID,
		Cwd:            "/tmp/project",
		TranscriptPath: transcriptPath,
		Reason:         "exit",
		EnqueuedAt:     time.Now().UTC(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal session_end payload: %v", err)
	}
	return string(b)
}

func writeTempTranscriptW(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return f.Name()
}

func makeTranscriptJSONL(pairs int) string {
	var lines string
	for i := 0; i < pairs; i++ {
		lines += fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"question %d"},"timestamp":"2026-03-28T12:00:00.000Z","uuid":"u%d"}`, i, i) + "\n"
		lines += fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":"answer %d"},"timestamp":"2026-03-28T12:00:01.000Z","uuid":"a%d"}`, i, i) + "\n"
	}
	return lines
}

func TestHandleSessionEndSuccess(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	transcriptPath := writeTempTranscriptW(t, makeTranscriptJSONL(3))
	payloadJSON := makeSessionEndPayload(t, "s1", "p1", transcriptPath)
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeSessionEndIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewSessionEndHandler(db)
	err := handler.Handle(ctx, job)
	if err != nil {
		t.Fatalf("HandleSessionEnd: %v", err)
	}

	// turns テーブルに 6 件（3 ペア × 2）
	var turnCount int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM turns WHERE session_id = 's1'")
	if err := row.Scan(&turnCount); err != nil {
		t.Fatalf("count turns: %v", err)
	}
	if turnCount != 6 {
		t.Errorf("expected 6 turns, got %d", turnCount)
	}

	// chunks テーブルに 3 件（3 ペア）
	var chunkCount int
	row = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks WHERE session_id = 's1'")
	if err := row.Scan(&chunkCount); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if chunkCount != 3 {
		t.Errorf("expected 3 chunks, got %d", chunkCount)
	}
}

func TestHandleSessionEndAlreadyIngested(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	transcriptPath := writeTempTranscriptW(t, makeTranscriptJSONL(2))
	payloadJSON := makeSessionEndPayload(t, "s1", "p1", transcriptPath)

	handler := worker.NewSessionEndHandler(db)

	// 1回目: 処理
	job1 := &queue.Job{ID: "job1", Type: queue.JobTypeSessionEndIngest, PayloadJSON: payloadJSON}
	if err := handler.Handle(ctx, job1); err != nil {
		t.Fatalf("HandleSessionEnd (1st): %v", err)
	}

	// 2回目: 既に ingested → スキップ（エラーなし）
	job2 := &queue.Job{ID: "job2", Type: queue.JobTypeSessionEndIngest, PayloadJSON: payloadJSON}
	if err := handler.Handle(ctx, job2); err != nil {
		t.Fatalf("HandleSessionEnd (2nd, already ingested): %v", err)
	}

	// turns は 4 件のまま（重複なし）
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM turns WHERE session_id = 's1'")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count turns: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4 turns (no duplicate), got %d", count)
	}
}

func TestHandleSessionEndNoTranscript(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	payloadJSON := makeSessionEndPayload(t, "s1", "p1", "/nonexistent/transcript.jsonl")
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeSessionEndIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewSessionEndHandler(db)
	err := handler.Handle(ctx, job)
	// transcript が存在しない → エラー（retry）
	if err == nil {
		t.Fatal("expected error when transcript does not exist")
	}
	if err != ingest.ErrTranscriptNotFound {
		t.Errorf("expected ErrTranscriptNotFound, got %v", err)
	}
}

func TestHandleSessionEndEmptyTranscript(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	// 空のトランスクリプト
	transcriptPath := writeTempTranscriptW(t, "")
	payloadJSON := makeSessionEndPayload(t, "s1", "p1", transcriptPath)
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeSessionEndIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewSessionEndHandler(db)
	err := handler.Handle(ctx, job)
	// 空 transcript は Ack（エラーなし）
	if err != nil {
		t.Fatalf("HandleSessionEnd empty transcript: %v", err)
	}

	// chunks は 0 件
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks WHERE session_id = 's1'")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 chunks for empty transcript, got %d", count)
	}
}

func TestHandleSessionEndInvalidJSON(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeSessionEndIngest,
		PayloadJSON: "INVALID JSON",
	}

	handler := worker.NewSessionEndHandler(db)
	err := handler.Handle(ctx, job)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
}

func TestHandleSessionEndSessionRecord(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	transcriptPath := writeTempTranscriptW(t, makeTranscriptJSONL(1))
	payloadJSON := makeSessionEndPayload(t, "s-end-test", "p1", transcriptPath)
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeSessionEndIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewSessionEndHandler(db)
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("HandleSessionEnd: %v", err)
	}

	// sessions テーブルに transcript_path が設定されていること
	var transcriptPathDB *string
	row := db.QueryRowContext(ctx, "SELECT transcript_path FROM sessions WHERE session_id = 's-end-test'")
	if err := row.Scan(&transcriptPathDB); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if transcriptPathDB == nil || *transcriptPathDB != transcriptPath {
		t.Errorf("expected transcript_path=%s, got %v", transcriptPath, transcriptPathDB)
	}
}
