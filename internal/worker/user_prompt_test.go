package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"database/sql"

	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
	"github.com/youyo/memoria/internal/worker"
)

// userPromptPayloadTest はテスト用の user_prompt payload 構造体。
type userPromptPayloadTest struct {
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
	Cwd        string    `json:"cwd"`
	Prompt     string    `json:"prompt"`
	CapturedAt time.Time `json:"captured_at"`
}

func makeUserPromptPayload(t *testing.T, sessionID, projectID, prompt string) string {
	t.Helper()
	payload := userPromptPayloadTest{
		SessionID:  sessionID,
		ProjectID:  projectID,
		Cwd:        "/tmp/project",
		Prompt:     prompt,
		CapturedAt: time.Now().UTC(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal user_prompt payload: %v", err)
	}
	return string(b)
}

// TestHandleUserPromptSuccess は正常にプロンプトが保存されることを確認する（U2）。
func TestHandleUserPromptSuccess(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p-up1")
	ctx := context.Background()

	payloadJSON := makeUserPromptPayload(t, "s1", "p-up1", "How do I implement a binary search tree?")
	job := &queue.Job{
		ID:          "job-up-1",
		Type:        queue.JobTypeUserPromptIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewUserPromptHandler(db)
	err := handler.Handle(ctx, job)
	if err != nil {
		t.Fatalf("HandleUserPrompt: %v", err)
	}

	// chunks テーブルに 1 件あることを確認
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count == 0 {
		t.Error("expected at least 1 chunk after user_prompt ingest")
	}
}

// TestHandleUserPromptInvalidJSON は不正 JSON をエラーとして処理することを確認する。
func TestHandleUserPromptInvalidJSON(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	job := &queue.Job{
		ID:          "job-up-invalid",
		Type:        queue.JobTypeUserPromptIngest,
		PayloadJSON: "INVALID JSON",
	}

	handler := worker.NewUserPromptHandler(db)
	err := handler.Handle(ctx, job)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
}

// TestHandleUserPromptDuplicate は同一プロンプトが重複保存されないことを確認する（U3）。
func TestHandleUserPromptDuplicate(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p-up2")
	ctx := context.Background()

	prompt := "duplicate prompt for deduplication test"
	payloadJSON := makeUserPromptPayload(t, "s1", "p-up2", prompt)
	job1 := &queue.Job{ID: "job-up-d1", Type: queue.JobTypeUserPromptIngest, PayloadJSON: payloadJSON}
	job2 := &queue.Job{ID: "job-up-d2", Type: queue.JobTypeUserPromptIngest, PayloadJSON: payloadJSON}

	handler := worker.NewUserPromptHandler(db)

	if err := handler.Handle(ctx, job1); err != nil {
		t.Fatalf("Handle (1st): %v", err)
	}
	if err := handler.Handle(ctx, job2); err != nil {
		t.Fatalf("Handle (2nd, duplicate): %v", err)
	}

	// chunks テーブルに 1 件のみ（SHA-256 重複排除）
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks WHERE content = ?", prompt)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 chunk (dedup), got %d", count)
	}
}

// TestHandleUserPromptEmpty は空プロンプトがスキップされることを確認する。
func TestHandleUserPromptEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p-up3")
	ctx := context.Background()

	payloadJSON := makeUserPromptPayload(t, "s1", "p-up3", "")
	job := &queue.Job{
		ID:          "job-up-empty",
		Type:        queue.JobTypeUserPromptIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewUserPromptHandler(db)
	err := handler.Handle(ctx, job)
	if err != nil {
		t.Fatalf("Handle empty prompt: %v", err)
	}

	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 chunks for empty prompt, got %d", count)
	}
}

// TestHandleUserPromptWithEmbedder は embedder ありで EmbedChunks が呼ばれることを確認する（U4）。
func TestHandleUserPromptWithEmbedder(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p-up4")
	ctx := context.Background()

	mock := &mockEmbedder{}
	handler := worker.NewUserPromptHandlerWithEmbedder(db, mock, "test-model", nil)

	payloadJSON := makeUserPromptPayload(t, "s1", "p-up4", "How do I use Go generics?")
	job := &queue.Job{
		ID:          "job-up-emb-1",
		Type:        queue.JobTypeUserPromptIngest,
		PayloadJSON: payloadJSON,
	}

	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// EmbedChunks が呼ばれたこと
	if !mock.called {
		t.Error("expected EmbedChunks to be called")
	}
	if len(mock.lastChunkIDs) == 0 {
		t.Error("expected at least 1 chunkID passed to EmbedChunks")
	}
}

// TestHandleUserPromptWithEmbedderError は embedding エラーでも ingest が成功することを確認する。
func TestHandleUserPromptWithEmbedderError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p-up5")
	ctx := context.Background()

	mock := &mockEmbedder{
		embedChunksFn: func(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error {
			return errors.New("embedding worker not available")
		},
	}
	handler := worker.NewUserPromptHandlerWithEmbedder(db, mock, "test-model", nil)

	payloadJSON := makeUserPromptPayload(t, "s1", "p-up5", "prompt with embedding error")
	job := &queue.Job{
		ID:          "job-up-emb-err-1",
		Type:        queue.JobTypeUserPromptIngest,
		PayloadJSON: payloadJSON,
	}

	// embedding エラーがあっても Handle は nil を返す（non-fatal）
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle should not return error on embedding failure: %v", err)
	}

	// chunks テーブルには保存されていること
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count == 0 {
		t.Error("expected chunk to be saved even when embedding fails")
	}
}
