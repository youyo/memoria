package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/worker"
)

// DaemonCmd は daemon サブコマンドグループを定義する（内部コマンド）。
type DaemonCmd struct {
	Ingest DaemonIngestCmd `cmd:"" help:"ingest worker デーモンを実行する（内部コマンド）" hidden:""`
}

// DaemonIngestCmd は daemon ingest コマンド（self-spawn で起動される内部コマンド）。
type DaemonIngestCmd struct{}

// Run は daemon ingest を実行する。
func (c *DaemonIngestCmd) Run(globals *Globals, cfg *config.Config, w *io.Writer) error {
	// DB を開く
	dbPath := config.DBFile()
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memoria daemon ingest: open db: %v\n", err)
		return err
	}
	defer database.Close()

	q := queue.New(database.SQL())
	runDir := config.RunDir()
	logDir := config.LogDir()

	idleTimeout := time.Duration(cfg.Worker.IngestIdleTimeout) * time.Second
	if idleTimeout <= 0 {
		idleTimeout = worker.DefaultIdleTimeout
	}

	daemon := worker.NewIngestDaemon(database.SQL(), q, runDir, logDir, idleTimeout)
	return daemon.Run(context.Background())
}
