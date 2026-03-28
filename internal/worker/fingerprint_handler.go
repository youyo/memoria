package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/youyo/memoria/internal/fingerprint"
	"github.com/youyo/memoria/internal/project"
	"github.com/youyo/memoria/internal/queue"
)

// ProjectRefreshPayload は project_refresh ジョブの payload。
type ProjectRefreshPayload struct {
	ProjectID   string `json:"project_id"`
	ProjectRoot string `json:"project_root"`
}

// ProjectSimilarityRefreshPayload は project_similarity_refresh ジョブの payload。
type ProjectSimilarityRefreshPayload struct {
	ProjectID string `json:"project_id"`
}

// projectEmbedClient は fingerprint handler で使う embedding クライアントのインターフェース。
// ingest.EmbedClient と同一インターフェースだが import cycle を避けるため定義する。
type projectEmbedClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// ProjectRefreshHandler は project_refresh ジョブを処理するハンドラ。
type ProjectRefreshHandler struct {
	db     *sql.DB
	client projectEmbedClient // nil の場合は embedding スキップ
	model  string
	logf   func(string, ...any)
}

// NewProjectRefreshHandler は ProjectRefreshHandler を生成する。
// client が nil の場合は embedding をスキップする。
func NewProjectRefreshHandler(db *sql.DB, client projectEmbedClient, model string, logf func(string, ...any)) *ProjectRefreshHandler {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &ProjectRefreshHandler{db: db, client: client, model: model, logf: logf}
}

// Handle は project_refresh ジョブを処理する。
//
// 処理フロー:
// 1. payload JSON デコード
// 2. fingerprint.Generate() でフィンガープリント生成
// 3. projects テーブルを更新
// 4. embedding を取得して project_embeddings に保存
func (h *ProjectRefreshHandler) Handle(ctx context.Context, job *queue.Job) error {
	var payload ProjectRefreshPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode project_refresh payload: %w", err)
	}

	if payload.ProjectID == "" || payload.ProjectRoot == "" {
		return fmt.Errorf("project_refresh: project_id and project_root are required")
	}

	// フィンガープリント生成
	info, err := fingerprint.Generate(payload.ProjectRoot)
	if err != nil {
		return fmt.Errorf("generate fingerprint for %s: %w", payload.ProjectRoot, err)
	}

	// DB 更新
	sim := project.NewSimilarityManager(h.db)
	if err := sim.UpdateFingerprintDB(ctx, payload.ProjectID,
		info.FingerprintJSON, info.FingerprintText,
		info.PrimaryLanguage, info.ProjectKind,
	); err != nil {
		return fmt.Errorf("update fingerprint db: %w", err)
	}

	h.logf("memoria: project_refresh done: project_id=%s lang=%s kind=%s\n",
		payload.ProjectID, info.PrimaryLanguage, info.ProjectKind)

	// Embedding（client が設定されている場合のみ）
	if h.client == nil || info.FingerprintText == "" {
		return nil
	}

	vecs, err := h.client.Embed(ctx, []string{info.FingerprintText})
	if err != nil {
		// embedding 失敗は非致命的
		h.logf("memoria: project_refresh embedding skipped: %v\n", err)
		return nil
	}
	if len(vecs) == 0 {
		return nil
	}

	if err := sim.UpsertProjectEmbedding(ctx, payload.ProjectID, h.model, vecs[0]); err != nil {
		h.logf("memoria: project_refresh save embedding failed: %v\n", err)
		// 非致命的
	}

	return nil
}

// ProjectSimilarityRefreshHandler は project_similarity_refresh ジョブを処理するハンドラ。
type ProjectSimilarityRefreshHandler struct {
	db   *sql.DB
	logf func(string, ...any)
}

// NewProjectSimilarityRefreshHandler は ProjectSimilarityRefreshHandler を生成する。
func NewProjectSimilarityRefreshHandler(db *sql.DB, logf func(string, ...any)) *ProjectSimilarityRefreshHandler {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &ProjectSimilarityRefreshHandler{db: db, logf: logf}
}

// Handle は project_similarity_refresh ジョブを処理する。
//
// 処理フロー:
// 1. payload JSON デコード
// 2. 対象プロジェクトの embedding を取得
// 3. 全 project_embeddings を取得
// 4. コサイン類似度を計算
// 5. project_similarity テーブルに upsert
func (h *ProjectSimilarityRefreshHandler) Handle(ctx context.Context, job *queue.Job) error {
	var payload ProjectSimilarityRefreshPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode project_similarity_refresh payload: %w", err)
	}

	if payload.ProjectID == "" {
		return fmt.Errorf("project_similarity_refresh: project_id is required")
	}

	sim := project.NewSimilarityManager(h.db)

	// 対象プロジェクトの embedding を取得
	targetVec, found, err := sim.GetProjectEmbedding(ctx, payload.ProjectID)
	if err != nil {
		return fmt.Errorf("get target embedding: %w", err)
	}
	if !found {
		// embedding がない場合はスキップ（project_refresh が先に完了していない可能性）
		h.logf("memoria: project_similarity_refresh: no embedding for %s, skipping\n", payload.ProjectID)
		return nil
	}

	// 全 project_embeddings を取得
	allEmbs, err := sim.GetAllProjectEmbeddings(ctx)
	if err != nil {
		return fmt.Errorf("get all embeddings: %w", err)
	}

	// コサイン類似度を計算して保存
	for _, emb := range allEmbs {
		if emb.ProjectID == payload.ProjectID {
			continue // 自分自身はスキップ
		}

		similarity := cosineSimilarity(targetVec, emb.Vector)
		if err := sim.UpsertSimilarity(ctx, payload.ProjectID, emb.ProjectID, float64(similarity)); err != nil {
			h.logf("memoria: project_similarity_refresh upsert failed: project_id=%s similar=%s err=%v\n",
				payload.ProjectID, emb.ProjectID, err)
			// 非致命的: 継続
		}
	}

	h.logf("memoria: project_similarity_refresh done: project_id=%s compared_with=%d projects\n",
		payload.ProjectID, len(allEmbs)-1)
	return nil
}

// cosineSimilarity は 2 つのベクトルのコサイン類似度を計算する。
// 長さが異なる or 零ベクトルの場合は 0.0 を返す。
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
