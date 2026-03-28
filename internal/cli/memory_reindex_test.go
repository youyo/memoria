package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/retrieval"
	"github.com/youyo/memoria/internal/testutil"
)

// insertTestEmbeddingJSON はテスト用に chunk_embeddings テーブルへ JSON のみ挿入するヘルパー。
func insertTestEmbeddingJSON(t *testing.T, db *sql.DB, chunkID string, vec []float32) {
	t.Helper()
	jsonBytes, err := json.Marshal(vec)
	if err != nil {
		t.Fatalf("marshal embedding: %v", err)
	}
	const insertSQL = `
INSERT INTO chunk_embeddings (chunk_id, model, embedding_json)
VALUES (?, 'test-model', ?)`
	if _, err := db.ExecContext(context.Background(), insertSQL, chunkID, string(jsonBytes)); err != nil {
		t.Fatalf("insertTestEmbeddingJSON: %v", err)
	}
}

// insertTestProjectWithEmbedding はテスト用に projects + project_embeddings へ JSON のみ挿入するヘルパー。
func insertTestProjectWithEmbedding(t *testing.T, db *sql.DB, projectID string, vec []float32) {
	t.Helper()
	const projSQL = `INSERT OR IGNORE INTO projects (project_id, project_root) VALUES (?, ?)`
	if _, err := db.ExecContext(context.Background(), projSQL, projectID, "/test/"+projectID); err != nil {
		t.Fatalf("insertTestProjectWithEmbedding insert project: %v", err)
	}
	jsonBytes, err := json.Marshal(vec)
	if err != nil {
		t.Fatalf("marshal embedding: %v", err)
	}
	const embSQL = `
INSERT INTO project_embeddings (project_id, model, embedding_json)
VALUES (?, 'test-model', ?)`
	if _, err := db.ExecContext(context.Background(), embSQL, projectID, string(jsonBytes)); err != nil {
		t.Fatalf("insertTestProjectWithEmbedding insert embedding: %v", err)
	}
}

// insertTestChunkForReindex はテスト用に chunks + chunk_embeddings へ挿入するヘルパー。
func insertTestChunkForReindex(t *testing.T, db *sql.DB, chunkID, projectID string, vec []float32) {
	t.Helper()
	const projSQL = `INSERT OR IGNORE INTO projects (project_id, project_root) VALUES (?, ?)`
	if _, err := db.ExecContext(context.Background(), projSQL, projectID, "/test/"+projectID); err != nil {
		t.Fatalf("insertTestChunkForReindex insert project: %v", err)
	}
	const chunkSQL = `
INSERT OR IGNORE INTO chunks (chunk_id, project_id, content, kind, importance, scope, content_hash)
VALUES (?, ?, 'test content', 'fact', 0.5, 'project', ?)`
	if _, err := db.ExecContext(context.Background(), chunkSQL, chunkID, projectID, "hash-"+chunkID); err != nil {
		t.Fatalf("insertTestChunkForReindex insert chunk: %v", err)
	}
	insertTestEmbeddingJSON(t, db, chunkID, vec)
}

// TestReindexChunkEmbeddings_Basic は chunk_embeddings の JSON → blob 変換を検証する。
func TestReindexChunkEmbeddings_Basic(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	sqlDB := database.SQL()

	vec := []float32{1.0, 0.5, 0.25}
	insertTestChunkForReindex(t, sqlDB, "chunk1", "proj1", vec)
	insertTestChunkForReindex(t, sqlDB, "chunk2", "proj1", []float32{0.0, 1.0, 0.0})

	// 変換前: blob は NULL
	var blobBefore []byte
	err := sqlDB.QueryRowContext(context.Background(),
		"SELECT embedding_blob FROM chunk_embeddings WHERE chunk_id = 'chunk1'",
	).Scan(&blobBefore)
	if err != nil {
		t.Fatalf("query before reindex: %v", err)
	}
	if blobBefore != nil {
		t.Error("expected embedding_blob to be NULL before reindex")
	}

	// reindex 実行
	count, err := reindexChunkEmbeddings(context.Background(), sqlDB, 100, false)
	if err != nil {
		t.Fatalf("reindexChunkEmbeddings: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 converted, got %d", count)
	}

	// 変換後: blob が正しい値になっている
	var blobAfter []byte
	err = sqlDB.QueryRowContext(context.Background(),
		"SELECT embedding_blob FROM chunk_embeddings WHERE chunk_id = 'chunk1'",
	).Scan(&blobAfter)
	if err != nil {
		t.Fatalf("query after reindex: %v", err)
	}
	if blobAfter == nil {
		t.Fatal("expected embedding_blob to be non-NULL after reindex")
	}

	// blob をデコードして元のベクトルと一致するか確認
	decoded, err := retrieval.BytesToFloat32Slice(blobAfter)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	if len(decoded) != len(vec) {
		t.Fatalf("decoded length %d != original %d", len(decoded), len(vec))
	}
	for i, v := range vec {
		if decoded[i] != v {
			t.Errorf("decoded[%d] = %f, want %f", i, decoded[i], v)
		}
	}
}

// TestReindexChunkEmbeddings_DryRun は dry-run モードが実際に書き込まないことを検証する。
func TestReindexChunkEmbeddings_DryRun(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	sqlDB := database.SQL()

	insertTestChunkForReindex(t, sqlDB, "chunk1", "proj1", []float32{1.0, 0.0})

	count, err := reindexChunkEmbeddings(context.Background(), sqlDB, 100, true)
	if err != nil {
		t.Fatalf("reindexChunkEmbeddings dry-run: %v", err)
	}
	if count != 1 {
		t.Errorf("dry-run count = %d, want 1", count)
	}

	// blob は依然として NULL のはず
	var blobAfter []byte
	err = sqlDB.QueryRowContext(context.Background(),
		"SELECT embedding_blob FROM chunk_embeddings WHERE chunk_id = 'chunk1'",
	).Scan(&blobAfter)
	if err != nil {
		t.Fatalf("query after dry-run: %v", err)
	}
	if blobAfter != nil {
		t.Error("dry-run should not have written blob")
	}
}

// TestReindexChunkEmbeddings_Idempotent は既に blob がある場合に再変換しないことを検証する。
func TestReindexChunkEmbeddings_Idempotent(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	sqlDB := database.SQL()

	insertTestChunkForReindex(t, sqlDB, "chunk1", "proj1", []float32{1.0, 0.0})

	// 1回目の変換
	count1, err := reindexChunkEmbeddings(context.Background(), sqlDB, 100, false)
	if err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	if count1 != 1 {
		t.Errorf("first reindex count = %d, want 1", count1)
	}

	// 2回目は 0 件（既に blob が存在する）
	count2, err := reindexChunkEmbeddings(context.Background(), sqlDB, 100, false)
	if err != nil {
		t.Fatalf("second reindex: %v", err)
	}
	if count2 != 0 {
		t.Errorf("second reindex count = %d, want 0 (idempotent)", count2)
	}
}

// TestReindexProjectEmbeddings_Basic は project_embeddings の JSON → blob 変換を検証する。
func TestReindexProjectEmbeddings_Basic(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	sqlDB := database.SQL()

	vec := []float32{0.1, 0.9, 0.5}
	insertTestProjectWithEmbedding(t, sqlDB, "proj1", vec)

	count, err := reindexProjectEmbeddings(context.Background(), sqlDB, 100, false)
	if err != nil {
		t.Fatalf("reindexProjectEmbeddings: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 converted, got %d", count)
	}

	var blobAfter []byte
	err = sqlDB.QueryRowContext(context.Background(),
		"SELECT embedding_blob FROM project_embeddings WHERE project_id = 'proj1'",
	).Scan(&blobAfter)
	if err != nil {
		t.Fatalf("query after reindex: %v", err)
	}
	if blobAfter == nil {
		t.Fatal("expected embedding_blob to be non-NULL after reindex")
	}

	decoded, err := retrieval.BytesToFloat32Slice(blobAfter)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	for i, v := range vec {
		if decoded[i] != v {
			t.Errorf("decoded[%d] = %f, want %f", i, decoded[i], v)
		}
	}
}

// TestMemoryReindexCmd_Run は CLI コマンドの統合テスト。
func TestMemoryReindexCmd_Run(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	sqlDB := database.SQL()

	// データを挿入
	insertTestChunkForReindex(t, sqlDB, "chunk1", "proj1", []float32{1.0, 0.0, 0.0})
	insertTestChunkForReindex(t, sqlDB, "chunk2", "proj1", []float32{0.0, 1.0, 0.0})
	insertTestProjectWithEmbedding(t, sqlDB, "proj1", []float32{0.5, 0.5})

	stdout, _, err := parseForTestWithDB(t, []string{"memory", "reindex"}, database)
	if err != nil {
		t.Fatalf("memory reindex error: %v", err)
	}

	// 出力に変換件数が含まれること
	if !strings.Contains(stdout, "chunk_embeddings=2") {
		t.Errorf("expected chunk_embeddings=2 in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "project_embeddings=1") {
		t.Errorf("expected project_embeddings=1 in output, got: %s", stdout)
	}
}

// TestMemoryReindexCmd_DryRun は --dry-run フラグを検証する。
func TestMemoryReindexCmd_DryRun(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	sqlDB := database.SQL()

	insertTestChunkForReindex(t, sqlDB, "chunk1", "proj1", []float32{1.0, 0.0})

	stdout, _, err := parseForTestWithDB(t, []string{"memory", "reindex", "--dry-run"}, database)
	if err != nil {
		t.Fatalf("memory reindex --dry-run error: %v", err)
	}

	if !strings.Contains(stdout, "dry-run") {
		t.Errorf("expected dry-run in output, got: %s", stdout)
	}

	// dry-run なので blob は NULL のまま
	var blobAfter []byte
	err = sqlDB.QueryRowContext(context.Background(),
		"SELECT embedding_blob FROM chunk_embeddings WHERE chunk_id = 'chunk1'",
	).Scan(&blobAfter)
	if err != nil {
		t.Fatalf("query after dry-run: %v", err)
	}
	if blobAfter != nil {
		t.Error("dry-run should not have written blob")
	}
}
