package worker

import (
	"context"
	"database/sql"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/embedding"
	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/queue"
)

// JobProcessor はジョブ種別ごとのハンドラを定義するインターフェース。
// テスト時はモックに差し替え可能。
type JobProcessor interface {
	HandleCheckpoint(ctx context.Context, job *queue.Job) error
	HandleSessionEnd(ctx context.Context, job *queue.Job) error
	HandleProjectRefresh(ctx context.Context, job *queue.Job) error
	HandleProjectSimilarityRefresh(ctx context.Context, job *queue.Job) error
	HandleUserPrompt(ctx context.Context, job *queue.Job) error
}

// DefaultJobProcessor は CheckpointHandler と SessionEndHandler を使った実装。
type DefaultJobProcessor struct {
	checkpoint        *CheckpointHandler
	sessionEnd        *SessionEndHandler
	projectRefresh    *ProjectRefreshHandler
	similarityRefresh *ProjectSimilarityRefreshHandler
	userPrompt        *UserPromptHandler
}

// NewDefaultJobProcessor は embedding なし（後方互換）の DefaultJobProcessor を生成する。
// テストやシンプルな用途向け。embedding が必要な場合は NewDefaultJobProcessorWithEmbedding を使う。
func NewDefaultJobProcessor(db *sql.DB) *DefaultJobProcessor {
	return &DefaultJobProcessor{
		checkpoint:        NewCheckpointHandler(db),
		sessionEnd:        NewSessionEndHandler(db),
		projectRefresh:    NewProjectRefreshHandler(db, nil, "", nil),
		similarityRefresh: NewProjectSimilarityRefreshHandler(db, nil),
		userPrompt:        NewUserPromptHandler(db),
	}
}

// NewDefaultJobProcessorWithEmbedding は embedding 付きの DefaultJobProcessor を生成する。
// embedding worker が UDS 経由で稼働していることを期待する。
// worker が未起動の場合、EmbedChunks がエラーを返すが ingest 自体は成功扱い（warn ログのみ）。
func NewDefaultJobProcessorWithEmbedding(db *sql.DB, cfg *config.Config, logf func(string, ...any)) *DefaultJobProcessor {
	model := cfg.Embedding.Model
	if model == "" {
		model = "cl-nagoya/ruri-v3-30m"
	}

	embeddingClient := embedding.New(config.SocketPath())
	embedder := ingest.NewChunkEmbedder(embeddingClient)

	return &DefaultJobProcessor{
		checkpoint:        NewCheckpointHandlerWithEmbedder(db, embedder, model, logf),
		sessionEnd:        NewSessionEndHandlerWithEmbedder(db, embedder, model, logf),
		projectRefresh:    NewProjectRefreshHandler(db, embeddingClient, model, logf),
		similarityRefresh: NewProjectSimilarityRefreshHandler(db, logf),
		userPrompt:        NewUserPromptHandlerWithEmbedder(db, embedder, model, logf),
	}
}

// HandleCheckpoint は checkpoint_ingest ジョブを処理する。
func (p *DefaultJobProcessor) HandleCheckpoint(ctx context.Context, job *queue.Job) error {
	return p.checkpoint.Handle(ctx, job)
}

// HandleSessionEnd は session_end_ingest ジョブを処理する。
func (p *DefaultJobProcessor) HandleSessionEnd(ctx context.Context, job *queue.Job) error {
	return p.sessionEnd.Handle(ctx, job)
}

// HandleProjectRefresh は project_refresh ジョブを処理する。
func (p *DefaultJobProcessor) HandleProjectRefresh(ctx context.Context, job *queue.Job) error {
	return p.projectRefresh.Handle(ctx, job)
}

// HandleProjectSimilarityRefresh は project_similarity_refresh ジョブを処理する。
func (p *DefaultJobProcessor) HandleProjectSimilarityRefresh(ctx context.Context, job *queue.Job) error {
	return p.similarityRefresh.Handle(ctx, job)
}

// HandleUserPrompt は user_prompt_ingest ジョブを処理する。
func (p *DefaultJobProcessor) HandleUserPrompt(ctx context.Context, job *queue.Job) error {
	return p.userPrompt.Handle(ctx, job)
}
