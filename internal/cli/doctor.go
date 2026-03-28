package cli

import (
	"encoding/json"
	"fmt"
	"io"

	cfg_pkg "github.com/youyo/memoria/internal/config"
	"github.com/youyo/memoria/internal/db"
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
func (c *DoctorCmd) Run(globals *Globals, w *io.Writer, database *db.DB) error {
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
