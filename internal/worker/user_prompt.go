package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/project"
	"github.com/youyo/memoria/internal/queue"
)

// userPromptPayload は user_prompt_ingest ジョブの payload（cli.UserPromptPayload と同一構造）。
// import cycle 回避のために worker パッケージ内で再定義する。
type userPromptPayload struct {
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
	Cwd        string    `json:"cwd"`
	Prompt     string    `json:"prompt"`
	CapturedAt time.Time `json:"captured_at"`
}

// UserPromptHandler は user_prompt_ingest ジョブを処理するハンドラ。
type UserPromptHandler struct {
	db       *sql.DB
	embedder ingest.Embedder // nil の場合は embedding スキップ
	model    string
	logf     func(string, ...any)
}

// NewUserPromptHandler は embedding なし（後方互換）の UserPromptHandler を生成する。
func NewUserPromptHandler(db *sql.DB) *UserPromptHandler {
	return &UserPromptHandler{
		db:   db,
		logf: func(string, ...any) {},
	}
}

// NewUserPromptHandlerWithEmbedder は embedding 付きの UserPromptHandler を生成する。
func NewUserPromptHandlerWithEmbedder(db *sql.DB, embedder ingest.Embedder, model string, logf func(string, ...any)) *UserPromptHandler {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &UserPromptHandler{
		db:       db,
		embedder: embedder,
		model:    model,
		logf:     logf,
	}
}

// Handle は user_prompt_ingest ジョブを処理する。
//
// 処理フロー:
// 1. payload JSON デコード → userPromptPayload
// 2. 空プロンプトはスキップ
// 3. sessions テーブルに UPSERT
// 4. ヒューリスティック enrichment
// 5. isolated チェック → scope 上書き
// 6. content_hash で重複チェック
// 7. chunks テーブルに INSERT（ON CONFLICT DO NOTHING）
// 8. embedding（embedder が設定されている場合のみ）
func (h *UserPromptHandler) Handle(ctx context.Context, job *queue.Job) error {
	// 1. payload デコード
	var payload userPromptPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode user_prompt payload: %w", err)
	}

	// 2. 空プロンプトはスキップ
	if strings.TrimSpace(payload.Prompt) == "" {
		return nil
	}

	// isolation_mode を確認（isolated プロジェクトでは scope を強制的に "project" に上書き）
	projectIsolated := project.IsIsolated(ctx, h.db, payload.ProjectID)

	// 3. sessions テーブルに UPSERT
	startedAt := payload.CapturedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if err := ingest.UpsertSession(ctx, h.db, ingest.SessionRecord{
		SessionID: payload.SessionID,
		ProjectID: payload.ProjectID,
		Cwd:       payload.Cwd,
		StartedAt: startedAt,
	}); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	// 4. ヒューリスティック enrichment
	enriched := ingest.Enrich(payload.Prompt)

	// 5. isolated プロジェクトでは scope を強制的に "project" に上書き
	if projectIsolated {
		enriched.Scope = "project"
	}

	// 6. content_hash
	contentHash := ingest.ContentHash(payload.Prompt)

	// 7. chunks テーブルに INSERT
	chunkID := uuid.New().String()
	if err := ingest.InsertChunk(ctx, h.db, ingest.ChunkRecord{
		ChunkID:      chunkID,
		SessionID:    payload.SessionID,
		ProjectID:    payload.ProjectID,
		Content:      payload.Prompt,
		Summary:      enriched.Summary,
		Kind:         enriched.Kind,
		Importance:   enriched.Importance,
		Scope:        enriched.Scope,
		KeywordsJSON: enriched.KeywordsJSON,
		ContentHash:  contentHash,
	}); err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}

	// 8. embedding（embedder が設定されている場合のみ）
	if h.embedder != nil {
		if err := h.embedder.EmbedChunks(ctx, h.db, []string{chunkID}, h.model); err != nil {
			// embedding 失敗は非致命的: ingest は成功扱い（warn ログのみ）
			h.logf("memoria: user_prompt embedding skipped: %v\n", err)
		}
	}

	return nil
}
