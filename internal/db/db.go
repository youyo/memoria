package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB は memoria の SQLite データベース接続を管理する。
type DB struct {
	sql  *sql.DB
	path string
}

// Open はデータディレクトリを作成し、SQLite データベースを開く。
// pragma を設定し、マイグレーションを実行してから返す。
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite は single writer 前提のため接続数を制限
	sqlDB.SetMaxOpenConns(1)

	if err := applyPragmas(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	if err := Migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &DB{sql: sqlDB, path: path}, nil
}

// applyPragmas は SQLite の pragma を設定する。
func applyPragmas(sqlDB *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA cache_size = -8000",
		"PRAGMA synchronous = NORMAL",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			return fmt.Errorf("exec %q: %w", p, err)
		}
	}
	return nil
}

// Close はデータベース接続を閉じる。
func (d *DB) Close() error {
	return d.sql.Close()
}

// Ping は DB 接続が生きているか確認する。
func (d *DB) Ping() error {
	return d.sql.Ping()
}

// SQL は生の *sql.DB を返す（テスト・高度なクエリ用）。
func (d *DB) SQL() *sql.DB {
	return d.sql
}

// Path は DB ファイルパスを返す。
func (d *DB) Path() string {
	return d.path
}
