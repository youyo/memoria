package ingest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/youyo/memoria/internal/ingest"
	"github.com/youyo/memoria/internal/testutil"
)

// mockEmbedClient は EmbedClient のテスト用モック。
type mockEmbedClient struct {
	embedFn func(ctx context.Context, texts []string) ([][]float32, error)
	called  bool
	lastTexts []string
}

func (m *mockEmbedClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.called = true
	m.lastTexts = texts
	return m.embedFn(ctx, texts)
}

// dummyEmbeddings は texts と同数の float32 スライスを返すヘルパー。
func dummyEmbeddings(texts []string) [][]float32 {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result
}

func TestEmbedChunks_EmptyChunkIDs(t *testing.T) {
	db := testutil.OpenTestDB(t)
	mock := &mockEmbedClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float32, error) {
			return dummyEmbeddings(texts), nil
		},
	}
	embedder := ingest.NewChunkEmbedder(mock)

	err := embedder.EmbedChunks(context.Background(), db, []string{}, "test-model")
	if err != nil {
		t.Fatalf("EmbedChunks with empty ids: %v", err)
	}
	// Embed は呼ばれないはず
	if mock.called {
		t.Error("Embed should not be called with empty chunkIDs")
	}
}

func TestEmbedChunks_Success(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	// projects テーブルに挿入
	_, err := db.ExecContext(ctx,
		`INSERT INTO projects (project_id, project_root, last_seen_at, created_at) VALUES ('p1', '/tmp/p1', strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}

	// sessions テーブルに挿入
	_, err = db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, project_id, cwd) VALUES ('s1', 'p1', '/tmp/p1')`,
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// chunks テーブルにテストデータを挿入
	chunkID1 := "chunk-001"
	chunkID2 := "chunk-002"
	for _, rec := range []struct {
		id, content, hash string
	}{
		{chunkID1, "content of chunk 1", "hash001"},
		{chunkID2, "content of chunk 2", "hash002"},
	} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO chunks (chunk_id, session_id, project_id, content, summary, kind, importance, scope, project_transferability, keywords_json, applies_to_json, content_hash, created_at)
			 VALUES (?, 's1', 'p1', ?, '', 'fact', 0.5, 'project', 0.5, '[]', '[]', ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
			rec.id, rec.content, rec.hash,
		)
		if err != nil {
			t.Fatalf("insert chunk %s: %v", rec.id, err)
		}
	}

	mock := &mockEmbedClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float32, error) {
			return dummyEmbeddings(texts), nil
		},
	}
	embedder := ingest.NewChunkEmbedder(mock)

	err = embedder.EmbedChunks(ctx, db, []string{chunkID1, chunkID2}, "test-model")
	if err != nil {
		t.Fatalf("EmbedChunks: %v", err)
	}

	// Embed が呼ばれたことを確認
	if !mock.called {
		t.Error("Embed should have been called")
	}
	if len(mock.lastTexts) != 2 {
		t.Errorf("expected 2 texts, got %d", len(mock.lastTexts))
	}

	// chunk_embeddings テーブルに 2 件保存されていること
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunk_embeddings")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunk_embeddings: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 chunk_embeddings, got %d", count)
	}

	// model が正しく保存されていること
	var model string
	row = db.QueryRowContext(ctx, "SELECT model FROM chunk_embeddings WHERE chunk_id = ?", chunkID1)
	if err := row.Scan(&model); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if model != "test-model" {
		t.Errorf("expected model=test-model, got %s", model)
	}
}

func TestEmbedChunks_SkipAlreadyEmbedded(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	// プロジェクトとセッションの挿入
	_, _ = db.ExecContext(ctx,
		`INSERT INTO projects (project_id, project_root, last_seen_at, created_at) VALUES ('p1', '/tmp/p1', strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, project_id, cwd) VALUES ('s1', 'p1', '/tmp/p1')`)

	chunkID1 := "chunk-skip-001"
	chunkID2 := "chunk-skip-002"
	for _, rec := range []struct {
		id, content, hash string
	}{
		{chunkID1, "already embedded content", "hash-skip-001"},
		{chunkID2, "new content", "hash-skip-002"},
	} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO chunks (chunk_id, session_id, project_id, content, summary, kind, importance, scope, project_transferability, keywords_json, applies_to_json, content_hash, created_at)
			 VALUES (?, 's1', 'p1', ?, '', 'fact', 0.5, 'project', 0.5, '[]', '[]', ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
			rec.id, rec.content, rec.hash,
		)
		if err != nil {
			t.Fatalf("insert chunk: %v", err)
		}
	}

	// chunkID1 は既に embedding 済みとして挿入
	_, err := db.ExecContext(ctx,
		`INSERT INTO chunk_embeddings (chunk_id, model, embedding_json, created_at) VALUES (?, 'test-model', '[0.1,0.2,0.3]', strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		chunkID1,
	)
	if err != nil {
		t.Fatalf("insert pre-existing embedding: %v", err)
	}

	embedCallCount := 0
	mock := &mockEmbedClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float32, error) {
			embedCallCount++
			// chunkID2 のみ embed されるはず → texts は 1 件
			if len(texts) != 1 {
				t.Errorf("expected 1 text (skip already embedded), got %d", len(texts))
			}
			return dummyEmbeddings(texts), nil
		},
	}
	embedder := ingest.NewChunkEmbedder(mock)

	err = embedder.EmbedChunks(ctx, db, []string{chunkID1, chunkID2}, "test-model")
	if err != nil {
		t.Fatalf("EmbedChunks: %v", err)
	}

	if embedCallCount != 1 {
		t.Errorf("expected Embed called once, got %d", embedCallCount)
	}

	// chunk_embeddings テーブルに 2 件（既存 + 新規 1 件）
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunk_embeddings")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunk_embeddings: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 chunk_embeddings (1 existing + 1 new), got %d", count)
	}
}

func TestEmbedChunks_EmbedError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	_, _ = db.ExecContext(ctx,
		`INSERT INTO projects (project_id, project_root, last_seen_at, created_at) VALUES ('p1', '/tmp/p1', strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, project_id, cwd) VALUES ('s1', 'p1', '/tmp/p1')`)

	chunkID := "chunk-err-001"
	_, err := db.ExecContext(ctx,
		`INSERT INTO chunks (chunk_id, session_id, project_id, content, summary, kind, importance, scope, project_transferability, keywords_json, applies_to_json, content_hash, created_at)
		 VALUES (?, 's1', 'p1', 'some content', '', 'fact', 0.5, 'project', 0.5, '[]', '[]', 'hash-err-001', strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		chunkID,
	)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	wantErr := errors.New("embedding worker not available")
	mock := &mockEmbedClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float32, error) {
			return nil, wantErr
		},
	}
	embedder := ingest.NewChunkEmbedder(mock)

	err = embedder.EmbedChunks(ctx, db, []string{chunkID}, "test-model")
	if err == nil {
		t.Fatal("expected error from EmbedChunks when Embed fails")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped wantErr, got %v", err)
	}
}

func TestEmbedChunks_Idempotent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	_, _ = db.ExecContext(ctx,
		`INSERT INTO projects (project_id, project_root, last_seen_at, created_at) VALUES ('p1', '/tmp/p1', strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`)
	_, _ = db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, project_id, cwd) VALUES ('s1', 'p1', '/tmp/p1')`)

	chunkID := "chunk-idem-001"
	_, err := db.ExecContext(ctx,
		`INSERT INTO chunks (chunk_id, session_id, project_id, content, summary, kind, importance, scope, project_transferability, keywords_json, applies_to_json, content_hash, created_at)
		 VALUES (?, 's1', 'p1', 'idempotent content', '', 'fact', 0.5, 'project', 0.5, '[]', '[]', 'hash-idem-001', strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		chunkID,
	)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	mock := &mockEmbedClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float32, error) {
			return dummyEmbeddings(texts), nil
		},
	}
	embedder := ingest.NewChunkEmbedder(mock)

	// 1回目
	if err := embedder.EmbedChunks(ctx, db, []string{chunkID}, "test-model"); err != nil {
		t.Fatalf("EmbedChunks (1st): %v", err)
	}

	// 2回目（冪等：エラーなし、重複なし）
	if err := embedder.EmbedChunks(ctx, db, []string{chunkID}, "test-model"); err != nil {
		t.Fatalf("EmbedChunks (2nd, idempotent): %v", err)
	}

	// chunk_embeddings に 1 件のみ
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunk_embeddings WHERE chunk_id = ?", chunkID)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count chunk_embeddings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 chunk_embedding (idempotent), got %d", count)
	}
}
