package worker

import (
	"context"
	"database/sql"
	"time"

	"github.com/youyo/memoria/internal/logging"
)

// HeartbeatInterval は heartbeat の更新間隔。
const HeartbeatInterval = 1 * time.Second

// RunHeartbeat は ctx がキャンセルされるまで 1 秒間隔で heartbeat を更新し続ける goroutine。
// DB 書き込みに失敗してもパニックせずログ出力して継続する。
// logf は printf 形式のログ関数（fmt.Fprintf(os.Stderr, ...) などを注入）。
func RunHeartbeat(ctx context.Context, db *sql.DB, workerName, workerID string, interval time.Duration, logf func(string, ...any)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := UpdateHeartbeat(ctx, db, workerName); err != nil {
				if ctx.Err() != nil {
					// context がキャンセル済みならエラーは無視
					return
				}
				if logf != nil {
					logf("memoria heartbeat: update failed: %v\n", err)
				}
				continue
			}
			// probe への応答
			if err := RespondToProbes(ctx, db, workerName, workerID); err != nil {
				if ctx.Err() != nil {
					return
				}
				if logf != nil {
					logf("memoria heartbeat: respond to probes failed: %v\n", err)
				}
			}
		}
	}
}

// StartHeartbeat は goroutine として RunHeartbeat を起動し、cancelFunc を返す。
func StartHeartbeat(ctx context.Context, db *sql.DB, workerName, workerID string, logf func(string, ...any)) context.CancelFunc {
	heartbeatCtx, cancel := context.WithCancel(ctx)
	go RunHeartbeat(heartbeatCtx, db, workerName, workerID, HeartbeatInterval, logf)
	return cancel
}

// logToStderr は logging.Info ラッパー。
func logToStderr(format string, args ...any) {
	logging.Info(format, args...)
}
