package project_test

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/project"
)

// openTestDB はテスト用のインメモリ SQLite DB を開き、マイグレーションを適用する。
func openTestDB(t *testing.T) *sql.DB {
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

func TestResolveProject_GitRoot(t *testing.T) {
	// t.TempDir() に git init した上で ResolveProject を呼ぶ
	tmpDir := t.TempDir()
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Skipf("git init failed (git not available?): %v", err)
	}

	sqlDB := openTestDB(t)
	r := project.NewResolver(sqlDB)

	ctx := context.Background()
	projectID, err := r.Resolve(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// project_id は 16 文字の hex であることを確認
	if len(projectID) != 16 {
		t.Errorf("project_id length = %d, want 16", len(projectID))
	}
	for _, c := range projectID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("project_id %q contains non-hex char %q", projectID, c)
			break
		}
	}

	// projects テーブルに行が存在することを確認
	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM projects WHERE project_id = ?", projectID).Scan(&count); err != nil {
		t.Fatalf("query projects: %v", err)
	}
	if count != 1 {
		t.Errorf("projects count = %d, want 1", count)
	}
}

func TestResolveProject_NotGitRepo(t *testing.T) {
	// git repo ではない tmpDir を cwd として渡す
	tmpDir := t.TempDir()

	sqlDB := openTestDB(t)
	r := project.NewResolver(sqlDB)

	ctx := context.Background()
	projectID, err := r.Resolve(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// project_id が 16 文字の hex であることを確認（cwd フォールバック）
	if len(projectID) != 16 {
		t.Errorf("project_id length = %d, want 16", len(projectID))
	}
}

func TestResolveProject_Upsert(t *testing.T) {
	// 同じ rootPath で2回 ResolveProject を呼ぶ
	tmpDir := t.TempDir()

	sqlDB := openTestDB(t)
	r := project.NewResolver(sqlDB)
	ctx := context.Background()

	id1, err := r.Resolve(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Resolve 1st: %v", err)
	}

	// 少し時間を空けて last_seen_at を変化させる
	time.Sleep(1 * time.Second)

	id2, err := r.Resolve(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Resolve 2nd: %v", err)
	}

	// project_id は同じであることを確認
	if id1 != id2 {
		t.Errorf("project_id changed: %q → %q", id1, id2)
	}

	// projects テーブルの行数が1件のままであることを確認
	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		t.Fatalf("query projects: %v", err)
	}
	if count != 1 {
		t.Errorf("projects count = %d, want 1 (should not duplicate)", count)
	}
}

func TestResolveProject_CancelledContext(t *testing.T) {
	// 既にキャンセル済みの context を渡す
	tmpDir := t.TempDir()

	sqlDB := openTestDB(t)
	r := project.NewResolver(sqlDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 即座にキャンセル

	_, err := r.Resolve(ctx, tmpDir)
	// エラーが返ることを確認（DB クエリは実行されない）
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestResolveProject_SymlinkPath(t *testing.T) {
	// symlink 経由の cwd で project_id が正しく生成されることを確認
	tmpDir := t.TempDir()
	symlinkDir := filepath.Join(os.TempDir(), "memoria_test_symlink_"+t.Name())
	if err := os.Symlink(tmpDir, symlinkDir); err != nil {
		t.Skipf("symlink creation failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(symlinkDir) })

	sqlDB := openTestDB(t)
	r := project.NewResolver(sqlDB)
	ctx := context.Background()

	// symlink パスで Resolve
	idViaSymlink, err := r.Resolve(ctx, symlinkDir)
	if err != nil {
		t.Fatalf("Resolve via symlink: %v", err)
	}

	// 実パスで Resolve
	realPath, _ := filepath.EvalSymlinks(tmpDir)
	r2 := project.NewResolver(sqlDB)
	idViaReal, err := r2.Resolve(ctx, realPath)
	if err != nil {
		t.Fatalf("Resolve via real path: %v", err)
	}

	// symlink と実パスで同じ project_id が生成されることを確認
	if idViaSymlink != idViaReal {
		t.Errorf("project_id via symlink %q != via real path %q", idViaSymlink, idViaReal)
	}
}
