package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	cfg_pkg "github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/worker"
)

// DoctorCheck は1つの診断チェック結果を表す。
type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// DoctorResult は doctor コマンド全体の結果を表す。
type DoctorResult struct {
	Checks []DoctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

// DoctorCmd はシステム診断コマンドを定義する。
type DoctorCmd struct{}

// Run は doctor コマンドを実行する。
func (c *DoctorCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		// DB が開けない場合もチェック結果として返す
		result := DoctorResult{OK: false}
		result.Checks = append(result.Checks, DoctorCheck{
			Name:   "db_connected",
			OK:     false,
			Detail: err.Error(),
		})
		switch globals.Format {
		case "json":
			enc := json.NewEncoder(*w)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		default:
			return printDoctorText(*w, result)
		}
	}
	return c.runWithDB(globals, w, database)
}

// runWithDB は DB が利用可能な場合の doctor コマンド実装。
func (c *DoctorCmd) runWithDB(globals *Globals, w *io.Writer, database *db.DB) error {
	result := DoctorResult{OK: true}

	// --- パス確認 ---
	configPath := resolveConfigPath(globals)
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "config_path",
		OK:     true,
		Detail: configPath,
	})
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "data_dir",
		OK:     true,
		Detail: cfg_pkg.DataDir(),
	})
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "state_dir",
		OK:     true,
		Detail: cfg_pkg.StateDir(),
	})
	result.Checks = append(result.Checks, DoctorCheck{
		Name:   "db_file",
		OK:     true,
		Detail: database.Path(),
	})

	// --- DB 接続確認 ---
	dbCheck := DoctorCheck{Name: "db_connected"}
	if err := database.Ping(); err != nil {
		dbCheck.OK = false
		dbCheck.Detail = err.Error()
		result.OK = false
	} else {
		var version int
		if err := database.SQL().QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
			dbCheck.OK = false
			dbCheck.Detail = fmt.Sprintf("schema version query failed: %v", err)
			result.OK = false
		} else {
			dbCheck.OK = true
			dbCheck.Detail = fmt.Sprintf("schema version: %d", version)
		}
	}
	result.Checks = append(result.Checks, dbCheck)

	// --- Pragma 確認 ---
	walCheck := DoctorCheck{Name: "pragma_wal"}
	var journalMode string
	if err := database.SQL().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		walCheck.OK = false
		walCheck.Detail = err.Error()
		result.OK = false
	} else if journalMode != "wal" {
		walCheck.OK = false
		walCheck.Detail = fmt.Sprintf("journal_mode = %s (want wal)", journalMode)
		result.OK = false
	} else {
		walCheck.OK = true
		walCheck.Detail = "journal_mode = wal"
	}
	result.Checks = append(result.Checks, walCheck)

	fkCheck := DoctorCheck{Name: "pragma_foreign_keys"}
	var fk int
	if err := database.SQL().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		fkCheck.OK = false
		fkCheck.Detail = err.Error()
		result.OK = false
	} else if fk != 1 {
		fkCheck.OK = false
		fkCheck.Detail = fmt.Sprintf("foreign_keys = %d (want 1)", fk)
		result.OK = false
	} else {
		fkCheck.OK = true
		fkCheck.Detail = "foreign_keys = ON"
	}
	result.Checks = append(result.Checks, fkCheck)

	// --- FTS テーブル確認 ---
	ftsCheck := DoctorCheck{Name: "fts_table"}
	var tableName string
	err := database.SQL().QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='chunks_fts'",
	).Scan(&tableName)
	if err != nil {
		ftsCheck.OK = false
		ftsCheck.Detail = "chunks_fts not found"
		result.OK = false
	} else {
		ftsCheck.OK = true
		ftsCheck.Detail = "chunks_fts exists"
	}
	result.Checks = append(result.Checks, ftsCheck)

	// --- Ingest worker 状態確認 ---
	ingestCheck := DoctorCheck{Name: "ingest_worker"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	liveness, lease, livenessErr := worker.CheckLiveness(ctx, database.SQL(), worker.WorkerNameIngest)
	if livenessErr != nil {
		ingestCheck.OK = false
		ingestCheck.Detail = fmt.Sprintf("check failed: %v", livenessErr)
		result.OK = false
	} else {
		switch liveness {
		case worker.LivenessAlive:
			ingestCheck.OK = true
			if lease != nil {
				ingestCheck.Detail = fmt.Sprintf("alive (pid=%d, worker_id=%s)", lease.PID, lease.WorkerID)
			} else {
				ingestCheck.Detail = "alive"
			}
		case worker.LivenessSuspect:
			ingestCheck.OK = false
			ingestCheck.Detail = "suspect (heartbeat delayed)"
			result.OK = false
		case worker.LivenessStale:
			ingestCheck.OK = false
			ingestCheck.Detail = "stale (no recent heartbeat)"
			result.OK = false
		case worker.LivenessNotRunning:
			ingestCheck.OK = false
			ingestCheck.Detail = "not running"
			result.OK = false
		}
	}
	result.Checks = append(result.Checks, ingestCheck)

	// --- Embedding worker 状態確認（ソケットファイルの存在確認のみ）---
	embeddingCheck := DoctorCheck{Name: "embedding_worker"}
	socketPath := cfg_pkg.SocketPath()
	if _, statErr := os.Stat(socketPath); statErr == nil {
		embeddingCheck.OK = true
		embeddingCheck.Detail = fmt.Sprintf("socket exists: %s", socketPath)
	} else {
		embeddingCheck.OK = false
		embeddingCheck.Detail = fmt.Sprintf("socket not found: %s", socketPath)
		result.OK = false
	}
	result.Checks = append(result.Checks, embeddingCheck)

	// --- Config 検証 ---
	configCheck := DoctorCheck{Name: "config_valid"}
	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		// 設定ファイルが存在しない場合はデフォルト値を使用するので OK
		configCheck.OK = true
		configCheck.Detail = "not found (using defaults)"
	} else {
		if _, loadErr := cfg_pkg.Load(configPath); loadErr != nil {
			configCheck.OK = false
			configCheck.Detail = fmt.Sprintf("parse error: %v", loadErr)
			result.OK = false
		} else {
			configCheck.OK = true
			configCheck.Detail = fmt.Sprintf("valid: %s", configPath)
		}
	}
	result.Checks = append(result.Checks, configCheck)

	// --- Queue depth 確認 ---
	queueCheck := DoctorCheck{Name: "queue_depth"}
	var queueDepth int
	if qErr := database.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'queued'").Scan(&queueDepth); qErr != nil {
		queueCheck.OK = false
		queueCheck.Detail = fmt.Sprintf("query failed: %v", qErr)
	} else {
		queueCheck.OK = true
		queueCheck.Detail = fmt.Sprintf("queued=%d", queueDepth)
	}
	result.Checks = append(result.Checks, queueCheck)

	// --- 出力 ---
	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	default:
		return printDoctorText(*w, result)
	}
}

// printDoctorText はテキスト形式で診断結果を出力する。
func printDoctorText(w io.Writer, result DoctorResult) error {
	for _, check := range result.Checks {
		status := "[ok]"
		if !check.OK {
			status = "[fail]"
		}
		fmt.Fprintf(w, "%-8s %-24s %s\n", status, check.Name+":", check.Detail)
	}
	fmt.Fprintln(w, "──────────────────────────────────────────")
	if result.OK {
		fmt.Fprintln(w, "All checks passed.")
	} else {
		fmt.Fprintln(w, "Some checks failed.")
	}
	return nil
}
