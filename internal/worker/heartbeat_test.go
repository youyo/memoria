package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/youyo/memoria/internal/testutil"
)

func TestHeartbeatUpdatesDB(t *testing.T) {
	db := testutil.OpenTestDB(t)
	workerID := uuid.New().String()

	// 古い heartbeat で lease を挿入
	const query = `
INSERT INTO worker_leases (worker_name, worker_id, pid, started_at, last_heartbeat_at)
VALUES ('ingest', ?, 1000, ?, ?)`
	oldTime := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	if _, err := db.ExecContext(context.Background(), query, workerID, oldTime, oldTime); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// heartbeat を短い間隔で起動
	go RunHeartbeat(ctx, db, "ingest", workerID, 50*time.Millisecond, nil)

	// 100ms 待機して heartbeat が更新されることを確認
	time.Sleep(200 * time.Millisecond)

	liveness, _, err := CheckLiveness(context.Background(), db, "ingest")
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessAlive {
		t.Errorf("expected alive after heartbeat, got %s", liveness)
	}
}

func TestHeartbeatStopsOnCancel(t *testing.T) {
	db := testutil.OpenTestDB(t)
	workerID := uuid.New().String()

	// lease を挿入
	lease := WorkerLease{
		WorkerName: "ingest",
		WorkerID:   workerID,
		PID:        2000,
		StartedAt:  time.Now().UTC(),
	}
	if err := UpsertLease(context.Background(), db, lease); err != nil {
		t.Fatalf("UpsertLease: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// heartbeat を起動
	done := make(chan struct{})
	go func() {
		RunHeartbeat(ctx, db, "ingest", workerID, 50*time.Millisecond, nil)
		close(done)
	}()

	// context をキャンセル
	cancel()

	// goroutine が終了するまで待つ
	select {
	case <-done:
		// 正常終了
	case <-time.After(2 * time.Second):
		t.Error("heartbeat goroutine did not stop after context cancel")
	}
}
