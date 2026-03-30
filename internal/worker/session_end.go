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

// sessionEndPayload は session_end_ingest ジョブの payload（cli.SessionEndPayload と同一構造）。
// import cycle 回避のために worker パッケージ内で再定義する。
type sessionEndPayload struct {
	SessionID      string    `json:"session_id"`
	ProjectID      string    `json:"project_id"`
	Cwd            string    `json:"cwd"`
	TranscriptPath string    `json:"transcript_path"`
	Reason         string    `json:"reason"`
	EnqueuedAt     time.Time `json:"enqueued_at"`
}

// SessionEndHandler は session_end_ingest ジョブを処理するハンドラ。
type SessionEndHandler struct {
	db       *sql.DB
	embedder ingest.Embedder // nil の場合は embedding スキップ
	model    string
	logf     func(string, ...any)
}

// NewSessionEndHandler は embedding なし（後方互換）の SessionEndHandler を生成する。
func NewSessionEndHandler(db *sql.DB) *SessionEndHandler {
	return &SessionEndHandler{
		db:   db,
		logf: func(string, ...any) {},
	}
}

// NewSessionEndHandlerWithEmbedder は embedding 付きの SessionEndHandler を生成する。
func NewSessionEndHandlerWithEmbedder(db *sql.DB, embedder ingest.Embedder, model string, logf func(string, ...any)) *SessionEndHandler {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &SessionEndHandler{
		db:       db,
		embedder: embedder,
		model:    model,
		logf:     logf,
	}
}

// Handle は session_end_ingest ジョブを処理する。
//
// 処理フロー:
// 1. payload JSON デコード → sessionEndPayload
// 2. transcript_path のファイルが存在するか確認
// 3. transcript パーサーで []Turn に変換
// 4. sessions テーブルに UPSERT（ended_at も更新）
// 5. 既に turns が存在すれば早期リターン（冪等性）
// 6. turns テーブルに INSERT
// 7. chunker で turns → []RawChunk に変換
// 8. 各 chunk に enrichment を適用
// 9. content_hash で重複チェック → INSERT（ON CONFLICT DO NOTHING）
func (h *SessionEndHandler) Handle(ctx context.Context, job *queue.Job) error {
	// 1. payload デコード
	var payload sessionEndPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return fmt.Errorf("decode session_end payload: %w", err)
	}

	// 2. transcript_path のファイルが存在するか確認
	if payload.TranscriptPath == "" {
		return fmt.Errorf("transcript_path is empty")
	}

	// 3. transcript パーサーで []Turn に変換
	turns, err := ingest.ParseTranscript(payload.TranscriptPath)
	if err != nil {
		// ErrTranscriptNotFound はそのまま返す（Fail → retry）
		return err
	}

	// 4. sessions テーブルに UPSERT
	now := time.Now().UTC()
	endedAt := &now
	if err := ingest.UpsertSession(ctx, h.db, ingest.SessionRecord{
		SessionID:      payload.SessionID,
		ProjectID:      payload.ProjectID,
		Cwd:            payload.Cwd,
		TranscriptPath: payload.TranscriptPath,
		EndedAt:        endedAt,
	}); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	// 5. 既に turns が存在すれば早期リターン（冪等性）
	count, err := ingest.CountTurnsBySession(ctx, h.db, payload.SessionID)
	if err != nil {
		return fmt.Errorf("count turns by session: %w", err)
	}
	if count > 0 {
		// 既に処理済み → スキップ
		return nil
	}

	// ターンが 0 件の場合もスキップ（エラーなし）
	if len(turns) == 0 {
		return nil
	}

	// 6. turns テーブルに INSERT
	for _, turn := range turns {
		if strings.TrimSpace(turn.Content) == "" {
			continue
		}
		turnID := uuid.New().String()
		if err := ingest.InsertTurn(ctx, h.db, ingest.TurnRecord{
			TurnID:    turnID,
			SessionID: payload.SessionID,
			Role:      turn.Role,
			Content:   turn.Content,
			CreatedAt: turn.CreatedAt,
		}); err != nil {
			return fmt.Errorf("insert turn: %w", err)
		}
	}

	// 7. chunker で turns → []RawChunk に変換
	ingestTurns := make([]ingest.Turn, len(turns))
	copy(ingestTurns, turns)

	rawChunks := ingest.Chunk(ingest.ChunkInput{
		Turns:     ingestTurns,
		SessionID: payload.SessionID,
		ProjectID: payload.ProjectID,
	})

	// isolation_mode を確認（isolated プロジェクトでは scope を強制的に "project" に上書き）
	projectIsolated := project.IsIsolated(ctx, h.db, payload.ProjectID)

	// 8 & 9. 各 chunk に enrichment を適用して INSERT
	var insertedChunkIDs []string
	for _, rawChunk := range rawChunks {
		if strings.TrimSpace(rawChunk.Content) == "" {
			continue
		}

		enriched := ingest.Enrich(rawChunk.Content)

		// isolated プロジェクトでは scope を強制的に "project" に上書き
		if projectIsolated {
			enriched.Scope = "project"
		}
		contentHash := ingest.ContentHash(rawChunk.Content)
		chunkID := uuid.New().String()

		if err := ingest.InsertChunk(ctx, h.db, ingest.ChunkRecord{
			ChunkID:      chunkID,
			SessionID:    payload.SessionID,
			ProjectID:    payload.ProjectID,
			Content:      rawChunk.Content,
			Summary:      enriched.Summary,
			Kind:         enriched.Kind,
			Importance:   enriched.Importance,
			Scope:        enriched.Scope,
			KeywordsJSON: enriched.KeywordsJSON,
			ContentHash:  contentHash,
		}); err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
		insertedChunkIDs = append(insertedChunkIDs, chunkID)
	}

	// 10. embedding（embedder が設定されている場合のみ）
	if h.embedder != nil && len(insertedChunkIDs) > 0 {
		if err := h.embedder.EmbedChunks(ctx, h.db, insertedChunkIDs, h.model); err != nil {
			// embedding 失敗は非致命的: ingest は成功扱い（warn ログのみ）
			h.logf("memoria: session_end embedding skipped: %v\n", err)
		}
	}

	return nil
}
