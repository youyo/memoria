package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
	"github.com/youyo/memoria/internal/worker"
)

// mockEmbedder は ingest.Embedder のテスト用モック。
type mockEmbedder struct {
	embedChunksFn func(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error
	called        bool
	lastChunkIDs  []string
}

func (m *mockEmbedder) EmbedChunks(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error {
	m.called = true
	m.lastChunkIDs = chunkIDs
	if m.embedChunksFn != nil {
		return m.embedChunksFn(ctx, db, chunkIDs, modelName)
	}
	return nil
}

// checkpointPayloadTest はテスト用の checkpoint payload 構造体。
type checkpointPayloadTest struct {
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
	Cwd        string    `json:"cwd"`
	Content    string    `json:"content"`
	CapturedAt time.Time `json:"captured_at"`
}

// insertTestProject はテスト用プロジェクトを projects テーブルに挿入する。
func insertTestProject(t *testing.T, db *sql.DB, projectID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO projects (project_id, project_root, last_seen_at, created_at) VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		projectID, "/tmp/"+projectID,
	)
	if err != nil {
		t.Fatalf("insert test project: %v", err)
	}
}

func makeCheckpointPayload(t *testing.T, sessionID, projectID, content string) string {
	t.Helper()
	payload := checkpointPayloadTest{
		SessionID:  sessionID,
		ProjectID:  projectID,
		Cwd:        "/tmp/project",
		Content:    content,
		CapturedAt: time.Now().UTC(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal checkpoint payload: %v", err)
	}
	return string(b)
}

func TestHandleCheckpointSuccess(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	payloadJSON := makeCheckpointPayload(t, "s1", "p1", "We decided to use Go for the backend")
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeCheckpointIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewCheckpointHandler(db)
	err := handler.Handle(ctx, job)
	if err != nil {
		t.Fatalf("HandleCheckpoint: %v", err)
	}

	// chunks テーブルに 1 件以上あることを確認
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count == 0 {
		t.Error("expected at least 1 chunk after checkpoint ingest")
	}
}

func TestHandleCheckpointInvalidJSON(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeCheckpointIngest,
		PayloadJSON: "INVALID JSON",
	}

	handler := worker.NewCheckpointHandler(db)
	err := handler.Handle(ctx, job)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
}

func TestHandleCheckpointDuplicate(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	content := "duplicate content for deduplication test"
	payloadJSON := makeCheckpointPayload(t, "s1", "p1", content)
	job1 := &queue.Job{ID: "job1", Type: queue.JobTypeCheckpointIngest, PayloadJSON: payloadJSON}
	job2 := &queue.Job{ID: "job2", Type: queue.JobTypeCheckpointIngest, PayloadJSON: payloadJSON}

	handler := worker.NewCheckpointHandler(db)

	if err := handler.Handle(ctx, job1); err != nil {
		t.Fatalf("HandleCheckpoint (1st): %v", err)
	}
	// 2回目は ON CONFLICT DO NOTHING → エラーなし
	if err := handler.Handle(ctx, job2); err != nil {
		t.Fatalf("HandleCheckpoint (2nd, duplicate): %v", err)
	}

	// chunks テーブルに 1 件のみ
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks WHERE content = ?", content)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 chunk (dedup), got %d", count)
	}
}

func TestHandleCheckpointEmptyContent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	payloadJSON := makeCheckpointPayload(t, "s1", "p1", "")
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeCheckpointIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewCheckpointHandler(db)
	// 空コンテンツはスキップ（エラーなし）
	err := handler.Handle(ctx, job)
	if err != nil {
		t.Fatalf("HandleCheckpoint empty content: %v", err)
	}

	// chunks テーブルに何も入っていない
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 chunks for empty content, got %d", count)
	}
}

func TestHandleCheckpointSessionCreated(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	payloadJSON := makeCheckpointPayload(t, "s-session-test", "p1", "test content")
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeCheckpointIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewCheckpointHandler(db)
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("HandleCheckpoint: %v", err)
	}

	// sessions テーブルに s-session-test があること
	var sessionID string
	row := db.QueryRowContext(ctx, "SELECT session_id FROM sessions WHERE session_id = ?", "s-session-test")
	if err := row.Scan(&sessionID); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if sessionID != "s-session-test" {
		t.Errorf("expected session_id=s-session-test, got %s", sessionID)
	}
}

func TestHandleCheckpointWithEmbedder(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	mock := &mockEmbedder{}
	handler := worker.NewCheckpointHandlerWithEmbedder(db, mock, "test-model", nil)

	payloadJSON := makeCheckpointPayload(t, "s1", "p1", "We decided to use Go for the backend")
	job := &queue.Job{
		ID:          "job-emb-1",
		Type:        queue.JobTypeCheckpointIngest,
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

func TestHandleCheckpointWithEmbedderError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	// embedding エラー → ingest は成功（エラーを返さない）
	mock := &mockEmbedder{
		embedChunksFn: func(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error {
			return errors.New("embedding worker not available")
		},
	}
	handler := worker.NewCheckpointHandlerWithEmbedder(db, mock, "test-model", nil)

	payloadJSON := makeCheckpointPayload(t, "s1", "p1", "content to ingest")
	job := &queue.Job{
		ID:          "job-emb-err-1",
		Type:        queue.JobTypeCheckpointIngest,
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

func TestHandleCheckpointWithoutEmbedder(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	// embedder なし（デフォルトコンストラクタ）
	handler := worker.NewCheckpointHandler(db)

	payloadJSON := makeCheckpointPayload(t, "s1", "p1", "content without embedding")
	job := &queue.Job{
		ID:          "job-no-emb-1",
		Type:        queue.JobTypeCheckpointIngest,
		PayloadJSON: payloadJSON,
	}

	// embedder なしでも正常動作
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle without embedder: %v", err)
	}

	// chunks に保存されていること
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count == 0 {
		t.Error("expected chunk to be saved without embedder")
	}

	// chunk_embeddings には何もないこと
	var embCount int
	row = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunk_embeddings")
	if err := row.Scan(&embCount); err != nil {
		t.Fatalf("count chunk_embeddings: %v", err)
	}
	if embCount != 0 {
		t.Errorf("expected 0 chunk_embeddings without embedder, got %d", embCount)
	}
}

// TestHandleCheckpointKindEnrichment は checkpoint の種別が enrichment で決まることを確認する。
func TestHandleCheckpointKindEnrichment(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	payloadJSON := makeCheckpointPayload(t, "s1", "p1", "We decided to use SQLite for local storage")
	job := &queue.Job{
		ID:          "job1",
		Type:        queue.JobTypeCheckpointIngest,
		PayloadJSON: payloadJSON,
	}

	handler := worker.NewCheckpointHandler(db)
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("HandleCheckpoint: %v", err)
	}

	var kind string
	row := db.QueryRowContext(ctx, "SELECT kind FROM chunks LIMIT 1")
	if err := row.Scan(&kind); err != nil {
		t.Fatalf("query kind: %v", err)
	}
	if kind != "decision" {
		t.Errorf("expected kind=decision, got %s", kind)
	}

	// ingest パッケージが正常にリンクされることを確認
	_ = ingest.ContentHash("test")
}
