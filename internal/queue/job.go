package queue

import "time"

// JobType はジョブの種別を表す。
type JobType string

const (
	JobTypeCheckpointIngest         JobType = "checkpoint_ingest"
	JobTypeSessionEndIngest         JobType = "session_end_ingest"
	JobTypeProjectRefresh           JobType = "project_refresh"
	JobTypeProjectSimilarityRefresh JobType = "project_similarity_refresh"
	JobTypeUserPromptIngest         JobType = "user_prompt_ingest"
)

// Status はジョブの状態を表す。
type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Job は jobs テーブルの1行を表す。
type Job struct {
	ID           string
	Type         JobType
	PayloadJSON  string
	Status       Status
	RetryCount   int
	MaxRetries   int
	RunAfter     time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
	ErrorMessage *string
	CreatedAt    time.Time
}
