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
	// uv run --project <python/ディレクトリ> で pyproject.toml を指定し、
	// venv 内の依存パッケージが使えるようにする。
	projectDir := filepath.Dir(workerScript)
	return []string{
		"run", "--project", projectDir, "python", workerScript,
		"--uds", sockPath,
		"--model", model,
		"--preload",
		"--idle-timeout", strconv.Itoa(idleTimeout),
	}
}

// findWorkerScript は python/worker.py のパスを以下の順序で探索する:
// 1. os.Args[0] の隣（make install 等でインストールされた場合）
// 2. os.Executable() の実パスの隣（シンボリックリンク経由の場合）
// 3. カレントディレクトリ（開発時）
func findWorkerScript() (string, error) {
	candidates := []string{
		filepath.Join(filepath.Dir(os.Args[0]), "python", "worker.py"),
	}

	if execPath, err := os.Executable(); err == nil {
		if realPath, err := filepath.EvalSymlinks(execPath); err == nil {
			binDir := filepath.Dir(realPath)
			candidates = append(candidates, filepath.Join(binDir, "python", "worker.py"))
			// Homebrew: bin/../libexec/python/worker.py
			candidates = append(candidates, filepath.Join(binDir, "..", "libexec", "python", "worker.py"))
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "python", "worker.py"))
	}

	// 重複を除きながら存在確認
	seen := make(map[string]bool)
	tried := make([]string, 0, len(candidates))
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		tried = append(tried, abs)
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	return "", fmt.Errorf(
		"embedding worker script (python/worker.py) not found; searched:\n  %s\nplace python/worker.py next to the memoria binary",
		joinLines(tried),
	)
}

// joinLines は文字列スライスを改行+インデントで結合する。
func joinLines(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += "\n  "
		}
		result += s
	}
	return result
}

// spawnEmbeddingWorker は embedding worker を uv run で spawn する。
func spawnEmbeddingWorker(cfg *config.Config) error {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("uv not found: install uv to enable embedding (https://docs.astral.sh/uv/)")
	}

	// python/worker.py を以下の順序で探索する:
	// 1. os.Args[0] の隣（make install 等でインストールされた場合）
	// 2. os.Executable() の実パスの隣（シンボリックリンク経由の場合）
	// 3. カレントディレクトリ（開発時: make build + ./bin/memoria）
	workerScript, err := findWorkerScript()
	if err != nil {
		return err
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
	venvPath := filepath.Join(config.StateDir(), "python-venv")
	env := append(os.Environ(), "UV_PROJECT_ENVIRONMENT="+venvPath)
	attr := &os.ProcAttr{
		Env:   env,
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
