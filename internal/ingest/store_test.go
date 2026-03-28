package ingest_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/testutil"
)

// insertTestProject はテスト用プロジェクトを projects テーブルに挿入する。
func insertTestProject(t *testing.T, db *sql.DB, projectID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO projects (project_id, project_root, last_seen_at, created_at) VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		projectID, "/tmp/"+projectID,
	)
	if err != nil {
		t.Fatalf("insert test project: %v", err)
	}
}

func TestUpsertSession(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
}

func TestUpsertSessionUpdate(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	// 1回目: INSERT
	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession (insert): %v", err)
	}

	// 2回目: 同じ session_id で UPSERT（ended_at を設定）
	now := time.Now().UTC()
	err = ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID:      "s1",
		ProjectID:      "p1",
		Cwd:            "/tmp/project",
		TranscriptPath: "/tmp/transcript.jsonl",
		EndedAt:        &now,
	})
	if err != nil {
		t.Fatalf("UpsertSession (update): %v", err)
	}
}

func TestInsertTurn(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	// まず session を作成
	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	err = ingest.InsertTurn(ctx, db, ingest.TurnRecord{
		TurnID:    "t1",
		SessionID: "s1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}
}

func TestCountTurnsBySession(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	// session を作成
	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// 0件確認
	count, err := ingest.CountTurnsBySession(ctx, db, "s1")
	if err != nil {
		t.Fatalf("CountTurnsBySession: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 turns, got %d", count)
	}

	// turn を追加
	err = ingest.InsertTurn(ctx, db, ingest.TurnRecord{
		TurnID:    "t1",
		SessionID: "s1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}

	// 1件確認
	count, err = ingest.CountTurnsBySession(ctx, db, "s1")
	if err != nil {
		t.Fatalf("CountTurnsBySession after insert: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 turn, got %d", count)
	}
}

func TestInsertChunk(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	// session を作成
	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	err = ingest.InsertChunk(ctx, db, ingest.ChunkRecord{
		ChunkID:      "c1",
		SessionID:    "s1",
		ProjectID:    "p1",
		Content:      "test chunk content",
		Summary:      "test chunk",
		Kind:         "fact",
		Importance:   0.5,
		Scope:        "project",
		KeywordsJSON: `["test","chunk"]`,
		ContentHash:  ingest.ContentHash("test chunk content"),
	})
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}
}

func TestInsertChunkDuplicate(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	// session を作成
	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	content := "duplicate chunk content"
	hash := ingest.ContentHash(content)
	rec := ingest.ChunkRecord{
		ChunkID:      "c1",
		SessionID:    "s1",
		ProjectID:    "p1",
		Content:      content,
		Summary:      "dup",
		Kind:         "fact",
		Importance:   0.5,
		Scope:        "project",
		KeywordsJSON: "[]",
		ContentHash:  hash,
	}

	// 1回目: INSERT
	if err := ingest.InsertChunk(ctx, db, rec); err != nil {
		t.Fatalf("InsertChunk (1st): %v", err)
	}

	// 2回目: ON CONFLICT DO NOTHING → エラーなし
	rec.ChunkID = "c2" // chunk_id を変えても content_hash が同じなら SKIP
	if err := ingest.InsertChunk(ctx, db, rec); err != nil {
		t.Fatalf("InsertChunk (2nd, duplicate): %v", err)
	}
}

func TestInsertChunkFTSSync(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()
	insertTestProject(t, db, "p1")

	// session を作成
	err := ingest.UpsertSession(ctx, db, ingest.SessionRecord{
		SessionID: "s1",
		ProjectID: "p1",
		Cwd:       "/tmp/project",
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	content := "memoriaproject unique keyword xyzzy12345"
	err = ingest.InsertChunk(ctx, db, ingest.ChunkRecord{
		ChunkID:      "c1",
		SessionID:    "s1",
		ProjectID:    "p1",
		Content:      content,
		Summary:      "test",
		Kind:         "fact",
		Importance:   0.5,
		Scope:        "project",
		KeywordsJSON: `["xyzzy12345"]`,
		ContentHash:  ingest.ContentHash(content),
	})
	if err != nil {
		t.Fatalf("InsertChunk: %v", err)
	}

	// FTS5 で検索できるか確認
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks_fts WHERE chunks_fts MATCH 'xyzzy12345'")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("FTS query: %v", err)
	}
	if count == 0 {
		t.Error("expected FTS to find the chunk, got 0")
	}
}

func TestContentHash(t *testing.T) {
	h1 := ingest.ContentHash("hello")
	h2 := ingest.ContentHash("hello")
	h3 := ingest.ContentHash("world")

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
	if len(h1) != 64 { // SHA-256 hex
		t.Errorf("expected 64 hex chars, got %d", len(h1))
	}
	// 全て小文字の hex 文字であること
	for _, c := range h1 {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("invalid hex char in hash: %c", c)
		}
	}
}
