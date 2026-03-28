package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
)

// newTestDaemon はテスト用の IngestDaemon を作成する。
// idleTimeout を短縮して高速なテストを実現する。
func newTestDaemon(t *testing.T, idleTimeout time.Duration) (*IngestDaemon, *queue.Queue) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	q := queue.New(db)
	runDir := t.TempDir()
	logDir := t.TempDir()

	d := NewIngestDaemon(db, q, runDir, logDir, idleTimeout)
	// テスト中はログを無視
	d.SetLogf(func(string, ...any) {})
	return d, q
}

func TestDaemonRunIdleTimeout(t *testing.T) {
	d, _ := newTestDaemon(t, 200*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := d.Run(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// idle timeout (200ms) + ポーリング (500ms) 以内に終了するはず
	// (上限を余裕を持って 3s に設定)
	if elapsed > 3*time.Second {
		t.Errorf("expected idle timeout within 3s, took %v", elapsed)
	}
}

func TestDaemonRunStopFile(t *testing.T) {
	d, _ := newTestDaemon(t, 30*time.Second) // idle timeout は長めに設定

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.Run(ctx)
	}()

	// daemon が起動するまで少し待つ
	time.Sleep(100 * time.Millisecond)

	// stop ファイルを作成
	stopPath := filepath.Join(d.runDir, "ingest.stop")
	if err := TouchFile(stopPath); err != nil {
		t.Fatalf("TouchFile: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("daemon did not stop after stop file creation within 5s")
	}
}

func TestDaemonCleansUpOnExit(t *testing.T) {
	d, _ := newTestDaemon(t, 100*time.Millisecond)

	ctx := context.Background()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// PID ファイルが削除されていること
	pidPath := filepath.Join(d.runDir, "ingest.pid")
	if FileExists(pidPath) {
		t.Error("expected pid file to be removed after daemon exit")
	}

	// worker_leases が削除されていること
	db := testutil.OpenTestDB(t)
	_ = db // 別の DB インスタンスでは確認できないため、daemon の DB で確認
	// daemon の d.db で確認する
	liveness, _, err := CheckLiveness(ctx, d.db, WorkerNameIngest)
	if err != nil {
		t.Fatalf("CheckLiveness: %v", err)
	}
	if liveness != LivenessNotRunning {
		t.Errorf("expected not_running after daemon exit, got %s", liveness)
	}
}

func TestDaemonDoubleStart(t *testing.T) {
	// 同じ runDir で2つ目の daemon を起動した場合は即座に終了する
	db := testutil.OpenTestDB(t)
	q := queue.New(db)
	runDir := t.TempDir()
	logDir := t.TempDir()

	d1 := NewIngestDaemon(db, q, runDir, logDir, 30*time.Second)
	d1.SetLogf(func(string, ...any) {})

	d2 := NewIngestDaemon(db, q, runDir, logDir, 30*time.Second)
	d2.SetLogf(func(string, ...any) {})

	ctx, cancel := context.WithCancel(context.Background())

	// d1 を起動
	d1Done := make(chan error, 1)
	go func() {
		d1Done <- d1.Run(ctx)
	}()

	// d1 が起動するまで待つ
	time.Sleep(100 * time.Millisecond)

	// d2 を起動 -> ロック取得失敗で即終了するはず
	d2Start := time.Now()
	err := d2.Run(context.Background())
	d2Elapsed := time.Since(d2Start)

	if err != nil {
		t.Errorf("d2.Run expected nil (graceful), got: %v", err)
	}

	// d2 が即座に終了することを確認（2 秒以内）
	if d2Elapsed > 2*time.Second {
		t.Errorf("d2 should exit immediately, took %v", d2Elapsed)
	}

	// d1 を停止
	cancel()
	select {
	case <-d1Done:
	case <-time.After(3 * time.Second):
		t.Error("d1 did not stop")
	}
}
