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
	"github.com/youyo/memoria/internal/queue"
)

// checkpointPayload は checkpoint_ingest ジョブの payload（cli.CheckpointPayload と同一構造）。
// import cycle 回避のために worker パッケージ内で再定義する。
type checkpointPayload struct {
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
	Cwd        string    `json:"cwd"`
	Content    string    `json:"content"`
	CapturedAt time.Time `json:"captured_at"`
}

// CheckpointHandler は checkpoint_ingest ジョブを処理するハンドラ。
type CheckpointHandler struct {
	db *sql.DB
}

// NewCheckpointHandler は CheckpointHandler を生成する。
func NewCheckpointHandler(db *sql.DB) *CheckpointHandler {
	return &CheckpointHandler{db: db}
}

// Handle は checkpoint_ingest ジョブを処理する。
//
// 処理フロー:
// 1. payload JSON デコード → CheckpointPayload
// 2. sessions テーブルに UPSERT
// 3. content を chunk 化（単一コンテンツ、長文の場合は分割）
// 4. ヒューリスティック enrichment
// 5. content_hash で重複チェック
// 6. chunks テーブルに INSERT（ON CONFLICT DO NOTHING）
func (h *CheckpointHandler) Handle(ctx context.Context, job *queue.Job) error {
	// 1. payload デコード
	var payload checkpointPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode checkpoint payload: %w", err)
	}

	// 空コンテンツはスキップ
	if strings.TrimSpace(payload.Content) == "" {
		return nil
	}

	// 2. sessions テーブルに UPSERT
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

	// 3. content を chunk 化（長文の場合は分割）
	contentParts := ingest.SplitLongContent(payload.Content)

	for _, part := range contentParts {
		if strings.TrimSpace(part) == "" {
			continue
		}

		// 4. ヒューリスティック enrichment
		enriched := ingest.Enrich(part)

		// 5. content_hash
		contentHash := ingest.ContentHash(part)

		// 6. chunks テーブルに INSERT
		chunkID := uuid.New().String()
		if err := ingest.InsertChunk(ctx, h.db, ingest.ChunkRecord{
			ChunkID:      chunkID,
			SessionID:    payload.SessionID,
			ProjectID:    payload.ProjectID,
			Content:      part,
			Summary:      enriched.Summary,
			Kind:         enriched.Kind,
			Importance:   enriched.Importance,
			Scope:        enriched.Scope,
			KeywordsJSON: enriched.KeywordsJSON,
			ContentHash:  contentHash,
		}); err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}

	return nil
}
