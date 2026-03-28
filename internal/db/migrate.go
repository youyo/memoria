package db

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate は未適用のマイグレーションを順番に実行する。
// 冪等性を持つ（適用済みは再実行しない）。
// NOTE: Go の fs.ReadDir はアルファベット順を保証する（Go 仕様）。
//       ファイル名ソートに依存しているため、このコメントを残すこと。
//       .sql 以外のファイル（.DS_Store 等）は拡張子チェックで除外する。
func Migrate(sqlDB *sql.DB) error {
	// 1. schema_migrations テーブルを作成（まだなければ）
	const createMigrations = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
)`
	if _, err := sqlDB.Exec(createMigrations); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 2. migrations/ ディレクトリから *.sql を昇順に列挙
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()

		// .sql 以外のファイルを除外
		if filepath.Ext(name) != ".sql" {
			continue
		}

		// 3. version 番号を抽出
		version, err := parseMigrationVersion(name)
		if err != nil {
			return fmt.Errorf("parse migration version %q: %w", name, err)
		}

		// schema_migrations に問い合わせ
		var count int
		if err := sqlDB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=?", version).Scan(&count); err != nil {
			return fmt.Errorf("query schema_migrations: %w", err)
		}
		if count > 0 {
			// 適用済みはスキップ
			continue
		}

		// 4. 未適用のみ実行
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration file %q: %w", name, err)
		}

		// トランザクションで各マイグレーションをアトミックに適用
		tx, err := sqlDB.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction for %q: %w", name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration %q: %w", name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations(version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %q: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %q: %w", name, err)
		}
	}

	return nil
}

// parseMigrationVersion はファイル名から version 番号を抽出する。
// 形式: NNNN_description.sql （例: 0001_initial.sql → 1）
// .sql 以外のファイルはエラーを返す（防御的チェック）。
func parseMigrationVersion(filename string) (int, error) {
	if filepath.Ext(filename) != ".sql" {
		return 0, fmt.Errorf("not a .sql file: %q", filename)
	}

	base := strings.TrimSuffix(filename, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid migration filename format: %q", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version number in %q: %w", filename, err)
	}

	return version, nil
}
