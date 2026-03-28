package queue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Queue は SQLite バックエンドのジョブキューを表す。
type Queue struct {
	db *sql.DB
}

// New は *sql.DB から Queue を作成する。
func New(db *sql.DB) *Queue {
	return &Queue{db: db}
}

// DequeueOptions は Dequeue の挙動を制御するオプション。
type DequeueOptions struct {
	// StaleTimeout: この時間を超えて running のままのジョブを queued に戻す。
	// 0 の場合は stale recovery を行わない（デフォルト）。
	StaleTimeout time.Duration
}

// normalizePayload は空文字列を "{}" に正規化する。
func normalizePayload(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

// withImmediateTx は BEGIN IMMEDIATE トランザクションを開始し、fn を実行する。
// fn がエラーを返した場合はロールバック、成功した場合はコミットする。
// SQLite の BEGIN IMMEDIATE は最初から write lock を取得するため、
// 複数プロセスが同時に Dequeue しても二重処理を防ぐことができる。
func withImmediateTx(ctx context.Context, db *sql.DB, fn func(conn *sql.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}

	if err := fn(conn); err != nil {
		conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		conn.ExecContext(ctx, "ROLLBACK") //nolint:errcheck
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Enqueue は新しいジョブをキューに追加する。
// jobID は uuid.New().String() で生成する。
// payloadJSON が空文字列の場合は "{}" を使う。
func (q *Queue) Enqueue(ctx context.Context, jobType JobType, payloadJSON string) (jobID string, err error) {
	return q.EnqueueAt(ctx, jobType, payloadJSON, time.Now().UTC())
}

// EnqueueAt は run_after を指定してジョブを追加する（テスト・将来の予約実行用）。
func (q *Queue) EnqueueAt(ctx context.Context, jobType JobType, payloadJSON string, runAfter time.Time) (jobID string, err error) {
	jobID = uuid.New().String()
	payloadJSON = normalizePayload(payloadJSON)
	runAfterStr := runAfter.UTC().Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)

	const query = `
INSERT INTO jobs (job_id, job_type, payload_json, status, retry_count, max_retries, run_after, created_at)
VALUES (?, ?, ?, 'queued', 0, 3, ?, ?)`

	if _, err := q.db.ExecContext(ctx, query, jobID, string(jobType), payloadJSON, runAfterStr, now); err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	return jobID, nil
}

// Dequeue は実行可能なジョブを1件取得する。
// BEGIN IMMEDIATE で write lock を取得して排他制御を行う。
// ジョブが存在しない場合は (nil, nil) を返す。
func (q *Queue) Dequeue(ctx context.Context, workerID string) (*Job, error) {
	return q.DequeueWithOptions(ctx, workerID, DequeueOptions{})
}

// DequeueWithOptions はオプションを指定して Dequeue を行う。
func (q *Queue) DequeueWithOptions(ctx context.Context, workerID string, opts DequeueOptions) (*Job, error) {
	var job *Job

	err := withImmediateTx(ctx, q.db, func(conn *sql.Conn) error {
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339)

		// Stale Recovery: StaleTimeout > 0 の場合、古い running ジョブを queued に戻す
		if opts.StaleTimeout > 0 {
			staleThreshold := now.Add(-opts.StaleTimeout).Format(time.RFC3339)
			const staleQuery = `
UPDATE jobs SET status = 'queued', started_at = NULL
WHERE status = 'running' AND started_at < ?`
			if _, err := conn.ExecContext(ctx, staleQuery, staleThreshold); err != nil {
				return fmt.Errorf("stale recovery: %w", err)
			}
		}

		// 実行可能なジョブを1件取得
		const selectQuery = `
SELECT job_id, job_type, payload_json, status, retry_count, max_retries, run_after,
       started_at, finished_at, error_message, created_at
FROM jobs
WHERE status = 'queued' AND run_after <= ?
ORDER BY run_after ASC
LIMIT 1`

		row := conn.QueryRowContext(ctx, selectQuery, nowStr)

		j := &Job{}
		var runAfterStr, createdAtStr string
		var startedAtStr, finishedAtStr, errMsg sql.NullString

		err := row.Scan(
			&j.ID, &j.Type, &j.PayloadJSON, &j.Status,
			&j.RetryCount, &j.MaxRetries, &runAfterStr,
			&startedAtStr, &finishedAtStr, &errMsg, &createdAtStr,
		)
		if err == sql.ErrNoRows {
			// ジョブなし
			return nil
		}
		if err != nil {
			return fmt.Errorf("select job: %w", err)
		}

		// 時刻パース
		if t, err := time.Parse(time.RFC3339, runAfterStr); err == nil {
			j.RunAfter = t
		}
		if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			j.CreatedAt = t
		}
		if startedAtStr.Valid {
			if t, err := time.Parse(time.RFC3339, startedAtStr.String); err == nil {
				j.StartedAt = &t
			}
		}
		if finishedAtStr.Valid {
			if t, err := time.Parse(time.RFC3339, finishedAtStr.String); err == nil {
				j.FinishedAt = &t
			}
		}
		if errMsg.Valid {
			j.ErrorMessage = &errMsg.String
		}

		// status を running に更新
		const updateQuery = `
UPDATE jobs SET status = 'running', started_at = ? WHERE job_id = ?`
		if _, err := conn.ExecContext(ctx, updateQuery, nowStr, j.ID); err != nil {
			return fmt.Errorf("update job to running: %w", err)
		}

		j.Status = StatusRunning
		startedAtTime := now
		j.StartedAt = &startedAtTime
		job = j
		return nil
	})

	if err != nil {
		return nil, err
	}
	return job, nil
}

// Ack はジョブを完了としてマークする。
// 存在しない job_id や既に done のジョブでもエラーにならない（冪等）。
func (q *Queue) Ack(ctx context.Context, jobID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	const query = `
UPDATE jobs SET status = 'done', finished_at = ?
WHERE job_id = ? AND status = 'running'`
	if _, err := q.db.ExecContext(ctx, query, now, jobID); err != nil {
		return fmt.Errorf("ack job: %w", err)
	}
	return nil
}

// shouldRetry は newRetryCount が maxRetries 未満かどうかを返す。
func shouldRetry(newRetryCount, maxRetries int) bool {
	return newRetryCount < maxRetries
}

// Fail はジョブを失敗としてマークする。
// retry_count < max_retries なら status='queued'、run_after=Backoff後の時刻に更新する。
// retry_count >= max_retries なら status='failed'、error_message を記録する。
func (q *Queue) Fail(ctx context.Context, jobID string, errMsg string) error {
	return withImmediateTx(ctx, q.db, func(conn *sql.Conn) error {
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339)

		// 1. 現在の retry_count と max_retries を取得
		var retryCount, maxRetries int
		const selectQuery = `SELECT retry_count, max_retries FROM jobs WHERE job_id = ?`
		if err := conn.QueryRowContext(ctx, selectQuery, jobID).Scan(&retryCount, &maxRetries); err != nil {
			if err == sql.ErrNoRows {
				// ジョブが存在しない場合は何もしない
				return nil
			}
			return fmt.Errorf("select retry_count: %w", err)
		}

		// 2. Go 側でリトライ判定
		newRetryCount := retryCount + 1

		if shouldRetry(newRetryCount, maxRetries) {
			// 3a. リトライ可能: queued に戻す
			runAfter := NextRunAfter(retryCount, now).Format(time.RFC3339)
			const updateQuery = `
UPDATE jobs SET status = 'queued', retry_count = ?, run_after = ?, error_message = ?
WHERE job_id = ? AND status = 'running'`
			if _, err := conn.ExecContext(ctx, updateQuery, newRetryCount, runAfter, errMsg, jobID); err != nil {
				return fmt.Errorf("update job to queued (retry): %w", err)
			}
		} else {
			// 3b. リトライ上限: failed にする
			const updateQuery = `
UPDATE jobs SET status = 'failed', retry_count = ?, finished_at = ?, error_message = ?
WHERE job_id = ? AND status = 'running'`
			if _, err := conn.ExecContext(ctx, updateQuery, newRetryCount, nowStr, errMsg, jobID); err != nil {
				return fmt.Errorf("update job to failed: %w", err)
			}
		}

		return nil
	})
}

// Purge は完了・失敗ジョブのうち、olderThan より古いものを削除する。
func (q *Queue) Purge(ctx context.Context, olderThan time.Duration) (deleted int64, err error) {
	threshold := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)

	const query = `
DELETE FROM jobs
WHERE status IN ('done', 'failed') AND finished_at < ?`

	result, err := q.db.ExecContext(ctx, query, threshold)
	if err != nil {
		return 0, fmt.Errorf("purge jobs: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

// Stats はキューの現在状態を返す（各 status のジョブ数）。
func (q *Queue) Stats(ctx context.Context) (map[Status]int, error) {
	// デフォルト値（全 Status = 0）を初期化
	stats := map[Status]int{
		StatusQueued:  0,
		StatusRunning: 0,
		StatusDone:    0,
		StatusFailed:  0,
	}

	const query = `SELECT status, COUNT(*) FROM jobs GROUP BY status`
	rows, err := q.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("stats query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status Status
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan stats: %w", err)
		}
		stats[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return stats, nil
}
