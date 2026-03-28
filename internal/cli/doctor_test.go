package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/db"
)

// doctorParseForTest は tmp ディレクトリに DB を作成して doctor コマンドを実行するヘルパー。
func doctorParseForTest(t *testing.T, args []string) (string, *CLI, error) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfgPath := filepath.Join(dir, "config.toml")
	allArgs := append([]string{"--config", cfgPath}, args...)
	return parseForTestWithDB(t, allArgs, database)
}

func TestDoctorCmd_OK(t *testing.T) {
	stdout, _, err := doctorParseForTest(t, []string{"doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("expected 'ok' in output, got: %s", stdout)
	}
}

func TestDoctorCmd_DBNotExist(t *testing.T) {
	// DB が存在しない初期状態でも doctor が成功する
	// （Open が DB を作成するため）
	stdout, _, err := doctorParseForTest(t, []string{"doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "db") {
		t.Errorf("expected db info in output, got: %s", stdout)
	}
}

func TestDoctorCmd_JSONFormat(t *testing.T) {
	stdout, _, err := doctorParseForTest(t, []string{"--format", "json", "doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Checks []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		} `json:"checks"`
		OK bool `json:"ok"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
	if !result.OK {
		t.Errorf("expected ok=true, got false")
	}
	if len(result.Checks) == 0 {
		t.Error("expected checks to be non-empty")
	}
}

func TestDoctorCmd_PragmaWAL(t *testing.T) {
	stdout, _, err := doctorParseForTest(t, []string{"--format", "json", "doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON: %v", jsonErr)
	}

	found := false
	for _, check := range result.Checks {
		if check.Name == "pragma_wal" {
			found = true
			if !check.OK {
				t.Error("expected pragma_wal check to be ok")
			}
		}
	}
	if !found {
		t.Errorf("expected pragma_wal check, not found in: %s", stdout)
	}
}

func TestDoctorCmd_PragmaForeignKeys(t *testing.T) {
	stdout, _, err := doctorParseForTest(t, []string{"--format", "json", "doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON: %v", jsonErr)
	}

	found := false
	for _, check := range result.Checks {
		if check.Name == "pragma_foreign_keys" {
			found = true
			if !check.OK {
				t.Error("expected pragma_foreign_keys check to be ok")
			}
		}
	}
	if !found {
		t.Errorf("expected pragma_foreign_keys check, not found in: %s", stdout)
	}
}

func TestDoctorCmd_FTSTableExists(t *testing.T) {
	stdout, _, err := doctorParseForTest(t, []string{"--format", "json", "doctor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON: %v", jsonErr)
	}

	found := false
	for _, check := range result.Checks {
		if check.Name == "fts_table" {
			found = true
			if !check.OK {
				t.Error("expected fts_table check to be ok")
			}
		}
	}
	if !found {
		t.Errorf("expected fts_table check, not found in: %s", stdout)
	}
}
