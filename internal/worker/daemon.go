package worker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/youyo/memoria/internal/queue"
)

const (
	// WorkerNameIngest は ingest worker の名前。
	WorkerNameIngest = "ingest"
	// DefaultIdleTimeout は ingest worker のデフォルト idle timeout。
	DefaultIdleTimeout = 60 * time.Second
	// StopPollInterval は stop ファイルのポーリング間隔。
	StopPollInterval = 500 * time.Millisecond
	// DequeueStaleTimeout は Dequeue 時の stale recovery timeout。
	DequeueStaleTimeout = 2 * time.Minute
)

// IngestDaemon は ingest worker のライフサイクルを管理する。
type IngestDaemon struct {
	db          *sql.DB
	q           *queue.Queue
	runDir      string
	logDir      string
	idleTimeout time.Duration
	logf        func(string, ...any)
	processor   JobProcessor
}

// NewIngestDaemon は IngestDaemon を生成する。
func NewIngestDaemon(db *sql.DB, q *queue.Queue, runDir, logDir string, idleTimeout time.Duration) *IngestDaemon {
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}
	d := &IngestDaemon{
		db:          db,
		q:           q,
		runDir:      runDir,
		logDir:      logDir,
		idleTimeout: idleTimeout,
	}
	d.logf = func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format, args...)
	}
	d.processor = NewDefaultJobProcessor(db)
	return d
}

// SetProcessor はジョブプロセッサを設定する（テスト用）。
func (d *IngestDaemon) SetProcessor(p JobProcessor) {
	d.processor = p
}

// SetLogf はログ関数を設定する（テスト用）。
func (d *IngestDaemon) SetLogf(logf func(string, ...any)) {
	d.logf = logf
}

// lockPath は ingest.lock のフルパスを返す。
func (d *IngestDaemon) lockPath() string {
	return filepath.Join(d.runDir, "ingest.lock")
}

// pidPath は ingest.pid のフルパスを返す。
func (d *IngestDaemon) pidPath() string {
	return filepath.Join(d.runDir, "ingest.pid")
}

// stopPath は ingest.stop のフルパスを返す。
func (d *IngestDaemon) stopPath() string {
	return filepath.Join(d.runDir, "ingest.stop")
}

// Run は IngestDaemon のメインループを実行する。
// 正常終了（idle timeout / stop ファイル）時は nil を返す。
func (d *IngestDaemon) Run(ctx context.Context) error {
	// 1. RunDir / LogDir を作成
	if err := os.MkdirAll(d.runDir, 0755); err != nil {
		return fmt.Errorf("mkdir run dir: %w", err)
	}
	if err := os.MkdirAll(d.logDir, 0755); err != nil {
		return fmt.Errorf("mkdir log dir: %w", err)
	}

	// 2. ファイルロック取得（二重起動防止）
	lock, err := AcquireLock(d.lockPath())
	if err != nil {
		if err == ErrLockHeld {
			// 既存プロセスが alive -> 正常終了
			d.logf("memoria daemon ingest: already running, exiting\n")
			return nil
		}
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()

	// 3. PID ファイル書き込み
	pid := os.Getpid()
	if err := WritePID(d.pidPath(), pid); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	defer RemovePID(d.pidPath()) //nolint:errcheck

	// 4. worker_leases に INSERT OR REPLACE
	workerID := uuid.New().String()
	lease := WorkerLease{
		WorkerName: WorkerNameIngest,
		WorkerID:   workerID,
		PID:        pid,
		StartedAt:  time.Now().UTC(),
	}
	if err := UpsertLease(ctx, d.db, lease); err != nil {
		return fmt.Errorf("upsert lease: %w", err)
	}
	defer d.cleanup(ctx, workerID)

	// 5. heartbeat goroutine 起動
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()
	go RunHeartbeat(heartbeatCtx, d.db, WorkerNameIngest, workerID, HeartbeatInterval, d.logf)

	// 6. daemon のメインコンテキスト（stop ファイルまたは idle timeout で cancel）
	mainCtx, cancelMain := context.WithCancel(ctx)
	defer cancelMain()

	// 7. watchdog goroutine: stop ファイルを 500ms 間隔でポーリング
	go d.runWatchdog(mainCtx, cancelMain)

	// 8. メインループ
	idleTimer := time.NewTimer(d.idleTimeout)
	defer idleTimer.Stop()

	d.logf("memoria daemon ingest: started (pid=%d, worker_id=%s)\n", pid, workerID)

	for {
		select {
		case <-mainCtx.Done():
			d.logf("memoria daemon ingest: stopping\n")
			return nil
		default:
		}

		job, err := d.q.DequeueWithOptions(mainCtx, workerID, queue.DequeueOptions{
			StaleTimeout: DequeueStaleTimeout,
		})
		if err != nil {
			if mainCtx.Err() != nil {
				return nil
			}
			d.logf("memoria daemon ingest: dequeue error: %v\n", err)
			// 短いバックオフ
			select {
			case <-mainCtx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		if job != nil {
			// ジョブがあれば idle timer をリセット
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(d.idleTimeout)

			// ジョブ処理（M08 で本実装）
			d.processJob(mainCtx, job)
		} else {
			// ジョブなし: idle timer を確認
			select {
			case <-idleTimer.C:
				d.logf("memoria daemon ingest: idle timeout, stopping\n")
				return nil
			case <-mainCtx.Done():
				return nil
			case <-time.After(StopPollInterval):
				// 次のポーリングへ
			}
		}
	}
}

// runWatchdog は stop ファイルの存在を 500ms 間隔でポーリングし、
// 発見したら cancelFn を呼んで daemon を停止する。
func (d *IngestDaemon) runWatchdog(ctx context.Context, cancelFn context.CancelFunc) {
	ticker := time.NewTicker(StopPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if FileExists(d.stopPath()) {
				d.logf("memoria daemon ingest: stop file detected\n")
				cancelFn()
				return
			}
		}
	}
}

// processJob はジョブを処理する。
func (d *IngestDaemon) processJob(ctx context.Context, job *queue.Job) {
	// current_job_id を lease に記録
	if err := UpdateLeaseJobID(ctx, d.db, WorkerNameIngest, job.ID); err != nil {
		d.logf("memoria daemon ingest: update lease job_id: %v\n", err)
	}

	var err error
	switch job.Type {
	case queue.JobTypeCheckpointIngest:
		err = d.processor.HandleCheckpoint(ctx, job)
	case queue.JobTypeSessionEndIngest:
		err = d.processor.HandleSessionEnd(ctx, job)
	default:
		d.logf("memoria daemon ingest: unknown job type: %s, skipping\n", job.Type)
		if ackErr := d.q.Ack(ctx, job.ID); ackErr != nil {
			d.logf("memoria daemon ingest: ack failed: %v\n", ackErr)
		}
		UpdateLeaseJobID(ctx, d.db, WorkerNameIngest, "") //nolint:errcheck
		return
	}

	if err != nil {
		d.logf("memoria daemon ingest: processJob failed: job_id=%s err=%v\n", job.ID, err)
		if failErr := d.q.Fail(ctx, job.ID, err.Error()); failErr != nil {
			d.logf("memoria daemon ingest: fail job: %v\n", failErr)
		}
	} else {
		if ackErr := d.q.Ack(ctx, job.ID); ackErr != nil {
			d.logf("memoria daemon ingest: ack failed: %v\n", ackErr)
		}
		// last_progress_at を更新
		UpdateLeaseProgress(ctx, d.db, WorkerNameIngest) //nolint:errcheck
	}

	// current_job_id をクリア
	UpdateLeaseJobID(ctx, d.db, WorkerNameIngest, "") //nolint:errcheck
}

// cleanup は daemon 停止時のクリーンアップを行う。
func (d *IngestDaemon) cleanup(ctx context.Context, workerID string) {
	// worker_leases を削除
	if err := DeleteLease(context.Background(), d.db, WorkerNameIngest); err != nil {
		d.logf("memoria daemon ingest: delete lease failed: %v\n", err)
	}

	// 未応答 probe を削除
	if err := DeletePendingProbes(context.Background(), d.db, WorkerNameIngest, workerID); err != nil {
		d.logf("memoria daemon ingest: delete pending probes failed: %v\n", err)
	}

	// stop ファイルを削除（存在する場合）
	if err := RemoveFile(d.stopPath()); err != nil {
		d.logf("memoria daemon ingest: remove stop file failed: %v\n", err)
	}

	d.logf("memoria daemon ingest: cleanup done\n")
}
