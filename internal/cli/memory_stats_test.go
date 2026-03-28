package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryStats_EmptyDB(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"memory", "stats"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout, "not implemented") {
		t.Errorf("memory stats should be implemented, got: %s", stdout)
	}
	// stats には何らかの数値情報が含まれる
	if !strings.Contains(stdout, "0") {
		t.Errorf("expected stats output to contain counts, got: %s", stdout)
	}
}

func TestMemoryStats_JSONFormat(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"--format", "json", "memory", "stats"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		ChunksTotal  int    `json:"chunks_total"`
		SessionTotal int    `json:"sessions_total"`
		JobsPending  int    `json:"jobs_pending"`
		DBSizeBytes  int64  `json:"db_size_bytes"`
		DBPath       string `json:"db_path"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
	// 空 DB なのでカウントは 0
	if result.ChunksTotal != 0 {
		t.Errorf("expected chunks_total=0 for empty DB, got: %d", result.ChunksTotal)
	}
	if result.DBPath == "" {
		t.Error("expected db_path to be non-empty")
	}
}

func TestMemoryStats_DBSizePositive(t *testing.T) {
	stdout, _, err := memoryParseForTest(t, []string{"--format", "json", "memory", "stats"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		DBSizeBytes int64 `json:"db_size_bytes"`
	}
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON: %v", jsonErr)
	}
	// 初期化済み DB はスキーマ分のサイズがある
	if result.DBSizeBytes <= 0 {
		t.Errorf("expected db_size_bytes > 0, got: %d", result.DBSizeBytes)
	}
}
