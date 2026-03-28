package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// FingerprintTTL はフィンガープリントの有効期間。
	FingerprintTTL = 24 * time.Hour
	// SimilarityTTL は類似プロジェクトキャッシュの有効期間。
	SimilarityTTL = 7 * 24 * time.Hour
)

// ProjectEmbedding は project_embeddings テーブルの1行を表す。
type ProjectEmbedding struct {
	ProjectID string
	Model     string
	Vector    []float32
}

// SimilarityManager は project_similarity / project_embeddings の操作を担う。
type SimilarityManager struct {
	db *sql.DB
}

// NewSimilarityManager は *sql.DB から SimilarityManager を生成する。
func NewSimilarityManager(db *sql.DB) *SimilarityManager {
	return &SimilarityManager{db: db}
}

// GetSimilarProjects は projectID の類似プロジェクトを取得し、
// similar_project_id -> similarity のマップを返す。
// データが存在しない場合は空マップを返す（エラーなし）。
func (m *SimilarityManager) GetSimilarProjects(ctx context.Context, projectID string) (map[string]float64, error) {
	const query = `
SELECT similar_project_id, similarity
FROM project_similarity
WHERE project_id = ?`

	rows, err := m.db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project_similarity: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var similarID string
		var similarity float64
		if err := rows.Scan(&similarID, &similarity); err != nil {
			return nil, fmt.Errorf("scan project_similarity: %w", err)
		}
		result[similarID] = similarity
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

// UpsertSimilarity は project_similarity テーブルにデータを upsert する。
// 既存の (project_id, similar_project_id) ペアがある場合は similarity と computed_at を更新する。
func (m *SimilarityManager) UpsertSimilarity(ctx context.Context, projectID, similarProjectID string, similarity float64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	const query = `
INSERT INTO project_similarity (project_id, similar_project_id, similarity, computed_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(project_id, similar_project_id) DO UPDATE SET
    similarity = excluded.similarity,
    computed_at = excluded.computed_at`

	if _, err := m.db.ExecContext(ctx, query, projectID, similarProjectID, similarity, now); err != nil {
		return fmt.Errorf("upsert project_similarity: %w", err)
	}
	return nil
}

// UpsertProjectEmbedding は project_embeddings テーブルにデータを upsert する。
// vector は JSON に変換して保存する。
func (m *SimilarityManager) UpsertProjectEmbedding(ctx context.Context, projectID, model string, vector []float32) error {
	jsonBytes, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	const query = `
INSERT INTO project_embeddings (project_id, model, embedding_json, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
    model = excluded.model,
    embedding_json = excluded.embedding_json,
    created_at = excluded.created_at`

	if _, err := m.db.ExecContext(ctx, query, projectID, model, string(jsonBytes), now); err != nil {
		return fmt.Errorf("upsert project_embeddings: %w", err)
	}
	return nil
}

// GetProjectEmbedding は projectID の embedding を取得する。
// found=false の場合は embedding が存在しない。
func (m *SimilarityManager) GetProjectEmbedding(ctx context.Context, projectID string) ([]float32, bool, error) {
	const query = `SELECT embedding_json FROM project_embeddings WHERE project_id = ?`
	var embJSON string
	err := m.db.QueryRowContext(ctx, query, projectID).Scan(&embJSON)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("query project_embeddings: %w", err)
	}
	var vec []float32
	if err := json.Unmarshal([]byte(embJSON), &vec); err != nil {
		return nil, false, fmt.Errorf("unmarshal embedding: %w", err)
	}
	return vec, true, nil
}

// GetAllProjectEmbeddings は全 project_embeddings を取得する。
// similarity 計算に使用する。
func (m *SimilarityManager) GetAllProjectEmbeddings(ctx context.Context) ([]ProjectEmbedding, error) {
	const query = `SELECT project_id, model, embedding_json FROM project_embeddings`
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query all project_embeddings: %w", err)
	}
	defer rows.Close()

	var result []ProjectEmbedding
	for rows.Next() {
		var pe ProjectEmbedding
		var embJSON string
		if err := rows.Scan(&pe.ProjectID, &pe.Model, &embJSON); err != nil {
			return nil, fmt.Errorf("scan project_embedding: %w", err)
		}
		if err := json.Unmarshal([]byte(embJSON), &pe.Vector); err != nil {
			// 破損したデータはスキップ
			continue
		}
		result = append(result, pe)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

// IsFingerprintFresh は projectID のフィンガープリントが TTL 内であるかを確認する。
// fingerprint_updated_at が NULL または TTL を超えている場合は false を返す。
func (m *SimilarityManager) IsFingerprintFresh(ctx context.Context, projectID string, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	const query = `SELECT fingerprint_updated_at FROM projects WHERE project_id = ?`
	var updatedAtStr sql.NullString
	if err := m.db.QueryRowContext(ctx, query, projectID).Scan(&updatedAtStr); err != nil {
		return false
	}
	if !updatedAtStr.Valid || updatedAtStr.String == "" {
		return false
	}
	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr.String)
	if err != nil {
		return false
	}
	return time.Since(updatedAt) < ttl
}

// IsSimilarityFresh は projectID の similarity キャッシュが TTL 内であるかを確認する。
// project_similarity に1件以上あり、computed_at が TTL 内であれば true を返す。
func (m *SimilarityManager) IsSimilarityFresh(ctx context.Context, projectID string, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	// 最新の computed_at を取得
	const query = `
SELECT MAX(computed_at) FROM project_similarity WHERE project_id = ?`
	var latestStr sql.NullString
	if err := m.db.QueryRowContext(ctx, query, projectID).Scan(&latestStr); err != nil {
		return false
	}
	if !latestStr.Valid || latestStr.String == "" {
		return false
	}
	latest, err := time.Parse(time.RFC3339, latestStr.String)
	if err != nil {
		return false
	}
	return time.Since(latest) < ttl
}

// UpdateFingerprintDB は projects テーブルの fingerprint 関連カラムを更新する。
func (m *SimilarityManager) UpdateFingerprintDB(ctx context.Context, projectID, fingerprintJSON, fingerprintText, primaryLanguage, projectKind string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	const query = `
UPDATE projects SET
    fingerprint_json = ?,
    fingerprint_text = ?,
    primary_language = ?,
    fingerprint_updated_at = ?
WHERE project_id = ?`

	if _, err := m.db.ExecContext(ctx, query,
		fingerprintJSON, fingerprintText, primaryLanguage, now, projectID,
	); err != nil {
		return fmt.Errorf("update fingerprint: %w", err)
	}
	return nil
}
