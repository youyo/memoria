package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/embedding"
)

// embeddingHealthPollInterval は /health ポーリングの間隔。
const embeddingHealthPollInterval = 100 * time.Millisecond

// spawnEmbeddingWorkerFn は spawn 処理のテスト差し替えポイント。
var spawnEmbeddingWorkerFn = spawnEmbeddingWorker

// EnsureEmbedding は embedding worker が起動していることを確認する。
// 起動していなければ uv run で spawn し、health ポーリングで起動を待つ。
// embedding は hook の critical path に入らないため、失敗してもエラーログのみで return する。
func EnsureEmbedding(ctx context.Context, cfg *config.Config) error {
	client := embedding.New(config.SocketPath())
	return ensureEmbeddingWithClient(ctx, cfg, client)
}

// ensureEmbeddingWithClient はテスト可能な内部実装。client を外部から注入する。
func ensureEmbeddingWithClient(ctx context.Context, cfg *config.Config, client *embedding.Client) error {
	// 既に起動中か確認
	if _, err := client.Health(ctx); err == nil {
		return nil
	}

	// 起動していなければ spawn する
	if err := spawnEmbeddingWorkerFn(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "memoria: ensureEmbedding: spawn: %v\n", err)
		return fmt.Errorf("spawn embedding worker: %w", err)
	}

	return waitForEmbeddingHealthWithClient(ctx, client)
}

// waitForEmbeddingHealth は embedding worker の /health が返るまでポーリングする。
// タイムアウト or context キャンセルで error を返す。
func waitForEmbeddingHealth(ctx context.Context, socketPath string) error {
	client := embedding.New(socketPath)
	return waitForEmbeddingHealthWithClient(ctx, client)
}

// waitForEmbeddingHealthWithClient はテスト可能な内部実装。
func waitForEmbeddingHealthWithClient(ctx context.Context, client *embedding.Client) error {
	ticker := time.NewTicker(embeddingHealthPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("embedding worker health timeout: %w", ctx.Err())
		case <-ticker.C:
			if _, err := client.Health(ctx); err == nil {
				return nil
			}
		}
	}
}

// buildEmbeddingWorkerArgs は uv run の引数スライスを構築する。
// テスト容易性のために独立関数として抽出する。
func buildEmbeddingWorkerArgs(cfg *config.Config, sockPath, workerScript string) []string {
	idleTimeout := cfg.Worker.EmbeddingIdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 600
	}
	model := cfg.Embedding.Model
	if model == "" {
		model = "cl-nagoya/ruri-v3-30m"
	}
	return []string{
		"run", "python", workerScript,
		"--uds", sockPath,
		"--model", model,
		"--preload",
		"--idle-timeout", strconv.Itoa(idleTimeout),
	}
}

// spawnEmbeddingWorker は embedding worker を uv run で spawn する。
func spawnEmbeddingWorker(cfg *config.Config) error {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("uv not found: install uv to enable embedding (https://docs.astral.sh/uv/)")
	}

	// os.Args[0] の隣にある python/worker.py を探す
	execDir := filepath.Dir(os.Args[0])
	workerScript := filepath.Join(execDir, "python", "worker.py")
	if _, err := os.Stat(workerScript); err != nil {
		return fmt.Errorf("embedding worker script not found at %q: place python/worker.py next to the memoria binary", workerScript)
	}

	sockPath := config.SocketPath()

	// stale なソケットファイルを削除してから起動
	_ = os.Remove(sockPath)

	if err := os.MkdirAll(config.LogDir(), 0755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}
	if err := os.MkdirAll(config.RunDir(), 0755); err != nil {
		return fmt.Errorf("mkdir run dir: %w", err)
	}

	logPath := filepath.Join(config.LogDir(), "embedding.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// ログファイルが開けない場合は /dev/null にフォールバック
		logFile, _ = os.Open(os.DevNull)
	}
	defer logFile.Close()

	workerArgs := buildEmbeddingWorkerArgs(cfg, sockPath, workerScript)
	args := append([]string{uvPath}, workerArgs...)
	attr := &os.ProcAttr{
		Files: []*os.File{nil, logFile, logFile},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}

	proc, err := os.StartProcess(uvPath, args, attr)
	if err != nil {
		return fmt.Errorf("start embedding worker: %w", err)
	}

	pidPath := filepath.Join(config.RunDir(), "embedding.pid")
	_ = WritePID(pidPath, proc.Pid)

	return proc.Release()
}
