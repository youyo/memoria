package worker

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/youyo/memoria/internal/testutil"
)

// ensureIngestWithDB のテスト用ラッパー（spawn は実際には行わない）
// 実際の spawn テストはインテグレーションテストになるため、
// ここでは liveness 判定部分のみテストする。

func TestEnsureIngest_AlreadyRunning(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// alive な lease を挿入
	workerID := uuid.New().String()
	lease := WorkerLease{
		WorkerName: WorkerNameIngest,
		WorkerID:   workerID,
		PID:        12345,
		StartedAt:  time.Now().UTC(),
	}
	if err := UpsertLease(context.Background(), db, lease); err != nil {
		t.Fatalf("UpsertLease: %v", err)
	}

	// alive の場合は spawn しないことを確認（spawn を検知するためのカウンター）
	spawnCount := 0
	checkLivenessAndMaybeSpawn(t, db, &spawnCount)

	if spawnCount != 0 {
		t.Errorf("expected 0 spawns for alive worker, got %d", spawnCount)
	}
}

func TestEnsureIngest_NotRunning(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// no lease -> not_running -> spawn するはず
	spawnCount := 0
	checkLivenessAndMaybeSpawn(t, db, &spawnCount)

	if spawnCount != 1 {
		t.Errorf("expected 1 spawn for not_running worker, got %d", spawnCount)
	}
}

func TestEnsureIngest_Stale(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// stale な lease を挿入（15 秒前）
	const query = `
INSERT INTO worker_leases (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES ('ingest', 'stale-id', 9999, ?, ?)`
	oldTime := time.Now().UTC().Add(-15 * time.Second).Format(time.RFC3339)
	if _, err := db.ExecContext(context.Background(), query, oldTime, oldTime); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// stale -> spawn するはず
	spawnCount := 0
	checkLivenessAndMaybeSpawn(t, db, &spawnCount)

	if spawnCount != 1 {
		t.Errorf("expected 1 spawn for stale worker, got %d", spawnCount)
	}
}

// checkLivenessAndMaybeSpawn は ensureIngestWithDB の liveness チェック部分のみを
// テストするためのヘルパー。実際の spawn の代わりにカウンターをインクリメントする。
func checkLivenessAndMaybeSpawn(t *testing.T, db *sql.DB, spawnCount *int) {
	t.Helper()

	ctx := context.Background()
	liveness, _, err := CheckLiveness(ctx, db, WorkerNameIngest)
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}

	switch liveness {
	case LivenessAlive:
		// spawn しない
	case LivenessSuspect, LivenessStale, LivenessNotRunning:
		*spawnCount++
	}
}

func TestEnsureIngest_Suspect_ShortContext(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// suspect な lease を挿入（5 秒前）
	const query = `
INSERT INTO worker_leases (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES ('ingest', 'suspect-id', 8888, ?, ?)`
	fiveSecondsAgo := time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339)
	if _, err := db.ExecContext(context.Background(), query, fiveSecondsAgo, fiveSecondsAgo); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// 残り時間が短い context (< minProbeTimeRemaining) で ensureIngestWithDB を呼ぶと
	// probe をスキップして spawn するはず
	// ここでは spawnDaemon の代わりに liveness チェックの動作のみを検証

	liveness, _, err := CheckLiveness(context.Background(), db, WorkerNameIngest)
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessSuspect {
		t.Errorf("expected suspect, got %s", liveness)
	}
}
