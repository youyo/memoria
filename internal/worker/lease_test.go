package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/youyo/memoria/internal/testutil"
)

func TestUpsertLease(t *testing.T) {
	db := testutil.OpenTestDB(t)

	workerID := uuid.New().String()
	lease := WorkerLease{
		WorkerName: "ingest",
		WorkerID:   workerID,
		PID:        12345,
		StartedAt:  time.Now().UTC(),
	}

	if err := UpsertLease(t.Context(), db, lease); err != nil {
		t.Fatalf("UpsertLease: %v", err)
	}

	// 取得して確認
	got, err := GetLease(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got == nil {
		t.Fatal("expected lease, got nil")
	}
	if got.WorkerID != workerID {
		t.Errorf("expected worker_id %s, got %s", workerID, got.WorkerID)
	}
	if got.PID != 12345 {
		t.Errorf("expected pid 12345, got %d", got.PID)
	}
}

func TestDeleteLease(t *testing.T) {
	db := testutil.OpenTestDB(t)

	workerID := uuid.New().String()
	lease := WorkerLease{
		WorkerName: "ingest",
		WorkerID:   workerID,
		PID:        99,
		StartedAt:  time.Now().UTC(),
	}
	if err := UpsertLease(t.Context(), db, lease); err != nil {
		t.Fatalf("UpsertLease: %v", err)
	}

	if err := DeleteLease(t.Context(), db, "ingest"); err != nil {
		t.Fatalf("DeleteLease: %v", err)
	}

	got, err := GetLease(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete, got lease")
	}
}

func TestCheckLiveness_NotRunning(t *testing.T) {
	db := testutil.OpenTestDB(t)

	liveness, lease, err := CheckLiveness(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessNotRunning {
		t.Errorf("expected not_running, got %s", liveness)
	}
	if lease != nil {
		t.Error("expected nil lease")
	}
}

func TestCheckLiveness_Alive(t *testing.T) {
	db := testutil.OpenTestDB(t)

	workerID := uuid.New().String()
	lease := WorkerLease{
		WorkerName: "ingest",
		WorkerID:   workerID,
		PID:        100,
		StartedAt:  time.Now().UTC(),
	}
	if err := UpsertLease(t.Context(), db, lease); err != nil {
		t.Fatalf("UpsertLease: %v", err)
	}

	// UpsertLease は現在時刻で last_heartbeat_at を設定するので alive になる
	liveness, _, err := CheckLiveness(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessAlive {
		t.Errorf("expected alive, got %s", liveness)
	}
}

func TestCheckLiveness_Suspect(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// 5 秒前の heartbeat を直接 INSERT する
	const query = `
INSERT INTO worker_leases (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES ('ingest', 'test-id', 200, ?, ?)`

	fiveSecondsAgo := time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339)
	if _, err := db.ExecContext(t.Context(), query, fiveSecondsAgo, fiveSecondsAgo); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	liveness, _, err := CheckLiveness(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessSuspect {
		t.Errorf("expected suspect, got %s", liveness)
	}
}

func TestCheckLiveness_Stale(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// 15 秒前の heartbeat を直接 INSERT する
	const query = `
INSERT INTO worker_leases (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES ('ingest', 'test-id', 300, ?, ?)`

	fifteenSecondsAgo := time.Now().UTC().Add(-15 * time.Second).Format(time.RFC3339)
	if _, err := db.ExecContext(t.Context(), query, fifteenSecondsAgo, fifteenSecondsAgo); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	liveness, _, err := CheckLiveness(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessStale {
		t.Errorf("expected stale, got %s", liveness)
	}
}

func TestUpdateHeartbeat(t *testing.T) {
	db := testutil.OpenTestDB(t)

	// 古い heartbeat で挿入
	const query = `
INSERT INTO worker_leases (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES ('ingest', 'test-id', 400, ?, ?)`

	oldTime := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	if _, err := db.ExecContext(t.Context(), query, oldTime, oldTime); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// UpdateHeartbeat を呼ぶ
	if err := UpdateHeartbeat(t.Context(), db, "ingest"); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}

	// 更新後は alive になるはず
	liveness, _, err := CheckLiveness(t.Context(), db, "ingest")
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessAlive {
		t.Errorf("expected alive after heartbeat update, got %s", liveness)
	}
}

func TestProbe(t *testing.T) {
	db := testutil.OpenTestDB(t)

	probeID := uuid.New().String()
	workerID := uuid.New().String()

	// probe を挿入
	if err := InsertProbe(t.Context(), db, probeID, "ingest", workerID, 999); err != nil {
		t.Fatalf("InsertProbe: %v", err)
	}

	// まだ responded_at なし
	responded, err := CheckProbeResponded(t.Context(), db, probeID)
	if err != nil {
		t.Fatalf("CheckProbeResponded: %v", err)
	}
	if responded {
		t.Error("expected not responded yet")
	}

	// RespondToProbes で応答
	if err := RespondToProbes(t.Context(), db, "ingest", workerID); err != nil {
		t.Fatalf("RespondToProbes: %v", err)
	}

	// 応答済みになる
	responded, err = CheckProbeResponded(t.Context(), db, probeID)
	if err != nil {
		t.Fatalf("CheckProbeResponded after respond: %v", err)
	}
	if !responded {
		t.Error("expected responded after RespondToProbes")
	}
}
