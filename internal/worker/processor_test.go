package worker_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
	"github.com/youyo/memoria/internal/worker"
)

// mockProcessor はテスト用のモック JobProcessor。
type mockProcessor struct {
	handleCheckpointFn func(ctx context.Context, job *queue.Job) error
	handleSessionEndFn func(ctx context.Context, job *queue.Job) error
}

func (m *mockProcessor) HandleCheckpoint(ctx context.Context, job *queue.Job) error {
	if m.handleCheckpointFn != nil {
		return m.handleCheckpointFn(ctx, job)
	}
	return nil
}

func (m *mockProcessor) HandleSessionEnd(ctx context.Context, job *queue.Job) error {
	if m.handleSessionEndFn != nil {
		return m.handleSessionEndFn(ctx, job)
	}
	return nil
}

func (m *mockProcessor) HandleProjectRefresh(ctx context.Context, job *queue.Job) error {
	return nil
}

func (m *mockProcessor) HandleProjectSimilarityRefresh(ctx context.Context, job *queue.Job) error {
	return nil
}

func newTestDaemonWithProcessor(t *testing.T, db *sql.DB, processor worker.JobProcessor) (*worker.IngestDaemon, *queue.Queue) {
	t.Helper()
	q := queue.New(db)
	runDir := t.TempDir()
	logDir := t.TempDir()
	d := worker.NewIngestDaemon(db, q, runDir, logDir, 100*time.Millisecond)
	d.SetLogf(func(string, ...any) {})
	d.SetProcessor(processor)
	return d, q
}

func TestProcessJobCheckpoint(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")

	handled := false
	mock := &mockProcessor{
		handleCheckpointFn: func(ctx context.Context, job *queue.Job) error {
			handled = true
			return nil
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// checkpoint_ingest ジョブを enqueue
	payload := makeCheckpointPayload(t, "s1", "p1", "test content")
	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// daemon を起動（idle timeout で自動終了）
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !handled {
		t.Error("expected HandleCheckpoint to be called")
	}

	// ジョブが done になっていること
	var status string
	row := db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status=done, got %s", status)
	}
}

func TestProcessJobSessionEnd(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")

	handled := false
	mock := &mockProcessor{
		handleSessionEndFn: func(ctx context.Context, job *queue.Job) error {
			handled = true
			return nil
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// transcript ファイルを作成
	f, err := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	f.Close()

	payload := makeSessionEndPayload(t, "s1", "p1", f.Name())
	jobID, err := q.Enqueue(ctx, queue.JobTypeSessionEndIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !handled {
		t.Error("expected HandleSessionEnd to be called")
	}

	var status string
	row := db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status=done, got %s", status)
	}
}

func TestProcessJobProjectRefresh(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	handled := false
	mock := &mockProcessor{
		handleCheckpointFn: nil,
		handleSessionEndFn: nil,
	}
	// HandleProjectRefresh をオーバーライドするためにカスタムモックを使用
	type extMock struct {
		*mockProcessor
	}
	// mockProcessor の HandleProjectRefresh は nil 返し（default 実装）
	_ = handled

	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := queue.New(db)
	// project_refresh ジョブを enqueue
	jobID, err := q.Enqueue(ctx, queue.JobTypeProjectRefresh, `{"project_id":"test-id","project_root":"/tmp"}`)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	runDir := t.TempDir()
	logDir := t.TempDir()
	d := worker.NewIngestDaemon(db, q, runDir, logDir, 100*time.Millisecond)
	d.SetLogf(func(string, ...any) {})
	d.SetProcessor(mock)

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// ジョブが done になっていること（mock は nil を返す）
	var status string
	row := db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status=done for project_refresh job, got %s", status)
	}
}

func TestProcessJobFailure(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")

	mock := &mockProcessor{
		handleCheckpointFn: func(ctx context.Context, job *queue.Job) error {
			return errors.New("simulated handler error")
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := makeCheckpointPayload(t, "s1", "p1", "test content")
	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = q

	// リトライ可能（retry_count < max_retries）→ queued になるはず
	var status string
	var retryCount int
	row := db.QueryRowContext(ctx, "SELECT status, retry_count FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status, &retryCount); err != nil {
		t.Fatalf("query job: %v", err)
	}
	// 1回失敗したので queued (retry) か failed
	if status != "queued" && status != "failed" {
		t.Errorf("expected queued or failed after failure, got %s", status)
	}
	if retryCount < 1 {
		t.Errorf("expected retry_count >= 1, got %d", retryCount)
	}
}

func TestProcessJobLeaseUpdated(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")

	jobIDChan := make(chan string, 1)
	mock := &mockProcessor{
		handleCheckpointFn: func(ctx context.Context, job *queue.Job) error {
			// processJob 処理中に current_job_id が設定されているはず
			jobIDChan <- job.ID
			return nil
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := makeCheckpointPayload(t, "s1", "p1", "test content")
	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case <-jobIDChan:
		// handler は呼ばれた
	default:
		t.Error("expected HandleCheckpoint to be called")
	}

	// 処理完了後、current_job_id は NULL になっているはず
	lease, err := worker.GetLease(ctx, db, worker.WorkerNameIngest)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if lease != nil && lease.CurrentJobID != nil {
		t.Errorf("expected current_job_id to be NULL after processing, got %s", *lease.CurrentJobID)
	}
}

// TestProcessJobWithRealHandlers は実際のハンドラを使ったエンドツーエンドのテスト。
func TestProcessJobWithRealHandlers(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")
	ctx := context.Background()

	runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 実際のプロセッサを使用
	d, q := newTestDaemonWithProcessor(t, db, worker.NewDefaultJobProcessor(db))

	// checkpoint_ingest ジョブを enqueue
	payload := makeCheckpointPayload(t, "s1", "p1", "decided to use Go for the backend")
	jobID, err := q.Enqueue(runCtx, queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// ジョブが done
	var status string
	row := db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status=done, got %s", status)
	}

	// chunks に 1 件以上
	var chunkCount int
	row = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks")
	if err := row.Scan(&chunkCount); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if chunkCount == 0 {
		t.Error("expected at least 1 chunk after processing")
	}
}

// FMT import 参照
var _ = fmt.Sprintf
var _ = ingest.ContentHash
