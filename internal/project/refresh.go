package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/youyo/memoria/internal/queue"
)

// RefreshEnqueuer は project_refresh / project_similarity_refresh をキューに投入するためのインターフェース。
// テスト時にモックに差し替え可能。
type RefreshEnqueuer interface {
	Enqueue(ctx context.Context, jobType queue.JobType, payloadJSON string) (string, error)
}

// ProjectRefreshPayload は project_refresh ジョブのペイロード（worker パッケージと同一構造）。
// import cycle 回避のため project パッケージ内で再定義する。
type projectRefreshPayload struct {
	ProjectID   string `json:"project_id"`
	ProjectRoot string `json:"project_root"`
}

// projectSimilarityRefreshPayload は project_similarity_refresh ジョブのペイロード。
type projectSimilarityRefreshPayload struct {
	ProjectID string `json:"project_id"`
}

// EnsureFreshFingerprint はプロジェクトのフィンガープリントが TTL 内であるかを確認し、
// 切れている場合は project_refresh ジョブをキューに投入する（非同期）。
// hook から呼ばれる想定であり、エラーが発生しても hook は継続する（best effort）。
func EnsureFreshFingerprint(ctx context.Context, db *sql.DB, q RefreshEnqueuer, projectID, projectRoot string) {
	sim := NewSimilarityManager(db)
	if sim.IsFingerprintFresh(ctx, projectID, FingerprintTTL) {
		return
	}

	payload := projectRefreshPayload{
		ProjectID:   projectID,
		ProjectRoot: projectRoot,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		// JSON 変換失敗は非致命的
		return
	}

	// 非同期でキュー投入（エラーは無視）
	_, _ = q.Enqueue(ctx, queue.JobTypeProjectRefresh, string(payloadJSON))
}

// EnsureFreshSimilarity はプロジェクトの similarity キャッシュが TTL 内であるかを確認し、
// 切れている場合は project_similarity_refresh ジョブをキューに投入する（非同期）。
func EnsureFreshSimilarity(ctx context.Context, db *sql.DB, q RefreshEnqueuer, projectID string) {
	sim := NewSimilarityManager(db)
	if sim.IsSimilarityFresh(ctx, projectID, SimilarityTTL) {
		return
	}

	payload := projectSimilarityRefreshPayload{ProjectID: projectID}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return
	}

	_, _ = q.Enqueue(ctx, queue.JobTypeProjectSimilarityRefresh, string(payloadJSON))
}

// GetProjectRoot は projects テーブルから project_root を取得する。
// 存在しない場合は空文字列を返す。
func GetProjectRoot(ctx context.Context, db *sql.DB, projectID string) string {
	var root string
	if err := db.QueryRowContext(ctx, "SELECT project_root FROM projects WHERE project_id = ?", projectID).Scan(&root); err != nil {
		return ""
	}
	return root
}

// RefreshFingerprintSync はフィンガープリントを同期的に更新する（テスト用）。
// fingerprint.Generate を呼ばずに直接 DB を更新する。
func RefreshFingerprintSync(ctx context.Context, db *sql.DB, projectID, fingerprintJSON, fingerprintText, primaryLanguage, projectKind string) error {
	sim := NewSimilarityManager(db)
	return sim.UpdateFingerprintDB(ctx, projectID, fingerprintJSON, fingerprintText, primaryLanguage, projectKind)
}

// GetSimilarProjectsForHook は hook 用に類似プロジェクトを取得する。
// TTL が切れている場合は非同期で更新をキューに投入し、現在のキャッシュを返す。
func GetSimilarProjectsForHook(ctx context.Context, db *sql.DB, q RefreshEnqueuer, projectID string) map[string]float64 {
	// TTL チェック + 非同期更新
	EnsureFreshSimilarity(ctx, db, q, projectID)

	sim := NewSimilarityManager(db)
	result, err := sim.GetSimilarProjects(ctx, projectID)
	if err != nil {
		return nil
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// GetProjectInfo は projects テーブルからプロジェクト情報を取得する。
type ProjectInfo struct {
	ProjectID   string
	ProjectRoot string
	RepoName    string
}

// GetProjectInfo は projectID からプロジェクト情報を取得する。
func GetProjectInfoByID(ctx context.Context, db *sql.DB, projectID string) (*ProjectInfo, error) {
	const q = `SELECT project_id, project_root, COALESCE(repo_name, '') FROM projects WHERE project_id = ?`
	info := &ProjectInfo{}
	if err := db.QueryRowContext(ctx, q, projectID).Scan(&info.ProjectID, &info.ProjectRoot, &info.RepoName); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("project not found: %s", projectID)
		}
		return nil, fmt.Errorf("get project info: %w", err)
	}
	return info, nil
}

// SimilarityTTLDays は SimilarityTTL を日数で表現したもの（SQLite クエリ用）。
const SimilarityTTLDays = float64(SimilarityTTL) / float64(24*time.Hour)
