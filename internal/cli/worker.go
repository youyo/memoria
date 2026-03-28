package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/worker"
)

// WorkerCmd は worker 管理サブコマンドグループを定義する。
type WorkerCmd struct {
	Start   WorkerStartCmd   `cmd:"" help:"Worker を起動する"`
	Stop    WorkerStopCmd    `cmd:"" help:"Worker を停止する"`
	Restart WorkerRestartCmd `cmd:"" help:"Worker を再起動する"`
	Status  WorkerStatusCmd  `cmd:"" help:"Worker の状態を表示する"`
}

// WorkerStartCmd は worker start コマンド。
type WorkerStartCmd struct{}

// Run は worker start を実行する（本番用: DB を自動オープン）。
func (c *WorkerStartCmd) Run(globals *Globals, w *io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worker.EnsureIngest(ctx)

	database, err := openWorkerDB()
	if err != nil {
		fmt.Fprintln(*w, "started")
		return nil
	}
	defer database.Close()

	return c.runWithSQL(ctx, w, database.SQL())
}

// RunWithDB はテスト可能な実装。sqlDB と runDir を外部から注入する。
func (c *WorkerStartCmd) RunWithDB(globals *Globals, w *io.Writer, sqlDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// テスト時は EnsureIngest を呼ばない（本番 daemon を起動しない）
	return c.runWithSQL(ctx, w, sqlDB)
}

func (c *WorkerStartCmd) runWithSQL(ctx context.Context, w *io.Writer, sqlDB *sql.DB) error {
	liveness, _, err := worker.CheckLiveness(ctx, sqlDB, worker.WorkerNameIngest)
	if err != nil || liveness == worker.LivenessNotRunning {
		fmt.Fprintln(*w, "started")
		return nil
	}
	if liveness == worker.LivenessAlive {
		fmt.Fprintln(*w, "already running")
		return nil
	}
	fmt.Fprintln(*w, "started")
	return nil
}

// WorkerStopCmd は worker stop コマンド。
type WorkerStopCmd struct{}

// Run は worker stop を実行する（本番用: DB と runDir を自動解決）。
func (c *WorkerStopCmd) Run(globals *Globals, w *io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	database, err := openWorkerDB()
	if err != nil {
		fmt.Fprintln(*w, "was not running")
		return nil
	}
	defer database.Close()

	return c.runWithSQL(ctx, w, database.SQL(), config.RunDir())
}

// RunWithDB はテスト可能な実装。sqlDB と runDir を外部から注入する。
func (c *WorkerStopCmd) RunWithDB(globals *Globals, w *io.Writer, sqlDB *sql.DB, runDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return c.runWithSQL(ctx, w, sqlDB, runDir)
}

func (c *WorkerStopCmd) runWithSQL(ctx context.Context, w *io.Writer, sqlDB *sql.DB, runDir string) error {
	stopPath := runDir + "/ingest.stop"
	pidPath := runDir + "/ingest.pid"

	liveness, _, err := worker.CheckLiveness(ctx, sqlDB, worker.WorkerNameIngest)
	if err != nil || liveness == worker.LivenessNotRunning {
		fmt.Fprintln(*w, "was not running")
		return nil
	}

	// stop ファイルを作成
	if err := worker.TouchFile(stopPath); err != nil {
		fmt.Fprintf(os.Stderr, "memoria worker stop: touch stop file: %v\n", err)
		return nil
	}

	// 最大 5 秒待機して liveness が stale/not_running になるのを確認
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			goto forceKill
		case <-ticker.C:
			liveness, _, _ = worker.CheckLiveness(ctx, sqlDB, worker.WorkerNameIngest)
			if liveness == worker.LivenessNotRunning || liveness == worker.LivenessStale {
				fmt.Fprintln(*w, "stopped")
				return nil
			}
		}
	}

forceKill:
	// 5 秒経過後もまだ alive なら PID ファイルから pid を取得して SIGTERM
	pid, err := worker.ReadPID(pidPath)
	if err != nil || pid == 0 {
		fmt.Fprintln(*w, "stopped")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintln(*w, "stopped")
		return nil
	}

	// SIGTERM
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintln(*w, "stopped")
		return nil
	}

	// SIGTERM 後 3 秒待機
	termDeadline := time.Now().Add(3 * time.Second)
	ticker2 := time.NewTicker(200 * time.Millisecond)
	defer ticker2.Stop()

	for time.Now().Before(termDeadline) {
		select {
		case <-ctx.Done():
			goto sigkill
		case <-ticker2.C:
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				// プロセスが終了した
				fmt.Fprintln(*w, "stopped")
				return nil
			}
		}
	}

sigkill:
	// SIGKILL
	_ = proc.Signal(syscall.SIGKILL)
	fmt.Fprintln(*w, "stopped")
	return nil
}

// WorkerRestartCmd は worker restart コマンド。
type WorkerRestartCmd struct{}

// Run は worker restart を実行する。
func (c *WorkerRestartCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// WorkerStatusOutput は worker status の JSON 出力構造体。
type WorkerStatusOutput struct {
	Status          string `json:"status"`
	WorkerID        string `json:"worker_id,omitempty"`
	PID             int    `json:"pid,omitempty"`
	LastHeartbeatAt string `json:"last_heartbeat_at,omitempty"`
	UptimeSeconds   int64  `json:"uptime_seconds,omitempty"`
}

// WorkerStatusCmd は worker status コマンド。
type WorkerStatusCmd struct{}

// Run は worker status を実行する（本番用: DB を自動オープン）。
func (c *WorkerStatusCmd) Run(globals *Globals, w *io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output := WorkerStatusOutput{
		Status: "not_running",
	}

	database, err := openWorkerDB()
	if err == nil {
		defer database.Close()
		c.fillOutput(ctx, database.SQL(), &output)
	}

	return c.writeOutput(globals, w, &output)
}

// RunWithDB はテスト可能な実装。sqlDB を外部から注入する。
func (c *WorkerStatusCmd) RunWithDB(globals *Globals, w *io.Writer, sqlDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output := WorkerStatusOutput{
		Status: "not_running",
	}

	c.fillOutput(ctx, sqlDB, &output)
	return c.writeOutput(globals, w, &output)
}

func (c *WorkerStatusCmd) fillOutput(ctx context.Context, sqlDB *sql.DB, output *WorkerStatusOutput) {
	liveness, lease, err := worker.CheckLiveness(ctx, sqlDB, worker.WorkerNameIngest)
	if err == nil {
		output.Status = liveness.String()
		if lease != nil {
			output.WorkerID = lease.WorkerID
			output.PID = lease.PID
			output.LastHeartbeatAt = lease.LastHeartbeatAt.UTC().Format(time.RFC3339)
			output.UptimeSeconds = int64(time.Since(lease.StartedAt).Seconds())
		}
	}
}

func (c *WorkerStatusCmd) writeOutput(globals *Globals, w *io.Writer, output *WorkerStatusOutput) error {
	if globals.Format == "json" {
		b, _ := json.Marshal(output)
		fmt.Fprintln(*w, string(b))
	} else {
		fmt.Fprintf(*w, "status: %s\n", output.Status)
		if output.PID > 0 {
			fmt.Fprintf(*w, "pid: %d\n", output.PID)
		}
		if output.WorkerID != "" {
			fmt.Fprintf(*w, "worker_id: %s\n", output.WorkerID)
		}
		if output.LastHeartbeatAt != "" {
			fmt.Fprintf(*w, "last_heartbeat_at: %s\n", output.LastHeartbeatAt)
		}
		if output.UptimeSeconds > 0 {
			fmt.Fprintf(*w, "uptime_seconds: %d\n", output.UptimeSeconds)
		}
	}
	return nil
}

// openWorkerDB は worker コマンド用に DB を開く（エラーは非致命的）。
func openWorkerDB() (*db.DB, error) {
	dbPath := config.DBFile()
	return db.Open(dbPath)
}

// newWorkerQueue は DB から queue を作成する（テスト用 DI）。
func newWorkerQueue(database *db.DB) *queue.Queue {
	return queue.New(database.SQL())
}
