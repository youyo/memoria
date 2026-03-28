package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/db"
)

// memoryParseForTest は tmp ディレクトリに DB を作成して memory コマンドを実行するヘルパー。
func memoryParseForTest(t *testing.T, args []string) (string, *CLI, error) {
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

func TestMemorySearch_EmptyDB(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "search", "test query"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 空 DB では "not implemented" ではなく、results が空（または空配列 JSON）を返す
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory search should be implemented, got: %s", stdout)
	}
}

func TestMemorySearch_JSONFormat(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"--format", "json", "memory", "search", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results interface{}
	if jsonErr := json.Unmarshal([]byte(stdout), &results); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
}

func TestMemorySearch_WithLimit(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "search", "--limit", "5", "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory search should be implemented, got: %s", stdout)
	}
}

func TestMemorySearch_WithKind(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "search", "--kind", "decision", "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory search should be implemented, got: %s", stdout)
	}
}
