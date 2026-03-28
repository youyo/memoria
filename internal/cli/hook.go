package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/project"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/worker"
)

// HookCmd は Claude Code hook サブコマンドグループを定義する。
type HookCmd struct {
	SessionStart HookSessionStartCmd `cmd:"" name:"session-start" help:"セッション開始時の hook"`
	UserPrompt   HookUserPromptCmd   `cmd:"" name:"user-prompt" help:"ユーザープロンプト送信時の hook"`
	Stop         HookStopCmd         `cmd:"" help:"レスポンス完了時の hook"`
	SessionEnd   HookSessionEndCmd   `cmd:"" name:"session-end" help:"セッション終了時の hook"`
}

// HookSessionStartCmd は session-start hook コマンド。
type HookSessionStartCmd struct{}

// Run は session-start hook を実行する。
func (c *HookSessionStartCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// HookUserPromptCmd は user-prompt hook コマンド。
type HookUserPromptCmd struct{}

// Run は user-prompt hook を実行する。
func (c *HookUserPromptCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// HookStopInput は Stop hook の stdin JSON 入力。
type HookStopInput struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

// CheckpointPayload は checkpoint_ingest ジョブの payload。
type CheckpointPayload struct {
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
	Cwd        string    `json:"cwd"`
	Content    string    `json:"content"`
	CapturedAt time.Time `json:"captured_at"`
}

// HookStopCmd は stop hook コマンド。
type HookStopCmd struct{}

// Run は stop hook を実行する（os.Stdin から読み取る）。
func (c *HookStopCmd) Run(globals *Globals, w *io.Writer, database *db.DB, q *queue.Queue) error {
	return c.RunWithReader(globals, w, os.Stdin, database.SQL(), q)
}

// RunWithReader はテスト可能な実装。reader から stdin を読み取る。
func (c *HookStopCmd) RunWithReader(globals *Globals, w *io.Writer, reader io.Reader, sqlDB *sql.DB, q *queue.Queue) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	// 1. stdin デコード
	var input HookStopInput
	if err := json.NewDecoder(reader).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook stop: failed to decode stdin: %v\n", err)
		return nil // exit 0
	}

	// 2. Project 解決
	resolver := project.NewResolver(sqlDB)
	projectID, err := resolver.Resolve(ctx, input.Cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook stop: failed to resolve project: %v\n", err)
		// best effort: project_id は部分的に取得できている可能性があるので継続
		if projectID == "" {
			return nil // exit 0
		}
	}

	// 3. payload 構築 + enqueue
	payload := CheckpointPayload{
		SessionID:  input.SessionID,
		ProjectID:  projectID,
		Cwd:        input.Cwd,
		Content:    input.LastAssistantMessage,
		CapturedAt: time.Now().UTC(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook stop: failed to marshal payload: %v\n", err)
		return nil // exit 0
	}

	if _, err := q.Enqueue(ctx, queue.JobTypeCheckpointIngest, string(payloadJSON)); err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook stop: failed to enqueue: %v\n", err)
		return nil // exit 0
	}

	// 4. ensureWorker（M07 まではスタブ）
	worker.EnsureIngest(ctx)

	return nil
}

// HookSessionEndCmd は session-end hook コマンド。
type HookSessionEndCmd struct{}

// Run は session-end hook を実行する。
func (c *HookSessionEndCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
