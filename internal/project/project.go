package project

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Resolver は cwd からプロジェクト情報を解決し、projects テーブルに記録する。
type Resolver struct {
	db *sql.DB
}

// NewResolver は *sql.DB から Resolver を作成する。
func NewResolver(db *sql.DB) *Resolver {
	return &Resolver{db: db}
}

// Resolve は cwd を受け取り、git root を優先してプロジェクトを解決する。
// projects テーブルに UPSERT し、project_id を返す。
// エラー時も project_id（cwd ベース）を返す（best effort）。
func (r *Resolver) Resolve(ctx context.Context, cwd string) (string, error) {
	rootPath := r.gitRoot(ctx, cwd)
	projectID := generateProjectID(rootPath)
	repoName := filepath.Base(rootPath)
	now := time.Now().UTC().Format(time.RFC3339)

	const upsertSQL = `
INSERT INTO projects (project_id, project_root, repo_name, last_seen_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
    last_seen_at = excluded.last_seen_at`

	_, err := r.db.ExecContext(ctx, upsertSQL, projectID, rootPath, repoName, now)
	if err != nil {
		return projectID, fmt.Errorf("upsert project: %w", err)
	}
	return projectID, nil
}

// gitRoot は exec.CommandContext で git root を取得する。
// 失敗時は cwd をそのまま返す。
// 戻り値には filepath.EvalSymlinks() + filepath.Clean() を適用して
// symlink やパスの揺れを正規化する。cwd フォールバック時にも同様に正規化する。
func (r *Resolver) gitRoot(ctx context.Context, cwd string) string {
	// git rev-parse に 200ms タイムアウトを設定
	gitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(gitCtx, "git", "-C", cwd, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
			return filepath.Clean(resolved)
		}
		return filepath.Clean(cwd)
	}
	raw := strings.TrimSpace(string(out))
	if resolved, err := filepath.EvalSymlinks(raw); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(raw)
}

// generateProjectID は rootPath の SHA-256 ハッシュ先頭 16 文字を返す。
func generateProjectID(rootPath string) string {
	h := sha256.Sum256([]byte(rootPath))
	return hex.EncodeToString(h[:])[:16]
}
