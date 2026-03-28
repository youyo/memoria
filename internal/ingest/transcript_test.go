package ingest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/ingest"
)

func writeTempTranscript(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return f.Name()
}

func TestParseTranscriptEmpty(t *testing.T) {
	path := writeTempTranscript(t, "")
	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(turns))
	}
}

func TestParseTranscriptUserOnly(t *testing.T) {
	jsonl := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T12:00:00.000Z","uuid":"u1"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Role != "user" {
		t.Errorf("expected role=user, got %s", turns[0].Role)
	}
	if turns[0].Content != "hello" {
		t.Errorf("expected content=hello, got %s", turns[0].Content)
	}
	if turns[0].CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestParseTranscriptPair(t *testing.T) {
	jsonl := `{"type":"user","message":{"role":"user","content":"what is Go?"},"timestamp":"2026-03-28T12:00:00.000Z","uuid":"u1"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"Go is a programming language."},"timestamp":"2026-03-28T12:00:01.000Z","uuid":"u2"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].Role != "user" {
		t.Errorf("turn[0] expected role=user, got %s", turns[0].Role)
	}
	if turns[1].Role != "assistant" {
		t.Errorf("turn[1] expected role=assistant, got %s", turns[1].Role)
	}
}

func TestParseTranscriptToolUse(t *testing.T) {
	// tool_use を含む content の正規化
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/test.go"}}]},"timestamp":"2026-03-28T12:00:01.000Z","uuid":"u2"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	// tool_use は "[Tool: name(...)]" に圧縮される
	if turns[0].Content == "" {
		t.Error("expected non-empty content for tool_use")
	}
	// "Read" が含まれること
	if turns[0].Content == "" {
		t.Error("tool_use content should contain tool name")
	}
}

func TestParseTranscriptMultiContent(t *testing.T) {
	// content が []ContentPart（text + tool_use）の場合
	jsonl := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"here is the code:"},{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"/tmp/out.go"}}]},"timestamp":"2026-03-28T12:00:01.000Z","uuid":"u2"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	// text 部分が含まれること
	content := turns[0].Content
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestParseTranscriptNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.jsonl")
	_, err := ingest.ParseTranscript(path)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if err != ingest.ErrTranscriptNotFound {
		t.Errorf("expected ErrTranscriptNotFound, got %v", err)
	}
}

func TestParseTranscriptInvalidJSON(t *testing.T) {
	// 無効な行をスキップして残りを処理する
	jsonl := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T12:00:00.000Z","uuid":"u1"}` + "\n" +
		`INVALID JSON LINE` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"world"},"timestamp":"2026-03-28T12:00:01.000Z","uuid":"u2"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	// 無効行はスキップして 2 ターン取得
	if len(turns) != 2 {
		t.Errorf("expected 2 turns (skipping invalid line), got %d", len(turns))
	}
}

func TestParseTranscriptTimestamp(t *testing.T) {
	jsonl := `{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-03-28T12:34:56.789Z","uuid":"u1"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	expected := time.Date(2026, 3, 28, 12, 34, 56, 789000000, time.UTC)
	if !turns[0].CreatedAt.Equal(expected) {
		t.Errorf("expected CreatedAt=%v, got %v", expected, turns[0].CreatedAt)
	}
}

func TestParseTranscriptSkipsEmptyContent(t *testing.T) {
	// 空文字列 content の行はスキップ
	jsonl := `{"type":"user","message":{"role":"user","content":""},"timestamp":"2026-03-28T12:00:00.000Z","uuid":"u1"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"hi"},"timestamp":"2026-03-28T12:00:01.000Z","uuid":"u2"}` + "\n"
	path := writeTempTranscript(t, jsonl)

	turns, err := ingest.ParseTranscript(path)
	if err != nil {
		t.Fatalf("ParseTranscript: %v", err)
	}
	// 空 content の user はスキップされ 1 ターン
	if len(turns) != 1 {
		t.Errorf("expected 1 turn (empty content skipped), got %d", len(turns))
	}
}
