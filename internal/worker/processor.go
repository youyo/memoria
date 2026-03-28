package worker

import (
	"context"
	"database/sql"

	"github.com/youyo/memoria/internal/queue"
)

// JobProcessor はジョブ種別ごとのハンドラを定義するインターフェース。
// テスト時はモックに差し替え可能。
type JobProcessor interface {
	HandleCheckpoint(ctx context.Context, job *queue.Job) error
	HandleSessionEnd(ctx context.Context, job *queue.Job) error
}

// DefaultJobProcessor は CheckpointHandler と SessionEndHandler を使った実装。
type DefaultJobProcessor struct {
	checkpoint *CheckpointHandler
	sessionEnd *SessionEndHandler
}

// NewDefaultJobProcessor は DefaultJobProcessor を生成する。
func NewDefaultJobProcessor(db *sql.DB) *DefaultJobProcessor {
	return &DefaultJobProcessor{
		checkpoint: NewCheckpointHandler(db),
		sessionEnd: NewSessionEndHandler(db),
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
