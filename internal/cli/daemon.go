package cli

import (
	"context"
	"io"

	"github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/worker"
	"github.com/youyo/memoria/internal/logging"
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
		logging.Error("memoria daemon ingest: open db: %v", err)
		return err
	}
	defer database.Close()

	q := queue.New(database.SQL())
	runDir := config.RunDir()
	logDir := config.LogDir()

	daemon := worker.NewIngestDaemonWithEmbedding(database.SQL(), q, runDir, logDir, cfg)
	return daemon.Run(context.Background())
}
