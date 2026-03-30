package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Worker.IngestIdleTimeout != 0 {
		t.Errorf("Worker.IngestIdleTimeout = %d, want 0 (deprecated)", cfg.Worker.IngestIdleTimeout)
	}
	if cfg.Worker.EmbeddingIdleTimeout != 0 {
		t.Errorf("Worker.EmbeddingIdleTimeout = %d, want 0 (deprecated)", cfg.Worker.EmbeddingIdleTimeout)
	}
	if cfg.Embedding.Model != "cl-nagoya/ruri-v3-30m" {
		t.Errorf("Embedding.Model = %q, want %q", cfg.Embedding.Model, "cl-nagoya/ruri-v3-30m")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Log.Level != cfg.Log.Level {
		t.Errorf("Log.Level = %q, want %q", loaded.Log.Level, cfg.Log.Level)
	}
	if loaded.Worker.IngestIdleTimeout != cfg.Worker.IngestIdleTimeout {
		t.Errorf("Worker.IngestIdleTimeout = %d, want %d", loaded.Worker.IngestIdleTimeout, cfg.Worker.IngestIdleTimeout)
	}
	if loaded.Worker.EmbeddingIdleTimeout != cfg.Worker.EmbeddingIdleTimeout {
		t.Errorf("Worker.EmbeddingIdleTimeout = %d, want %d", loaded.Worker.EmbeddingIdleTimeout, cfg.Worker.EmbeddingIdleTimeout)
	}
	if loaded.Embedding.Model != cfg.Embedding.Model {
		t.Errorf("Embedding.Model = %q, want %q", loaded.Embedding.Model, cfg.Embedding.Model)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load on nonexistent path should return defaults, got error: %v", err)
	}
	def := DefaultConfig()
	if cfg.Log.Level != def.Log.Level {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, def.Log.Level)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("not valid toml = [[["), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("Load with invalid TOML should return error, got nil")
	}
}

func TestSave_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	// 存在しないサブディレクトリ
	path := filepath.Join(dir, "subdir", "config.toml")
	cfg := DefaultConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save should create parent dir, got: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}
