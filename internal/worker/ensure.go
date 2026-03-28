package worker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
)

// probePollInterval は probe 確認のポーリング間隔。
const probePollInterval = 100 * time.Millisecond

// spawnWaitDuration は spawn 後に liveness を確認するまでの待機時間。
const spawnWaitDuration = 300 * time.Millisecond

// minProbeTimeRemaining は probe をスキップして即 spawn するための最小残り時間。
// SessionEnd (800ms) のような短い hook では probe をスキップする。
const minProbeTimeRemaining = 500 * time.Millisecond

// EnsureIngest は ingest worker が起動していることを確認する。
// 起動していなければ self-spawn する。
// この関数は hook から呼ばれるため、ctx のタイムアウトを遵守する。
func EnsureIngest(ctx context.Context) {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureIngest: open db: %v\n", err)
		return
	}
	defer db.Close()

	ensureIngestWithDB(ctx, db.SQL())
}

// ensureIngestWithDB はテスト可能な内部実装。
func ensureIngestWithDB(ctx context.Context, db *sql.DB) {
	liveness, lease, err := CheckLiveness(ctx, db, WorkerNameIngest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureIngest: check liveness: %v\n", err)
		return
	}

	switch liveness {
	case LivenessAlive:
		// 既に起動中 -> 何もしない
		return

	case LivenessSuspect:
		// suspect: probe で強い確認を試みる
		// 残り時間が短い場合は probe をスキップ
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < minProbeTimeRemaining {
			// 残り時間が短いか deadline なし -> stale 扱いで spawn
			spawnDaemon(ctx)
			return
		}

		// probe を送信して応答を待つ
		probeTimeout := remaining - 100*time.Millisecond
		if probeTimeout > 2*time.Second {
			probeTimeout = 2 * time.Second
		}

		probeID := uuid.New().String()
		if err := InsertProbe(ctx, db, probeID, WorkerNameIngest, lease.WorkerID, os.Getpid()); err != nil {
			fmt.Fprintf(os.Stderr, "memoria: ensureIngest: insert probe: %v\n", err)
			// probe 失敗 -> stale 扱いで spawn
			spawnDaemon(ctx)
			return
		}

		// probe 応答を待つ
		probeCtx, cancelProbe := context.WithTimeout(ctx, probeTimeout)
		defer cancelProbe()

		if waitForProbe(probeCtx, db, probeID) {
			// alive 確認
			return
		}
		// 応答なし -> stale 扱いで spawn
		spawnDaemon(ctx)

	case LivenessStale, LivenessNotRunning:
		// stale / 未登録 -> spawn
		spawnDaemon(ctx)
	}
}

// waitForProbe は probe に応答があるまで待つ。
// 応答があれば true、タイムアウトなら false を返す。
func waitForProbe(ctx context.Context, db *sql.DB, probeID string) bool {
	ticker := time.NewTicker(probePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			responded, err := CheckProbeResponded(ctx, db, probeID)
			if err != nil {
				return false
			}
			if responded {
				return true
			}
		}
	}
}

// spawnDaemon は `memoria daemon ingest` を self-spawn する。
func spawnDaemon(ctx context.Context) {
	logPath := filepath.Join(config.LogDir(), "ingest.log")
	if err := os.MkdirAll(config.LogDir(), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureIngest: mkdir log dir: %v\n", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureIngest: open log file: %v\n", err)
		// ログファイルが開けない場合は /dev/null にリダイレクト
		logFile, err = os.Open(os.DevNull)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memoria: ensureIngest: open /dev/null: %v\n", err)
			return
		}
	}
	defer logFile.Close()

	execPath := os.Args[0]
	attr := &os.ProcAttr{
		Files: []*os.File{nil, logFile, logFile},
		Sys: &syscall.SysProcAttr{
			Setsid: true, // 親プロセスから切り離す
		},
	}

	proc, err := os.StartProcess(execPath, []string{execPath, "daemon", "ingest"}, attr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureIngest: spawn daemon: %v\n", err)
		return
	}

	// Wait しない（detached daemon）
	// proc.Release() でシステムリソースを解放
	if err := proc.Release(); err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureIngest: release proc: %v\n", err)
	}

	// spawn 後 300ms 待機して liveness を確認
	select {
	case <-ctx.Done():
		return
	case <-time.After(spawnWaitDuration):
	}
}

// openDB は config からデフォルト DB パスを使って DB を開く。
func openDB() (*db.DB, error) {
	dbPath := config.DBFile()
	return db.Open(dbPath)
}
