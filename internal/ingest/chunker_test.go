package ingest_test

import (
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/ingest"
)

func TestChunkEmpty(t *testing.T) {
	input := ingest.ChunkInput{
		Turns:     []ingest.Turn{},
		SessionID: "s1",
		ProjectID: "p1",
	}
	chunks := ingest.Chunk(input)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestChunkSinglePair(t *testing.T) {
	turns := []ingest.Turn{
		{Role: "user", Content: "what is Go?"},
		{Role: "assistant", Content: "Go is a programming language."},
	}
	input := ingest.ChunkInput{Turns: turns, SessionID: "s1", ProjectID: "p1"}
	chunks := ingest.Chunk(input)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].Content, "what is Go?") {
		t.Errorf("chunk content should contain user message")
	}
	if !strings.Contains(chunks[0].Content, "Go is a programming language.") {
		t.Errorf("chunk content should contain assistant message")
	}
}

func TestChunkMultiplePairs(t *testing.T) {
	turns := []ingest.Turn{
		{Role: "user", Content: "question 1"},
		{Role: "assistant", Content: "answer 1"},
		{Role: "user", Content: "question 2"},
		{Role: "assistant", Content: "answer 2"},
		{Role: "user", Content: "question 3"},
		{Role: "assistant", Content: "answer 3"},
	}
	input := ingest.ChunkInput{Turns: turns, SessionID: "s1", ProjectID: "p1"}
	chunks := ingest.Chunk(input)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
}

func TestChunkOrphanUser(t *testing.T) {
	// 末尾に対応する assistant がない user ターン
	turns := []ingest.Turn{
		{Role: "user", Content: "question 1"},
		{Role: "assistant", Content: "answer 1"},
		{Role: "user", Content: "orphan question"},
	}
	input := ingest.ChunkInput{Turns: turns, SessionID: "s1", ProjectID: "p1"}
	chunks := ingest.Chunk(input)

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (including orphan user), got %d", len(chunks))
	}
	if !strings.Contains(chunks[1].Content, "orphan question") {
		t.Errorf("last chunk should contain orphan user message")
	}
}

func TestChunkToolTurn(t *testing.T) {
	// tool ターンは直前の user/assistant ペアに包含
	turns := []ingest.Turn{
		{Role: "user", Content: "do something"},
		{Role: "tool", Content: "[Tool: Read(file=/tmp/x.go)]"},
		{Role: "assistant", Content: "done"},
	}
	input := ingest.ChunkInput{Turns: turns, SessionID: "s1", ProjectID: "p1"}
	chunks := ingest.Chunk(input)

	// tool ターンは分割されず、1 chunk になるはず
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (tool included in pair), got %d", len(chunks))
	}
}

func TestChunkLongContent(t *testing.T) {
	// MaxChunkBytes (16KiB) を超えるコンテンツは分割される
	longContent := strings.Repeat("a", ingest.MaxChunkBytes+100)
	turns := []ingest.Turn{
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: longContent},
	}
	input := ingest.ChunkInput{Turns: turns, SessionID: "s1", ProjectID: "p1"}
	chunks := ingest.Chunk(input)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for long content, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Content) > ingest.MaxChunkBytes+1000 {
			t.Errorf("chunk[%d] is too large: %d bytes", i, len(c.Content))
		}
	}
}

func TestChunkContentFormat(t *testing.T) {
	// chunk content のフォーマット確認
	turns := []ingest.Turn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	input := ingest.ChunkInput{Turns: turns, SessionID: "s1", ProjectID: "p1"}
	chunks := ingest.Chunk(input)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	content := chunks[0].Content
	if !strings.HasPrefix(content, "User:") && !strings.Contains(content, "User: hello") {
		t.Errorf("chunk content should have User: prefix, got: %s", content[:min(50, len(content))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
