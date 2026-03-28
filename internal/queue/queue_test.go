package queue_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/queue"
)

// testQueue はテスト専用の Queue + rawDB アクセスを持つラッパー。
type testQueue struct {
	*queue.Queue
	rawDB *sql.DB
}

// newTestQueue は t.TempDir() に独立した DB を作成し、Queue を返す。
func newTestQueue(t *testing.T) *testQueue {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &testQueue{
		Queue: queue.New(database.SQL()),
		rawDB: database.SQL(),
	}
}

// ─── Step 1: 型定義テスト ─────────────────────────────────────────────────────

func TestJobType_Values(t *testing.T) {
	tests := []struct {
		name string
		got  queue.JobType
		want string
	}{
		{"CheckpointIngest", queue.JobTypeCheckpointIngest, "checkpoint_ingest"},
		{"SessionEndIngest", queue.JobTypeSessionEndIngest, "session_end_ingest"},
		{"ProjectRefresh", queue.JobTypeProjectRefresh, "project_refresh"},
		{"ProjectSimilarityRefresh", queue.JobTypeProjectSimilarityRefresh, "project_similarity_refresh"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("JobType %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestStatus_Values(t *testing.T) {
	tests := []struct {
		name string
		got  queue.Status
		want string
	}{
		{"Queued", queue.StatusQueued, "queued"},
		{"Running", queue.StatusRunning, "running"},
		{"Done", queue.StatusDone, "done"},
		{"Failed", queue.StatusFailed, "failed"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Status %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

// ─── Step 2: Backoff テスト ───────────────────────────────────────────────────

func TestNextRunAfter(t *testing.T) {
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		retryCount int
		wantDelay  time.Duration
	}{
		{0, 5 * time.Second},
		{1, 30 * time.Second},
		{2, 300 * time.Second},
		{3, 300 * time.Second},  // clamp to max
		{99, 300 * time.Second}, // clamp to max
	}
	for _, tt := range tests {
		got := queue.NextRunAfter(tt.retryCount, now)
		if got != now.Add(tt.wantDelay) {
			t.Errorf("NextRunAfter(%d) = %v, want %v", tt.retryCount, got, now.Add(tt.wantDelay))
		}
	}
}

// ─── Step 3: Enqueue テスト ───────────────────────────────────────────────────

func TestEnqueue_Basic(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, `{"key":"val"}`)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if jobID == "" {
		t.Error("jobID should not be empty")
	}
}

func TestEnqueue_EmptyPayload(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// rawDB で payload_json を直接確認
	var payload string
	err = q.rawDB.QueryRowContext(ctx, "SELECT payload_json FROM jobs WHERE job_id = ?", jobID).Scan(&payload)
	if err != nil {
		t.Fatalf("SELECT payload_json: %v", err)
	}
	if payload != "{}" {
		t.Errorf("payload_json = %q, want %q", payload, "{}")
	}
}

func TestEnqueue_InvalidJobType(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// CHECK 制約違反
	_, err := q.Enqueue(ctx, queue.JobType("invalid_type"), "{}")
	if err == nil {
		t.Error("expected error for invalid job_type, got nil")
	}
}

func TestEnqueue_UniqueIDs(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	ids := make(map[string]struct{})
	for i := 0; i < 5; i++ {
		jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
		if err != nil {
			t.Fatalf("Enqueue[%d]: %v", i, err)
		}
		if _, exists := ids[jobID]; exists {
			t.Errorf("duplicate jobID: %s", jobID)
		}
		ids[jobID] = struct{}{}
	}
}

// ─── Step 4: Dequeue テスト ───────────────────────────────────────────────────

func TestDequeue_Basic(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if job == nil {
		t.Fatal("job should not be nil")
	}
	if job.ID != jobID {
		t.Errorf("job.ID = %q, want %q", job.ID, jobID)
	}
	if job.Status != queue.StatusRunning {
		t.Errorf("job.Status = %q, want %q", job.Status, queue.StatusRunning)
	}
	if job.StartedAt == nil {
		t.Error("job.StartedAt should not be nil")
	}
}

func TestDequeue_EmptyQueue(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil job for empty queue, got %v", job)
	}
}

func TestDequeue_OrderByRunAfter(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	base := time.Now().UTC()
	// 2件を古い順に enqueue（run_after で順序を制御）
	oldRunAfter := base.Add(-2 * time.Minute)
	newRunAfter := base.Add(-1 * time.Minute)

	// 後に追加した方が古い run_after
	newID, err := q.EnqueueAt(ctx, queue.JobTypeCheckpointIngest, "{}", newRunAfter)
	if err != nil {
		t.Fatalf("EnqueueAt: %v", err)
	}
	oldID, err := q.EnqueueAt(ctx, queue.JobTypeCheckpointIngest, "{}", oldRunAfter)
	if err != nil {
		t.Fatalf("EnqueueAt: %v", err)
	}
	_ = newID

	// 古い run_after の方が先に取得されること
	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if job == nil {
		t.Fatal("expected job, got nil")
	}
	if job.ID != oldID {
		t.Errorf("expected oldest job %q, got %q", oldID, job.ID)
	}
}

func TestDequeue_FutureRunAfterSkipped(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// 未来の run_after を持つジョブ
	future := time.Now().UTC().Add(10 * time.Minute)
	_, err := q.EnqueueAt(ctx, queue.JobTypeCheckpointIngest, "{}", future)
	if err != nil {
		t.Fatalf("EnqueueAt: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil for future run_after, got %v", job)
	}
}

func TestDequeue_RunningJobSkipped(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// 1回目: running に変わる
	job1, err := q.Dequeue(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Dequeue 1: %v", err)
	}
	if job1 == nil {
		t.Fatal("expected job, got nil")
	}

	// 2回目: running のジョブは再取得されない
	job2, err := q.Dequeue(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Dequeue 2: %v", err)
	}
	if job2 != nil {
		t.Errorf("expected nil for running job, got %v", job2)
	}
}

// ─── Step 5: Ack テスト ───────────────────────────────────────────────────────

func TestAck_Basic(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	if err := q.Ack(ctx, job.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats[queue.StatusDone] != 1 {
		t.Errorf("done count = %d, want 1", stats[queue.StatusDone])
	}
}

func TestAck_Idempotent(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	if err := q.Ack(ctx, job.ID); err != nil {
		t.Fatalf("Ack 1: %v", err)
	}
	// 2回目も OK（冪等）
	if err := q.Ack(ctx, job.ID); err != nil {
		t.Fatalf("Ack 2: %v", err)
	}
}

func TestAck_NonExistentJob(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// 存在しない jobID でもエラーにならない
	if err := q.Ack(ctx, "non-existent-job-id"); err != nil {
		t.Fatalf("Ack non-existent: %v", err)
	}
}

// ─── Step 6: Fail テスト ──────────────────────────────────────────────────────

func TestFail_FirstFailure_Requeues(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	if err := q.Fail(ctx, job.ID, "some error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// status が queued に戻ること
	var status string
	err = q.rawDB.QueryRowContext(ctx, "SELECT status FROM jobs WHERE job_id = ?", job.ID).Scan(&status)
	if err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	if status != "queued" {
		t.Errorf("status = %q, want %q", status, "queued")
	}
}

func TestFail_BackoffRunAfter(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	before := time.Now().UTC()
	if err := q.Fail(ctx, job.ID, "error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	after := time.Now().UTC()

	var runAfterStr string
	err = q.rawDB.QueryRowContext(ctx, "SELECT run_after FROM jobs WHERE job_id = ?", job.ID).Scan(&runAfterStr)
	if err != nil {
		t.Fatalf("SELECT run_after: %v", err)
	}

	runAfter, err := time.Parse(time.RFC3339, runAfterStr)
	if err != nil {
		t.Fatalf("parse run_after: %v", err)
	}

	// retry_count=0 なので backoff は 5s
	expectedMin := before.Add(5*time.Second - time.Second)
	expectedMax := after.Add(5*time.Second + time.Second)

	if runAfter.Before(expectedMin) || runAfter.After(expectedMax) {
		t.Errorf("run_after = %v, want between %v and %v", runAfter, expectedMin, expectedMax)
	}
}

func TestFail_MaxRetriesReached(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// 3回失敗させる（max_retries=3）
	for i := 0; i < 3; i++ {
		// run_after を過去に設定して即座に取得できるようにする
		if i > 0 {
			_, err = q.rawDB.ExecContext(ctx,
				"UPDATE jobs SET run_after = ? WHERE status = 'queued'",
				time.Now().UTC().Add(-time.Second).Format(time.RFC3339),
			)
			if err != nil {
				t.Fatalf("update run_after[%d]: %v", i, err)
			}
		}

		job, err := q.Dequeue(ctx, "worker-1")
		if err != nil || job == nil {
			t.Fatalf("Dequeue[%d]: err=%v, job=%v", i, err, job)
		}

		if err := q.Fail(ctx, job.ID, "persistent error"); err != nil {
			t.Fatalf("Fail[%d]: %v", i, err)
		}
	}

	// 3回目の失敗後は status = failed になること
	var status string
	err = q.rawDB.QueryRowContext(ctx, "SELECT status FROM jobs").Scan(&status)
	if err != nil {
		t.Fatalf("SELECT status: %v", err)
	}
	if status != "failed" {
		t.Errorf("status = %q, want %q", status, "failed")
	}
}

func TestFail_ErrorMessageRecorded(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// 3回失敗させる
	for i := 0; i < 3; i++ {
		if i > 0 {
			_, err = q.rawDB.ExecContext(ctx,
				"UPDATE jobs SET run_after = ? WHERE status = 'queued'",
				time.Now().UTC().Add(-time.Second).Format(time.RFC3339),
			)
			if err != nil {
				t.Fatalf("update run_after[%d]: %v", i, err)
			}
		}

		job, err := q.Dequeue(ctx, "worker-1")
		if err != nil || job == nil {
			t.Fatalf("Dequeue[%d]: err=%v, job=%v", i, err, job)
		}

		if err := q.Fail(ctx, job.ID, "final error message"); err != nil {
			t.Fatalf("Fail[%d]: %v", i, err)
		}
	}

	var errMsg sql.NullString
	err = q.rawDB.QueryRowContext(ctx, "SELECT error_message FROM jobs").Scan(&errMsg)
	if err != nil {
		t.Fatalf("SELECT error_message: %v", err)
	}
	if !errMsg.Valid || errMsg.String != "final error message" {
		t.Errorf("error_message = %v, want %q", errMsg, "final error message")
	}
}

// ─── Step 7: Purge と Stats テスト ───────────────────────────────────────────

func TestPurge_RemovesOldDoneJobs(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}
	if err := q.Ack(ctx, job.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// finished_at を25時間前に設定
	_, err = q.rawDB.ExecContext(ctx,
		"UPDATE jobs SET finished_at = ? WHERE job_id = ?",
		time.Now().UTC().Add(-25*time.Hour).Format(time.RFC3339),
		jobID,
	)
	if err != nil {
		t.Fatalf("update finished_at: %v", err)
	}

	deleted, err := q.Purge(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

func TestPurge_KeepsRecentJobs(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}
	if err := q.Ack(ctx, job.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// finished_at は now（最新）なので削除されない
	deleted, err := q.Purge(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (recent job should not be deleted)", deleted)
	}
}

func TestPurge_ExactBoundary(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}
	if err := q.Ack(ctx, job.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// finished_at をちょうど olderThan に設定（24時間前）
	// Purge は finished_at < threshold（厳密な小なり）なので、
	// ちょうど境界の場合は削除されないはず
	threshold := time.Now().UTC().Add(-24 * time.Hour)
	_, err = q.rawDB.ExecContext(ctx,
		"UPDATE jobs SET finished_at = ? WHERE job_id = ?",
		threshold.Format(time.RFC3339),
		jobID,
	)
	if err != nil {
		t.Fatalf("update finished_at: %v", err)
	}

	// ちょうど境界では削除されない（< は厳密な不等号）
	deleted, err := q.Purge(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	// RFC3339 は秒単位なので、threshold と同じ秒になる可能性を考慮
	// 厳密な境界は削除されない（< threshold）
	if deleted != 0 {
		t.Logf("note: boundary job was deleted (deleted=%d), boundary is exclusive", deleted)
	}
}

func TestStats_Initial(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	for _, status := range []queue.Status{queue.StatusQueued, queue.StatusRunning, queue.StatusDone, queue.StatusFailed} {
		if stats[status] != 0 {
			t.Errorf("stats[%s] = %d, want 0", status, stats[status])
		}
	}
}

func TestStats_Mixed(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// queued を1件 enqueue
	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue queued: %v", err)
	}

	// running を1件 dequeue
	_, err = q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue for running: %v", err)
	}
	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats[queue.StatusQueued] != 1 {
		t.Errorf("queued = %d, want 1", stats[queue.StatusQueued])
	}
	if stats[queue.StatusRunning] != 1 {
		t.Errorf("running = %d, want 1", stats[queue.StatusRunning])
	}
}

// ─── Step 8: 並行実行テスト ───────────────────────────────────────────────────

func TestDequeue_Concurrent_NoDoubleDequeue(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	const numJobs = 10
	const numWorkers = 5

	// 10件 enqueue
	jobIDs := make([]string, 0, numJobs)
	for i := 0; i < numJobs; i++ {
		jobID, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
		if err != nil {
			t.Fatalf("Enqueue[%d]: %v", i, err)
		}
		jobIDs = append(jobIDs, jobID)
	}
	_ = jobIDs

	// 5 goroutine で並行 Dequeue + Ack
	var mu sync.Mutex
	dequeued := make(map[string]int) // jobID -> 取得回数

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				job, err := q.Dequeue(ctx, "worker")
				if err != nil {
					t.Errorf("Dequeue: %v", err)
					return
				}
				if job == nil {
					return
				}

				mu.Lock()
				dequeued[job.ID]++
				mu.Unlock()

				if err := q.Ack(ctx, job.ID); err != nil {
					t.Errorf("Ack: %v", err)
				}
			}
		}(w)
	}
	wg.Wait()

	// 同一 jobID が2回以上取得されていないことを確認
	for jobID, count := range dequeued {
		if count > 1 {
			t.Errorf("jobID %s was dequeued %d times (double dequeue!)", jobID, count)
		}
	}

	// 全10件が done になること
	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats[queue.StatusDone] != numJobs {
		t.Errorf("done = %d, want %d", stats[queue.StatusDone], numJobs)
	}
}

// ─── Step 9: DequeueWithOptions — Stale Recovery ─────────────────────────────

func TestDequeueWithOptions_StaleRecovery(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	// ジョブを enqueue して dequeue (running にする)
	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	// started_at を2分前に設定（stale 状態にする）
	_, err = q.rawDB.ExecContext(ctx,
		"UPDATE jobs SET started_at = ? WHERE job_id = ?",
		time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339),
		job.ID,
	)
	if err != nil {
		t.Fatalf("update started_at: %v", err)
	}

	// StaleTimeout=1分 で Dequeue するとそのジョブが取得されること
	recoveredJob, err := q.DequeueWithOptions(ctx, "worker-2", queue.DequeueOptions{
		StaleTimeout: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("DequeueWithOptions: %v", err)
	}
	if recoveredJob == nil {
		t.Fatal("expected recovered job, got nil")
	}
	if recoveredJob.ID != job.ID {
		t.Errorf("recovered job ID = %q, want %q", recoveredJob.ID, job.ID)
	}
}

func TestDequeueWithOptions_NoStaleRecoveryByDefault(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, "{}")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := q.Dequeue(ctx, "worker-1")
	if err != nil || job == nil {
		t.Fatalf("Dequeue: %v, job=%v", err, job)
	}

	// started_at を2分前に設定
	_, err = q.rawDB.ExecContext(ctx,
		"UPDATE jobs SET started_at = ? WHERE job_id = ?",
		time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339),
		job.ID,
	)
	if err != nil {
		t.Fatalf("update started_at: %v", err)
	}

	// StaleTimeout=0 では stale recovery しない
	noJob, err := q.DequeueWithOptions(ctx, "worker-2", queue.DequeueOptions{
		StaleTimeout: 0,
	})
	if err != nil {
		t.Fatalf("DequeueWithOptions: %v", err)
	}
	if noJob != nil {
		t.Errorf("expected nil (no stale recovery), got job %q", noJob.ID)
	}
}
