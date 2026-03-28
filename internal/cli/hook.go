package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	cfg_pkg "github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/embedding"
	"github.com/youyo/memoria/internal/project"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/retrieval"
	"github.com/youyo/memoria/internal/worker"
)

// HookCmd は Claude Code hook サブコマンドグループを定義する。
type HookCmd struct {
	SessionStart HookSessionStartCmd `cmd:"" name:"session-start" help:"セッション開始時の hook"`
	UserPrompt   HookUserPromptCmd   `cmd:"" name:"user-prompt" help:"ユーザープロンプト送信時の hook"`
	Stop         HookStopCmd         `cmd:"" help:"レスポンス完了時の hook"`
	SessionEnd   HookSessionEndCmd   `cmd:"" name:"session-end" help:"セッション終了時の hook"`
}

// HookOutput は SessionStart / UserPromptSubmit hook の JSON 出力。
type HookOutput struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

// HookSpecificOutput は hook 固有の出力。
type HookSpecificOutput struct {
	HookEventName    string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// writeHookOutput は HookOutput を w に JSON として書き出す。
func writeHookOutput(w io.Writer, eventName, additionalContext string) error {
	out := HookOutput{
		HookSpecificOutput: HookSpecificOutput{
			HookEventName:    eventName,
			AdditionalContext: additionalContext,
		},
	}
	enc := json.NewEncoder(w)
	return enc.Encode(out)
}

// HookSessionStartInput は SessionStart hook の stdin JSON 入力。
type HookSessionStartInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	Source         string `json:"source"`
}

// HookSessionStartCmd は session-start hook コマンド。
type HookSessionStartCmd struct{}

// Run は session-start hook を実行する（os.Stdin から読み取る）。
func (c *HookSessionStartCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return writeHookOutput(*w, "SessionStart", "")
	}
	return c.RunWithReader(globals, *w, os.Stdin, database.SQL(), nil)
}

// RunWithReader はテスト可能な実装。
// embedder が nil の場合は FTS only モードで動作する。
func (c *HookSessionStartCmd) RunWithReader(globals *Globals, w io.Writer, reader io.Reader, sqlDB *sql.DB, embedder retrieval.Embedder) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var input HookSessionStartInput
	if err := json.NewDecoder(reader).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-start: failed to decode stdin: %v\n", err)
		// 失敗時も空の additionalContext を返す
		return writeHookOutput(w, "SessionStart", "")
	}

	// Project 解決
	resolver := project.NewResolver(sqlDB)
	projectID, err := resolver.Resolve(ctx, input.Cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-start: failed to resolve project: %v\n", err)
		if projectID == "" {
			return writeHookOutput(w, "SessionStart", "")
		}
	}

	// M13: フィンガープリント TTL チェック + 非同期更新
	q := queue.New(sqlDB)
	project.EnsureFreshFingerprint(ctx, sqlDB, q, projectID, input.Cwd)

	// M13: similar projects を取得（TTL 切れ時は非同期更新をキューに投入）
	similarProjects := project.GetSimilarProjectsForHook(ctx, sqlDB, q, projectID)

	// retrieval
	r := retrieval.New(sqlDB, embedder)
	results, err := r.SessionStart(ctx, projectID, similarProjects, 4)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-start: retrieval error: %v\n", err)
		return writeHookOutput(w, "SessionStart", "")
	}

	additionalContext := retrieval.FormatContext(results)
	return writeHookOutput(w, "SessionStart", additionalContext)
}

// HookUserPromptInput は UserPromptSubmit hook の stdin JSON 入力。
type HookUserPromptInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Prompt    string `json:"prompt"`
}

// HookUserPromptCmd は user-prompt hook コマンド。
type HookUserPromptCmd struct{}

// Run は user-prompt hook を実行する（os.Stdin から読み取る）。
func (c *HookUserPromptCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB, cfg *cfg_pkg.Config) error {
	database, err := lazyDB.Get()
	if err != nil {
		return writeHookOutput(*w, "UserPromptSubmit", "")
	}
	embedder := newEmbedderFromConfig(cfg)
	return c.RunWithReader(globals, *w, os.Stdin, database.SQL(), embedder)
}

// RunWithReader はテスト可能な実装。
// embedder が nil の場合は FTS only モードで動作する。
func (c *HookUserPromptCmd) RunWithReader(globals *Globals, w io.Writer, reader io.Reader, sqlDB *sql.DB, embedder retrieval.Embedder) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var input HookUserPromptInput
	if err := json.NewDecoder(reader).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook user-prompt: failed to decode stdin: %v\n", err)
		return writeHookOutput(w, "UserPromptSubmit", "")
	}

	// Project 解決
	resolver := project.NewResolver(sqlDB)
	projectID, err := resolver.Resolve(ctx, input.Cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook user-prompt: failed to resolve project: %v\n", err)
		if projectID == "" {
			return writeHookOutput(w, "UserPromptSubmit", "")
		}
	}

	// M13: similar projects を取得（TTL 切れ時は非同期更新をキューに投入）
	q := queue.New(sqlDB)
	similarProjects := project.GetSimilarProjectsForHook(ctx, sqlDB, q, projectID)

	// retrieval
	r := retrieval.New(sqlDB, embedder)
	results, err := r.UserPrompt(ctx, projectID, similarProjects, input.Prompt, 5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook user-prompt: retrieval error: %v\n", err)
		return writeHookOutput(w, "UserPromptSubmit", "")
	}

	additionalContext := retrieval.FormatContext(results)
	return writeHookOutput(w, "UserPromptSubmit", additionalContext)
}

// newEmbedderFromConfig は設定から embedding.Client を生成する。
// embedding worker が利用できない場合は nil を返す（degraded mode）。
func newEmbedderFromConfig(cfg *cfg_pkg.Config) retrieval.Embedder {
	// UDS パスを解決
	socketPath := embeddingSocketPath()
	if socketPath == "" {
		return nil
	}
	return embedding.New(socketPath)
}

// embeddingSocketPath は embedding worker の UDS パスを返す。
// 環境変数 MEMORIA_EMBEDDING_SOCK が設定されている場合はそれを優先する。
func embeddingSocketPath() string {
	if v := os.Getenv("MEMORIA_EMBEDDING_SOCK"); v != "" {
		return v
	}
	return cfg_pkg.SocketPath()
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
func (c *HookStopCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return nil // DB エラー時は exit 0（hook は block しない）
	}
	q := queue.New(database.SQL())
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

// HookSessionEndInput は SessionEnd hook の stdin JSON 入力。
type HookSessionEndInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	Reason         string `json:"reason"`
}

// SessionEndPayload は session_end_ingest ジョブの payload。
type SessionEndPayload struct {
	SessionID      string    `json:"session_id"`
	ProjectID      string    `json:"project_id"`
	Cwd            string    `json:"cwd"`
	TranscriptPath string    `json:"transcript_path"`
	Reason         string    `json:"reason"`
	EnqueuedAt     time.Time `json:"enqueued_at"`
}

// HookSessionEndCmd は session-end hook コマンド。
type HookSessionEndCmd struct{}

// Run は session-end hook を実行する（os.Stdin から読み取る）。
func (c *HookSessionEndCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return nil // DB エラー時は exit 0（hook は block しない）
	}
	q := queue.New(database.SQL())
	return c.RunWithReader(globals, w, os.Stdin, database.SQL(), q)
}

// RunWithReader はテスト可能な実装。reader から stdin を読み取る。
func (c *HookSessionEndCmd) RunWithReader(globals *Globals, w *io.Writer, reader io.Reader, sqlDB *sql.DB, q *queue.Queue) error {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	// 1. stdin デコード
	var input HookSessionEndInput
	if err := json.NewDecoder(reader).Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-end: failed to decode stdin: %v\n", err)
		return nil // exit 0
	}

	// 2. Project 解決
	resolver := project.NewResolver(sqlDB)
	projectID, err := resolver.Resolve(ctx, input.Cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-end: failed to resolve project: %v\n", err)
		// best effort: project_id は部分的に取得できている可能性があるので継続
		if projectID == "" {
			return nil // exit 0
		}
	}

	// 3. payload 構築 + enqueue
	// transcript ファイルは読み込まず、パスのみ保存（M08 ingest worker が担当）
	payload := SessionEndPayload{
		SessionID:      input.SessionID,
		ProjectID:      projectID,
		Cwd:            input.Cwd,
		TranscriptPath: input.TranscriptPath,
		Reason:         input.Reason,
		EnqueuedAt:     time.Now().UTC(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-end: failed to marshal payload: %v\n", err)
		return nil // exit 0
	}

	if _, err := q.Enqueue(ctx, queue.JobTypeSessionEndIngest, string(payloadJSON)); err != nil {
		fmt.Fprintf(os.Stderr, "memoria hook session-end: failed to enqueue: %v\n", err)
		return nil // exit 0
	}

	// 4. ensureWorker（M07 まではスタブ）
	worker.EnsureIngest(ctx)

	return nil
}
