package worker

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Liveness は worker の生存状態を表す。
type Liveness int

const (
	// LivenessAlive は heartbeat が 3 秒以内の状態。
	LivenessAlive Liveness = iota
	// LivenessSuspect は heartbeat が 3〜10 秒の状態。
	LivenessSuspect
	// LivenessStale は heartbeat が 10 秒超の状態。
	LivenessStale
	// LivenessNotRunning は worker_leases にレコードがない状態。
	LivenessNotRunning
)

// String は Liveness の文字列表現を返す。
func (l Liveness) String() string {
	switch l {
	case LivenessAlive:
		return "alive"
	case LivenessSuspect:
		return "suspect"
	case LivenessStale:
		return "stale"
	case LivenessNotRunning:
		return "not_running"
	default:
		return "unknown"
	}
}

// WorkerLease は worker_leases テーブルの1行を表す。
type WorkerLease struct {
	WorkerName      string
	WorkerID        string
	PID             int
	StartedAt       time.Time
	LastHeartbeatAt time.Time
	LastProgressAt  *time.Time
	CurrentJobID    *string
}

// UpsertLease は worker_leases に INSERT OR REPLACE する。
func UpsertLease(ctx context.Context, db *sql.DB, lease WorkerLease) error {
	const query = `
INSERT OR REPLACE INTO worker_leases
  (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES (?, ?, ?, ?, ?)`

	now := time.Now().UTC().Format(time.RFC3339)
	startedAt := lease.StartedAt.UTC().Format(time.RFC3339)

	_, err := db.ExecContext(ctx, query,
		lease.WorkerName,
		lease.WorkerID,
		lease.PID,
		startedAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert lease: %w", err)
	}
	return nil
}

// UpdateHeartbeat は last_heartbeat_at を現在時刻に更新する。
func UpdateHeartbeat(ctx context.Context, db *sql.DB, workerName string) error {
	const query = `
UPDATE worker_leases SET last_heartbeat_at = ?
WHERE worker_name = ?`

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, query, now, workerName)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	return nil
}

// DeleteLease は worker_leases から worker_name のレコードを削除する。
func DeleteLease(ctx context.Context, db *sql.DB, workerName string) error {
	const query = `DELETE FROM worker_leases WHERE worker_name = ?`
	_, err := db.ExecContext(ctx, query, workerName)
	if err != nil {
		return fmt.Errorf("delete lease: %w", err)
	}
	return nil
}

// GetLease は worker_leases から worker_name のレコードを取得する。
// レコードが存在しない場合は nil, nil を返す。
func GetLease(ctx context.Context, db *sql.DB, workerName string) (*WorkerLease, error) {
	const query = `
SELECT worker_name, worker_id, pid, started_at, last_heartbeat_at,
       last_progress_at, current_job_id
FROM worker_leases
WHERE worker_name = ?`

	row := db.QueryRowContext(ctx, query, workerName)

	var lease WorkerLease
	var startedAtStr, lastHeartbeatAtStr string
	var lastProgressAtStr sql.NullString
	var currentJobID sql.NullString

	err := row.Scan(
		&lease.WorkerName,
		&lease.WorkerID,
		&lease.PID,
		&startedAtStr,
		&lastHeartbeatAtStr,
		&lastProgressAtStr,
		&currentJobID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lease: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, startedAtStr); err == nil {
		lease.StartedAt = t
	}
	if t, err := time.Parse(time.RFC3339, lastHeartbeatAtStr); err == nil {
		lease.LastHeartbeatAt = t
	}
	if lastProgressAtStr.Valid {
		if t, err := time.Parse(time.RFC3339, lastProgressAtStr.String); err == nil {
			lease.LastProgressAt = &t
		}
	}
	if currentJobID.Valid {
		lease.CurrentJobID = &currentJobID.String
	}

	return &lease, nil
}

// CheckLiveness は worker の liveness を判定して返す。
// alive: 3 秒以内, suspect: 3〜10 秒, stale: 10 秒超, not_running: レコードなし
func CheckLiveness(ctx context.Context, db *sql.DB, workerName string) (Liveness, *WorkerLease, error) {
	lease, err := GetLease(ctx, db, workerName)
	if err != nil {
		return LivenessNotRunning, nil, err
	}
	if lease == nil {
		return LivenessNotRunning, nil, nil
	}

	elapsed := time.Since(lease.LastHeartbeatAt)
	switch {
	case elapsed <= 3*time.Second:
		return LivenessAlive, lease, nil
	case elapsed <= 10*time.Second:
		return LivenessSuspect, lease, nil
	default:
		return LivenessStale, lease, nil
	}
}

// DeletePendingProbes は responded_at が NULL の probe を削除する（停止時のクリーンアップ）。
func DeletePendingProbes(ctx context.Context, db *sql.DB, workerName string, workerID string) error {
	const query = `
DELETE FROM worker_probes
WHERE worker_name = ? AND target_worker_id = ? AND responded_at IS NULL`

	_, err := db.ExecContext(ctx, query, workerName, workerID)
	if err != nil {
		return fmt.Errorf("delete pending probes: %w", err)
	}
	return nil
}

// RespondToProbes は自分宛の未応答 probe に responded_at を設定する。
func RespondToProbes(ctx context.Context, db *sql.DB, workerName string, workerID string) error {
	const query = `
UPDATE worker_probes SET responded_at = ?
WHERE worker_name = ? AND target_worker_id = ? AND responded_at IS NULL`

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, query, now, workerName, workerID)
	if err != nil {
		return fmt.Errorf("respond to probes: %w", err)
	}
	return nil
}

// InsertProbe は worker_probes に probe レコードを挿入する。
func InsertProbe(ctx context.Context, db *sql.DB, probeID, workerName, targetWorkerID string, requestedByPID int) error {
	const query = `
INSERT INTO worker_probes (probe_id, worker_name, target_worker_id, requested_by_pid)
VALUES (?, ?, ?, ?)`

	_, err := db.ExecContext(ctx, query, probeID, workerName, targetWorkerID, requestedByPID)
	if err != nil {
		return fmt.Errorf("insert probe: %w", err)
	}
	return nil
}

// UpdateLeaseJobID は worker_leases の current_job_id を更新する。
// jobID が空文字列の場合は NULL を設定する。
func UpdateLeaseJobID(ctx context.Context, db *sql.DB, workerName string, jobID string) error {
	var val interface{}
	if jobID != "" {
		val = jobID
	}
	const query = `UPDATE worker_leases SET current_job_id = ? WHERE worker_name = ?`
	_, err := db.ExecContext(ctx, query, val, workerName)
	if err != nil {
		return fmt.Errorf("update lease job_id: %w", err)
	}
	return nil
}

// UpdateLeaseProgress は worker_leases の last_progress_at を現在時刻に更新する。
func UpdateLeaseProgress(ctx context.Context, db *sql.DB, workerName string) error {
	const query = `UPDATE worker_leases SET last_progress_at = ? WHERE worker_name = ?`
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, query, now, workerName)
	if err != nil {
		return fmt.Errorf("update lease progress: %w", err)
	}
	return nil
}

// CheckProbeResponded は probe_id の responded_at が設定されているか確認する。
func CheckProbeResponded(ctx context.Context, db *sql.DB, probeID string) (bool, error) {
	const query = `SELECT responded_at FROM worker_probes WHERE probe_id = ?`

	var respondedAt sql.NullString
	err := db.QueryRowContext(ctx, query, probeID).Scan(&respondedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check probe: %w", err)
	}
	return respondedAt.Valid, nil
}
