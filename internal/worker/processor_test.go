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

func (m *mockProcessor) HandleUserPrompt(ctx context.Context, job *queue.Job) error {
	return nil
}

func newTestDaemonWithProcessor(t *testing.T, db *sql.DB, processor worker.JobProcessor) (*worker.IngestDaemon, *queue.Queue) {
	t.Helper()
	q := queue.New(db)
	runDir := t.TempDir()
	logDir := t.TempDir()
	d := worker.NewIngestDaemon(db, q, runDir, logDir)
	d.SetLogf(func(string, ...any) {})
	d.SetProcessor(processor)
	return d, q
}

func TestProcessJobCheckpoint(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")

	handled := false
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := &mockProcessor{
		handleCheckpointFn: func(ctx context.Context, job *queue.Job) error {
			handled = true
			// ハンドラ完了後、Ack 処理が終わってから daemon を停止させる（遅延 cancel）
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			return nil
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	// checkpoint_ingest ジョブを enqueue
	payload := makeCheckpointPayload(t, "s1", "p1", "test content")
	jobID, err := q.Enqueue(context.Background(), queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// daemon を起動（ジョブ処理後 cancel() で終了）
	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !handled {
		t.Error("expected HandleCheckpoint to be called")
	}

	// ジョブが done になっていること（query は新しい context で）
	queryCtx := context.Background()
	var status string
	row := db.QueryRowContext(queryCtx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
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
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := &mockProcessor{
		handleSessionEndFn: func(ctx context.Context, job *queue.Job) error {
			handled = true
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			return nil
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	// transcript ファイルを作成
	f, err := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	f.Close()

	payload := makeSessionEndPayload(t, "s1", "p1", f.Name())
	jobID, err := q.Enqueue(context.Background(), queue.JobTypeSessionEndIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !handled {
		t.Error("expected HandleSessionEnd to be called")
	}

	queryCtx := context.Background()
	var status string
	row := db.QueryRowContext(queryCtx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status=done, got %s", status)
	}
}

func TestProcessJobProjectRefresh(t *testing.T) {
	db := testutil.OpenTestDB(t)

	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := &mockProcessor{
		handleCheckpointFn: nil,
		handleSessionEndFn: nil,
	}
	// HandleProjectRefresh は nil を返す（mockProcessor.HandleProjectRefresh は default 実装）
	// ジョブ処理後に cancel を呼ぶためラッパーを使う
	inner := worker.NewDefaultJobProcessor(db)
	proc := &cancelOnHandleProcessor{inner: inner, cancel: cancel}

	q := queue.New(db)
	// project_refresh ジョブを enqueue
	jobID, err := q.Enqueue(context.Background(), queue.JobTypeProjectRefresh, `{"project_id":"test-id","project_root":"/tmp"}`)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	runDir := t.TempDir()
	logDir := t.TempDir()
	d := worker.NewIngestDaemon(db, q, runDir, logDir)
	d.SetLogf(func(string, ...any) {})
	d.SetProcessor(proc)

	_ = mock // 変数参照（使用されないwarning回避）

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	queryCtx := context.Background()
	// ジョブが done になっていること
	var status string
	row := db.QueryRowContext(queryCtx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
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

	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := &mockProcessor{
		handleCheckpointFn: func(ctx context.Context, job *queue.Job) error {
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			return errors.New("simulated handler error")
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	payload := makeCheckpointPayload(t, "s1", "p1", "test content")
	jobID, err := q.Enqueue(context.Background(), queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	_ = q

	queryCtx := context.Background()
	// リトライ可能（retry_count < max_retries）→ queued になるはず
	var status string
	var retryCount int
	row := db.QueryRowContext(queryCtx, "SELECT status, retry_count FROM jobs WHERE job_id = ?", jobID)
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
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mock := &mockProcessor{
		handleCheckpointFn: func(ctx context.Context, job *queue.Job) error {
			// processJob 処理中に current_job_id が設定されているはず
			jobIDChan <- job.ID
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()
			return nil
		},
	}

	d, q := newTestDaemonWithProcessor(t, db, mock)

	payload := makeCheckpointPayload(t, "s1", "p1", "test content")
	_, err := q.Enqueue(context.Background(), queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case <-jobIDChan:
		// handler は呼ばれた
	default:
		t.Error("expected HandleCheckpoint to be called")
	}

	queryCtx := context.Background()
	// 処理完了後、current_job_id は NULL になっているはず
	lease, err := worker.GetLease(queryCtx, db, worker.WorkerNameIngest)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if lease != nil && lease.CurrentJobID != nil {
		t.Errorf("expected current_job_id to be NULL after processing, got %s", *lease.CurrentJobID)
	}
}

// cancelOnHandleProcessor は実際のプロセッサをラップし、ジョブ処理後に cancel を呼ぶ。
// idle timeout が廃止されたテストでの daemon 停止用。
// Ack 処理が完了するよう遅延 cancel を使用する。
type cancelOnHandleProcessor struct {
	inner  worker.JobProcessor
	cancel context.CancelFunc
}

func (p *cancelOnHandleProcessor) delayedCancel() {
	go func() {
		time.Sleep(50 * time.Millisecond)
		p.cancel()
	}()
}

func (p *cancelOnHandleProcessor) HandleCheckpoint(ctx context.Context, job *queue.Job) error {
	err := p.inner.HandleCheckpoint(ctx, job)
	p.delayedCancel()
	return err
}

func (p *cancelOnHandleProcessor) HandleSessionEnd(ctx context.Context, job *queue.Job) error {
	err := p.inner.HandleSessionEnd(ctx, job)
	p.delayedCancel()
	return err
}

func (p *cancelOnHandleProcessor) HandleProjectRefresh(ctx context.Context, job *queue.Job) error {
	err := p.inner.HandleProjectRefresh(ctx, job)
	p.delayedCancel()
	return err
}

func (p *cancelOnHandleProcessor) HandleProjectSimilarityRefresh(ctx context.Context, job *queue.Job) error {
	err := p.inner.HandleProjectSimilarityRefresh(ctx, job)
	p.delayedCancel()
	return err
}

func (p *cancelOnHandleProcessor) HandleUserPrompt(ctx context.Context, job *queue.Job) error {
	err := p.inner.HandleUserPrompt(ctx, job)
	p.delayedCancel()
	return err
}

// TestProcessJobWithRealHandlers は実際のハンドラを使ったエンドツーエンドのテスト。
func TestProcessJobWithRealHandlers(t *testing.T) {
	db := testutil.OpenTestDB(t)
	insertTestProject(t, db, "p1")

	runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 実際のプロセッサをラッパーに包む（ジョブ処理後に cancel で daemon を停止）
	inner := worker.NewDefaultJobProcessor(db)
	proc := &cancelOnHandleProcessor{inner: inner, cancel: cancel}
	d, q := newTestDaemonWithProcessor(t, db, proc)

	// checkpoint_ingest ジョブを enqueue
	payload := makeCheckpointPayload(t, "s1", "p1", "decided to use Go for the backend")
	jobID, err := q.Enqueue(context.Background(), queue.JobTypeCheckpointIngest, payload)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := d.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	queryCtx := context.Background()
	// ジョブが done
	var status string
	row := db.QueryRowContext(queryCtx, "SELECT status FROM jobs WHERE job_id = ?", jobID)
	if err := row.Scan(&status); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected status=done, got %s", status)
	}

	// chunks に 1 件以上
	var chunkCount int
	row = db.QueryRowContext(queryCtx, "SELECT COUNT(*) FROM chunks")
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
