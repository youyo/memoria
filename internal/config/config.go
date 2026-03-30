package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config は memoria の設定を表す。
type Config struct {
	Log       LogConfig       `toml:"log"`
	Worker    WorkerConfig    `toml:"worker"`
	Embedding EmbeddingConfig `toml:"embedding"`
}

// LogConfig はログ関連の設定。
type LogConfig struct {
	Level string `toml:"level"` // debug, info, warn, error
}

// WorkerConfig は Worker 関連の設定。
// IngestIdleTimeout / EmbeddingIdleTimeout は後方互換のためフィールドを残すが、使用されない（常駐化）。
type WorkerConfig struct {
	IngestIdleTimeout    int `toml:"ingest_idle_timeout"`    // 廃止（後方互換のため残存）
	EmbeddingIdleTimeout int `toml:"embedding_idle_timeout"` // 廃止（後方互換のため残存）
}

// EmbeddingConfig は embedding 関連の設定。
type EmbeddingConfig struct {
	Model string `toml:"model"` // デフォルト "cl-nagoya/ruri-v3-30m"
}

// DefaultConfig はデフォルト設定を返す。
func DefaultConfig() *Config {
	return &Config{
		Log: LogConfig{
			Level: "info",
		},
		Worker: WorkerConfig{
			IngestIdleTimeout:    0,
			EmbeddingIdleTimeout: 0,
		},
		Embedding: EmbeddingConfig{
			Model: "cl-nagoya/ruri-v3-30m",
		},
	}
}

// Load は指定パスから config.toml を読み込む。
// ファイルが存在しない場合は DefaultConfig() を返す（エラーなし）。
// TOML のパースに失敗した場合はエラーを返す。
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat config file: %w", err)
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return cfg, nil
}

// Save は config を指定パスに TOML 形式で保存する。
// 一時ファイルに書き込み後 os.Rename() でアトミックに置換する。
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// 一時ファイルを同じディレクトリに作成（Rename を同一ファイルシステム内で行うため）
	tmp, err := os.CreateTemp(dir, ".config.toml.tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// エラー時は一時ファイルを削除
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if err = toml.NewEncoder(tmp).Encode(cfg); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode config: %w", err)
	}

	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err = os.Chmod(tmpPath, 0644); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename config file: %w", err)
	}

	return nil
}
