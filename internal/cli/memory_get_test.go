package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/db"
)

func TestMemoryGet_NotFound(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "get", "nonexistent-id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// not found の場合は "not found" を含む出力
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory get should be implemented, got: %s", stdout)
	}
	if !strings.Contains(stdout, "not found") {
		t.Errorf("expected 'not found' in output, got: %s", stdout)
	}
}

func TestMemoryGet_JSONFormat_NotFound(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"--format", "json", "memory", "get", "nonexistent-id"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
}

// testChunk はテスト用チャンクデータ型。
type testChunk struct {
	ChunkID     string
	ProjectID   string
	Content     string
	Summary     string
	Kind        string
	Importance  float64
	Scope       string
	ContentHash string
}

// openTestDBWithChunks はテスト用 DB を作成し、指定されたチャンクを挿入して返す。
// project_id が空でない場合、先に projects テーブルへダミーレコードを挿入する（外部キー制約対応）。
func openTestDBWithChunks(t *testing.T, chunks []testChunk) (*db.DB, string) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	sqlDB := database.SQL()

	// project_id の重複排除してプロジェクトを先に挿入
	seen := map[string]bool{}
	for _, ch := range chunks {
		if ch.ProjectID != "" && !seen[ch.ProjectID] {
			seen[ch.ProjectID] = true
			_, err := sqlDB.Exec(
				`INSERT INTO projects (project_id, project_root) VALUES (?, ?)`,
				ch.ProjectID, "/tmp/"+ch.ProjectID,
			)
			if err != nil {
				t.Fatalf("insert project: %v", err)
			}
		}
	}

	for _, ch := range chunks {
		_, err := sqlDB.Exec(
			`INSERT INTO chunks (chunk_id, project_id, content, summary, kind, importance, scope, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ch.ChunkID, ch.ProjectID, ch.Content, ch.Summary, ch.Kind, ch.Importance, ch.Scope, ch.ContentHash,
		)
		if err != nil {
			t.Fatalf("insert chunk: %v", err)
		}
	}

	cfgPath := filepath.Join(dir, "config.toml")
	return database, cfgPath
}

func TestMemoryGet_PrefixMatch(t *testing.T) {
	database, cfgPath := openTestDBWithChunks(t, []testChunk{
		{
			ChunkID:     "abcd1234-5678-9012-3456-789012345678",
			ProjectID:   "proj1",
			Content:     "test content",
			Summary:     "test summary",
			Kind:        "fact",
			Importance:  0.5,
			Scope:       "project",
			ContentHash: "hash1",
		},
	})

	stdout, _, err := parseForTestWithDB(t, []string{"--config", cfgPath, "memory", "get", "abcd1234"}, database)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "test content") {
		t.Errorf("expected 'test content' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "abcd1234-5678-9012-3456-789012345678") {
		t.Errorf("expected full chunk_id in output, got: %s", stdout)
	}
}

func TestMemoryGet_FullIDMatch(t *testing.T) {
	database, cfgPath := openTestDBWithChunks(t, []testChunk{
		{
			ChunkID:     "abcd1234-5678-9012-3456-789012345678",
			ProjectID:   "proj1",
			Content:     "full id content",
			Summary:     "full id summary",
			Kind:        "decision",
			Importance:  0.8,
			Scope:       "global",
			ContentHash: "hash2",
		},
	})

	// 完全な chunk_id で取得できること
	stdout, _, err := parseForTestWithDB(t, []string{"--config", cfgPath, "memory", "get", "abcd1234-5678-9012-3456-789012345678"}, database)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "full id content") {
		t.Errorf("expected 'full id content' in output, got: %s", stdout)
	}
}

func TestMemoryGet_AmbiguousPrefix(t *testing.T) {
	database, cfgPath := openTestDBWithChunks(t, []testChunk{
		{
			ChunkID:     "abcd1111-0000-0000-0000-000000000001",
			ProjectID:   "proj1",
			Content:     "content one",
			Summary:     "",
			Kind:        "fact",
			Importance:  0.5,
			Scope:       "project",
			ContentHash: "hash3",
		},
		{
			ChunkID:     "abcd2222-0000-0000-0000-000000000002",
			ProjectID:   "proj1",
			Content:     "content two",
			Summary:     "",
			Kind:        "fact",
			Importance:  0.5,
			Scope:       "project",
			ContentHash: "hash4",
		},
	})

	// 共通プレフィックス "abcd" で ambiguous になること
	stdout, _, err := parseForTestWithDB(t, []string{"--config", cfgPath, "memory", "get", "abcd"}, database)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "ambiguous") {
		t.Errorf("expected 'ambiguous' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "abcd1111-0000-0000-0000-000000000001") {
		t.Errorf("expected first chunk_id listed in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "abcd2222-0000-0000-0000-000000000002") {
		t.Errorf("expected second chunk_id listed in output, got: %s", stdout)
	}
}

func TestMemoryGet_AmbiguousPrefix_JSON(t *testing.T) {
	database, cfgPath := openTestDBWithChunks(t, []testChunk{
		{
			ChunkID:     "xyz11111-0000-0000-0000-000000000001",
			ProjectID:   "proj1",
			Content:     "content a",
			Summary:     "",
			Kind:        "fact",
			Importance:  0.5,
			Scope:       "project",
			ContentHash: "hash5",
		},
		{
			ChunkID:     "xyz22222-0000-0000-0000-000000000002",
			ProjectID:   "proj1",
			Content:     "content b",
			Summary:     "",
			Kind:        "fact",
			Importance:  0.5,
			Scope:       "project",
			ContentHash: "hash6",
		},
	})

	stdout, _, err := parseForTestWithDB(t, []string{"--config", cfgPath, "--format", "json", "memory", "get", "xyz"}, database)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
	if !strings.Contains(result["error"], "ambiguous") {
		t.Errorf("expected 'ambiguous' in JSON error field, got: %s", result["error"])
	}
}
