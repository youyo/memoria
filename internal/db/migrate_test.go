package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openRawDB はテスト用に pragma なし・migrate なしの生の *sql.DB を開く。
func openRawDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "raw_test.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return sqlDB
}

func TestMigrate_AppliesInitial(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Open 時に自動実行済みのはずだが、テーブルの存在を確認
	tables := []string{
		"projects", "project_aliases", "project_similarity",
		"sessions", "turns", "chunks", "chunks_fts",
		"chunk_embeddings", "project_embeddings",
		"jobs", "worker_leases", "worker_probes",
		"schema_migrations",
	}
	for _, tbl := range tables {
		var name string
		err := d.SQL().QueryRow(
			"SELECT name FROM sqlite_master WHERE type IN ('table','shadow') AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", tbl, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	dir := t.TempDir()
	sqlDB := openRawDB(t, dir)
	defer sqlDB.Close()

	if err := Migrate(sqlDB); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(sqlDB); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrate_SchemaVersion(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var version int
	err = d.SQL().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestMigrate_FTSTrigger(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// project を先に作成（FK制約）
	_, err = d.SQL().Exec(`INSERT INTO projects(project_id, project_root) VALUES ('p1', '/tmp/test')`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	_, err = d.SQL().Exec(`
        INSERT INTO chunks(chunk_id, project_id, content, kind, content_hash)
        VALUES ('c1', 'p1', 'error handling design in Go', 'decision', 'hash1')
    `)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	// NOTE: FTS5 のデフォルトトークナイザー（unicode61）は日本語の単語境界を認識しない。
	// 英語ワードによる FTS 動作確認。日本語対応は M12 でトークナイザーを調整する。
	var count int
	if err := d.SQL().QueryRow("SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'error'").Scan(&count); err != nil {
		t.Fatalf("query fts: %v", err)
	}
	if count == 0 {
		t.Error("FTS trigger did not sync chunk content")
	}
}

func TestParseMigrationVersion(t *testing.T) {
	tests := []struct {
		filename string
		want     int
		wantErr  bool
	}{
		{"0001_initial.sql", 1, false},
		{"0002_add_xxx.sql", 2, false},
		{"0000_init.sql", 0, false},   // ゼロは version 0 として許容
		{"abc_foo.sql", 0, true},      // 非数字プレフィックス → エラー
		{"0001", 0, true},             // 拡張子なし → エラー（.sql フィルタで除外済みのはずだが防御的に）
	}
	for _, tt := range tests {
		got, err := parseMigrationVersion(tt.filename)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseMigrationVersion(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseMigrationVersion(%q) = %d, want %d", tt.filename, got, tt.want)
		}
	}
}
