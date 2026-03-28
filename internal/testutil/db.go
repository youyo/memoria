package testutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/youyo/memoria/internal/db"
)

// OpenTestDB はテスト用の SQLite DB を開き、マイグレーションを適用する。
// テスト終了時に自動でクローズされる。
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database.SQL()
}
