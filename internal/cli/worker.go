package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/embedding"
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
	cfg, _ := config.Load(config.ConfigFile())

	var wg sync.WaitGroup

	// ingest worker を並列起動
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		worker.EnsureIngest(ctx)
	}()

	// embedding worker を spawn（health check は待たない — hook 側で EnsureEmbedding する）
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if cfg != nil {
			if err := worker.SpawnEmbeddingIfNeeded(ctx, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "memoria worker start: embedding: %v\n", err)
			}
		}
	}()

	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	// テスト時は EnsureIngest / EnsureEmbedding を呼ばない（本番 daemon を起動しない）
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
	// embedding worker を先に停止
	stopWorkerByPID(ctx, filepath.Join(runDir, "embedding.pid"))

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

// stopWorkerByPID は PID ファイルからプロセスを探して SIGTERM -> SIGKILL で停止する。
// ingest / embedding 共通ヘルパー。
func stopWorkerByPID(ctx context.Context, pidPath string) {
	pid, err := worker.ReadPID(pidPath)
	if err != nil || pid == 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			_ = proc.Signal(syscall.SIGKILL)
			worker.RemovePID(pidPath) //nolint:errcheck
			return
		case <-time.After(200 * time.Millisecond):
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				// プロセスが終了した
				worker.RemovePID(pidPath) //nolint:errcheck
				return
			}
		}
	}
	_ = proc.Signal(syscall.SIGKILL)
	worker.RemovePID(pidPath) //nolint:errcheck
}

// WorkerRestartCmd は worker restart コマンド。
type WorkerRestartCmd struct{}

// Run は worker restart を実行する。
func (c *WorkerRestartCmd) Run(globals *Globals, w *io.Writer) error {
	stop := &WorkerStopCmd{}
	if err := stop.Run(globals, w); err != nil {
		return err
	}
	start := &WorkerStartCmd{}
	return start.Run(globals, w)
}

// EmbeddingWorkerStatus は embedding worker の状態を表す。
type EmbeddingWorkerStatus struct {
	Status     string `json:"status"` // "running" | "not_running" | "unknown"
	Model      string `json:"model,omitempty"`
	Dimensions int    `json:"dimensions,omitempty"`
	Device     string `json:"device,omitempty"`
	PID        int    `json:"pid,omitempty"`
}

// WorkerStatusOutput は worker status の JSON 出力構造体。
type WorkerStatusOutput struct {
	Status          string `json:"status"`
	WorkerID        string `json:"worker_id,omitempty"`
	PID             int    `json:"pid,omitempty"`
	LastHeartbeatAt string `json:"last_heartbeat_at,omitempty"`
	UptimeSeconds   int64  `json:"uptime_seconds,omitempty"`

	// M10 追加フィールド
	Embedding EmbeddingWorkerStatus `json:"embedding"`
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

	// embedding worker の状態を確認
	output.Embedding = checkEmbeddingStatus(ctx, config.SocketPath(), config.RunDir())

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

	// embedding worker の状態を確認（テスト時は接続できないので "not_running" になる）
	output.Embedding = checkEmbeddingStatus(ctx, config.SocketPath(), config.RunDir())

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

// checkEmbeddingStatus は embedding worker の状態を確認して EmbeddingWorkerStatus を返す。
func checkEmbeddingStatus(ctx context.Context, socketPath, runDir string) EmbeddingWorkerStatus {
	// 500ms タイムアウトで health チェック
	healthCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	client := embedding.New(socketPath)
	resp, err := client.Health(healthCtx)
	if err == nil && resp != nil {
		status := EmbeddingWorkerStatus{
			Status:     "running",
			Model:      resp.Model,
			Dimensions: resp.Dimensions,
			Device:     resp.Device,
		}
		// PID ファイルから PID を取得
		pidPath := filepath.Join(runDir, "embedding.pid")
		pid, _ := worker.ReadPID(pidPath)
		status.PID = pid
		return status
	}

	return EmbeddingWorkerStatus{Status: "not_running"}
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
		fmt.Fprintf(*w, "embedding: %s\n", output.Embedding.Status)
		if output.Embedding.Model != "" {
			fmt.Fprintf(*w, "embedding_model: %s\n", output.Embedding.Model)
		}
		if output.Embedding.PID > 0 {
			fmt.Fprintf(*w, "embedding_pid: %d\n", output.Embedding.PID)
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
