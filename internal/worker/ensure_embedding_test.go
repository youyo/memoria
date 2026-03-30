package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/embedding"
)

// configWithHealthyEmbedding は /health に ok を返す mock client を使う config helper。
func configWithHealthyEmbedding(t *testing.T) (*config.Config, *embedding.Client) {
	t.Helper()
	rt := &mockRT{fn: func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/health" {
			b, _ := json.Marshal(embedding.HealthResponse{Status: "ok", Model: "test-model", Dimensions: 256, Device: "cpu"})
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(b))}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}}
	client := embedding.NewWithHTTPClient("http://localhost", &http.Client{Transport: rt})
	cfg := config.DefaultConfig()
	return cfg, client
}

// configWithDeadEmbedding は /health に失敗する mock client。
func configWithDeadEmbedding(t *testing.T) (*config.Config, *embedding.Client) {
	t.Helper()
	rt := &mockRT{fn: func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	client := embedding.NewWithHTTPClient("http://localhost", &http.Client{Transport: rt})
	cfg := config.DefaultConfig()
	return cfg, client
}

// configWithEventuallyHealthyEmbedding は n 回失敗した後に成功する mock client。
func configWithEventuallyHealthyEmbedding(t *testing.T, failCount int) (*config.Config, *embedding.Client) {
	t.Helper()
	attempts := 0
	rt := &mockRT{fn: func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/health" {
			attempts++
			if attempts <= failCount {
				return nil, errors.New("not ready yet")
			}
			b, _ := json.Marshal(embedding.HealthResponse{Status: "ok", Model: "test-model", Dimensions: 256, Device: "cpu"})
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(b))}, nil
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}}
	client := embedding.NewWithHTTPClient("http://localhost", &http.Client{Transport: rt})
	cfg := config.DefaultConfig()
	return cfg, client
}

type mockRT struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (m *mockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

func TestEnsureEmbedding_AlreadyRunning(t *testing.T) {
	cfg, client := configWithHealthyEmbedding(t)

	// 既に起動中の場合は spawn を呼ばない
	spawnCalled := false
	origSpawn := spawnEmbeddingWorkerFn
	defer func() { spawnEmbeddingWorkerFn = origSpawn }()
	spawnEmbeddingWorkerFn = func(_ *config.Config) error {
		spawnCalled = true
		return nil
	}

	err := ensureEmbeddingWithClient(context.Background(), cfg, client)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if spawnCalled {
		t.Error("expected spawnEmbeddingWorker NOT to be called when already running")
	}
}

func TestEnsureEmbedding_NotRunning_SpawnCalled(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "notexist.sock")
	_ = sockPath
	cfg, client := configWithDeadEmbedding(t)

	spawnCalled := false
	origSpawn := spawnEmbeddingWorkerFn
	defer func() { spawnEmbeddingWorkerFn = origSpawn }()
	spawnEmbeddingWorkerFn = func(_ *config.Config) error {
		spawnCalled = true
		return errors.New("mock: uv not available in test")
	}

	_ = ensureEmbeddingWithClient(context.Background(), cfg, client)
	if !spawnCalled {
		t.Error("expected spawnEmbeddingWorker to be called")
	}
}

func TestWaitForEmbeddingHealth_Success(t *testing.T) {
	_, client := configWithEventuallyHealthyEmbedding(t, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := waitForEmbeddingHealthWithClient(ctx, client)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForEmbeddingHealth_Timeout(t *testing.T) {
	_, client := configWithDeadEmbedding(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := waitForEmbeddingHealthWithClient(ctx, client)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestBuildEmbeddingWorkerArgs(t *testing.T) {
	cfg := &config.Config{
		Worker:    config.WorkerConfig{},
		Embedding: config.EmbeddingConfig{Model: "cl-nagoya/ruri-v3-30m"},
	}
	args := buildEmbeddingWorkerArgs(cfg, "/tmp/test.sock", "/path/to/worker.py")
	if !slices.Contains(args, "--preload") {
		t.Error("expected --preload in args")
	}
	if !slices.Contains(args, "--uds") {
		t.Error("expected --uds in args")
	}
	if !slices.Contains(args, "--model") {
		t.Error("expected --model in args")
	}
	// --idle-timeout は廃止されたため args に含まれないことを確認
	if slices.Contains(args, "--idle-timeout") {
		t.Error("expected --idle-timeout NOT to be in args (deprecated)")
	}
}

func TestBuildEmbeddingWorkerArgs_Defaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Embedding.Model = ""
	args := buildEmbeddingWorkerArgs(cfg, "/tmp/test.sock", "/path/to/worker.py")
	if !slices.Contains(args, "cl-nagoya/ruri-v3-30m") {
		t.Error("expected default model in args")
	}
	// --idle-timeout は廃止されたため args に含まれないことを確認
	if slices.Contains(args, "--idle-timeout") {
		t.Error("expected --idle-timeout NOT to be in args (deprecated)")
	}
}
