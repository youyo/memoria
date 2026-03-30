package retrieval_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/retrieval"
	"github.com/youyo/memoria/internal/testutil"
)

// insertTestChunk はテスト用に chunks テーブルへレコードを挿入するヘルパー。
func insertTestChunk(t *testing.T, db *sql.DB, chunkID, projectID, content, summary, kind string, importance float64, scope string) {
	t.Helper()
	const insertSQL = `
INSERT INTO chunks (chunk_id, project_id, content, summary, kind, importance, scope, content_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.ExecContext(context.Background(), insertSQL,
		chunkID, projectID, content, summary, kind, importance, scope,
		fmt.Sprintf("hash-%s", chunkID),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insertTestChunk: %v", err)
	}
}

// insertTestEmbedding はテスト用に chunk_embeddings テーブルへレコードを挿入するヘルパー。
func insertTestEmbedding(t *testing.T, db *sql.DB, chunkID string, vec []float32) {
	t.Helper()
	jsonBytes, err := json.Marshal(vec)
	if err != nil {
		t.Fatalf("marshal embedding: %v", err)
	}
	const insertSQL = `
INSERT INTO chunk_embeddings (chunk_id, model, embedding_json)
VALUES (?, ?, ?)`
	_, err = db.ExecContext(context.Background(), insertSQL, chunkID, "test-model", string(jsonBytes))
	if err != nil {
		t.Fatalf("insertTestEmbedding: %v", err)
	}
}

// insertTestProject はテスト用に projects テーブルへレコードを挿入するヘルパー。
func insertTestProject(t *testing.T, db *sql.DB, projectID, root string) {
	t.Helper()
	const insertSQL = `
INSERT OR IGNORE INTO projects (project_id, project_root, repo_name)
VALUES (?, ?, ?)`
	_, err := db.ExecContext(context.Background(), insertSQL, projectID, root, root)
	if err != nil {
		t.Fatalf("insertTestProject: %v", err)
	}
}

// mockEmbedder は Embedder interface のモック実装。
type mockEmbedder struct {
	embeddings [][]float32
	err        error
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([][]float32, len(texts))
	for i := range texts {
		if i < len(m.embeddings) {
			result[i] = m.embeddings[i]
		} else {
			result[i] = m.embeddings[0]
		}
	}
	return result, nil
}

// --- CosineSimilarity テスト ---

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1.0, 0.0, 0.0}
	got := retrieval.CosineSimilarity(v, v)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("identical vectors should have similarity 1.0, got %f", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	got := retrieval.CosineSimilarity(a, b)
	if math.Abs(float64(got)) > 1e-6 {
		t.Errorf("orthogonal vectors should have similarity ~0, got %f", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{1.0, 1.0}
	got := retrieval.CosineSimilarity(a, b)
	if got != 0.0 {
		t.Errorf("zero vector should return 0, got %f", got)
	}
}

func TestCosineSimilarity_DifferentLength(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	got := retrieval.CosineSimilarity(a, b)
	if got != 0.0 {
		t.Errorf("different length should return 0, got %f", got)
	}
}

// --- Float32SliceToBytes / BytesToFloat32Slice テスト ---

func TestFloat32SliceToBytes_RoundTrip(t *testing.T) {
	original := []float32{1.0, -0.5, 0.25, 3.14}
	encoded := retrieval.Float32SliceToBytes(original)
	decoded, err := retrieval.BytesToFloat32Slice(encoded)
	if err != nil {
		t.Fatalf("BytesToFloat32Slice error: %v", err)
	}
	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(original))
	}
	for i, v := range original {
		if decoded[i] != v {
			t.Errorf("decoded[%d] = %v, want %v", i, decoded[i], v)
		}
	}
}

func TestFloat32SliceToBytes_EmptySlice(t *testing.T) {
	result := retrieval.Float32SliceToBytes(nil)
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
	result2 := retrieval.Float32SliceToBytes([]float32{})
	if result2 != nil {
		t.Errorf("expected nil for empty slice, got %v", result2)
	}
}

func TestBytesToFloat32Slice_InvalidLength(t *testing.T) {
	// 5 バイトは 4 の倍数でないのでエラー
	_, err := retrieval.BytesToFloat32Slice([]byte{0x01, 0x02, 0x03, 0x04, 0x05})
	if err == nil {
		t.Error("expected error for non-multiple-of-4 bytes, got nil")
	}
}

func TestBytesToFloat32Slice_EmptyBytes(t *testing.T) {
	result, err := retrieval.BytesToFloat32Slice(nil)
	if err != nil {
		t.Fatalf("BytesToFloat32Slice(nil) error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty bytes, got %v", result)
	}
}

func TestFloat32SliceToBytes_ByteLength(t *testing.T) {
	vec := []float32{1.0, 2.0, 3.0}
	b := retrieval.Float32SliceToBytes(vec)
	if len(b) != len(vec)*4 {
		t.Errorf("expected %d bytes, got %d", len(vec)*4, len(b))
	}
}

// --- ベンチマーク ---

func makeTestVec(n int) []float32 {
	vec := make([]float32, n)
	for i := range vec {
		vec[i] = float32(i) * 0.01
	}
	return vec
}

func BenchmarkCosineSimilarity(b *testing.B) {
	dim := 768
	a := makeTestVec(dim)
	c := makeTestVec(dim)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieval.CosineSimilarity(a, c)
	}
}

func BenchmarkFloat32SliceToBytes(b *testing.B) {
	vec := makeTestVec(768)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieval.Float32SliceToBytes(vec)
	}
}

func BenchmarkBytesToFloat32Slice(b *testing.B) {
	vec := makeTestVec(768)
	blob := retrieval.Float32SliceToBytes(vec)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieval.BytesToFloat32Slice(blob) //nolint:errcheck
	}
}

func BenchmarkEmbeddingRoundTrip_JSON(b *testing.B) {
	dim := 768
	vec := makeTestVec(dim)
	jsonBytes, _ := json.Marshal(vec)
	jsonStr := string(jsonBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v []float32
		json.Unmarshal([]byte(jsonStr), &v) //nolint:errcheck
	}
}

func BenchmarkEmbeddingRoundTrip_Blob(b *testing.B) {
	dim := 768
	vec := makeTestVec(dim)
	blob := retrieval.Float32SliceToBytes(vec)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		retrieval.BytesToFloat32Slice(blob) //nolint:errcheck
	}
}

// --- VectorSearch blob パステスト ---

func TestVectorSearch_BlobPath(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "vector search content", "summary", "decision", 0.8, "project")

	// blob 形式で embedding を挿入
	vec := []float32{1.0, 0.0, 0.0}
	blob := retrieval.Float32SliceToBytes(vec)
	jsonBytes, _ := json.Marshal(vec)
	const insertSQL = `
INSERT INTO chunk_embeddings (chunk_id, model, embedding_json, embedding_blob)
VALUES (?, 'model', ?, ?)`
	if _, err := sqlDB.Exec(insertSQL, "chunk1", string(jsonBytes), blob); err != nil {
		t.Fatalf("insert embedding with blob: %v", err)
	}

	embedder := &mockEmbedder{
		embeddings: [][]float32{{0.9, 0.1, 0.0}},
	}

	r := retrieval.New(sqlDB, embedder)
	ctx := context.Background()
	results, err := r.UserPrompt(ctx, "proj1", nil, "vector search", 5, false)
	if err != nil {
		t.Fatalf("UserPrompt BlobPath error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].ChunkID != "chunk1" {
		t.Errorf("expected chunk1, got %q", results[0].ChunkID)
	}
}

func TestVectorSearch_JSONFallback(t *testing.T) {
	// embedding_blob が NULL の場合は embedding_json にフォールバックする
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "json fallback content", "summary", "fact", 0.7, "project")

	// JSON のみ（blob なし）
	vec := []float32{1.0, 0.0, 0.0}
	jsonBytes, _ := json.Marshal(vec)
	const insertSQL = `
INSERT INTO chunk_embeddings (chunk_id, model, embedding_json)
VALUES (?, 'model', ?)`
	if _, err := sqlDB.Exec(insertSQL, "chunk1", string(jsonBytes)); err != nil {
		t.Fatalf("insert embedding json only: %v", err)
	}

	embedder := &mockEmbedder{
		embeddings: [][]float32{{0.9, 0.1, 0.0}},
	}

	r := retrieval.New(sqlDB, embedder)
	ctx := context.Background()
	results, err := r.UserPrompt(ctx, "proj1", nil, "fallback test", 5, false)
	if err != nil {
		t.Fatalf("UserPrompt JSONFallback error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result with JSON fallback")
	}
}

// --- RRF テスト ---

func TestRRF_SingleList(t *testing.T) {
	list := []retrieval.RankedResult{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.5},
		{ID: "c", Score: 0.1},
	}
	merged := retrieval.MergeRRF([][]retrieval.RankedResult{list}, 60)
	if len(merged) != 3 {
		t.Fatalf("expected 3 results, got %d", len(merged))
	}
	// ランク1が最高スコアを持つ
	if merged[0].ID != "a" {
		t.Errorf("expected top result 'a', got %q", merged[0].ID)
	}
}

func TestRRF_TwoLists(t *testing.T) {
	fts := []retrieval.RankedResult{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.5},
	}
	vec := []retrieval.RankedResult{
		{ID: "b", Score: 0.8},
		{ID: "c", Score: 0.6},
	}
	merged := retrieval.MergeRRF([][]retrieval.RankedResult{fts, vec}, 60)
	// "b" は両方にランクインしているので上位に来るはず
	found := false
	for i, r := range merged {
		if r.ID == "b" && i <= 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'b' to be in top-2 (appears in both lists)")
	}
}

func TestRRF_EmptyLists(t *testing.T) {
	merged := retrieval.MergeRRF([][]retrieval.RankedResult{}, 60)
	if len(merged) != 0 {
		t.Errorf("empty input should return empty, got %d", len(merged))
	}
}

// --- ProjectBoost テスト ---

func TestProjectBoost_SameProject(t *testing.T) {
	results := []retrieval.RankedResult{
		{ID: "a", Score: 1.0, ProjectID: "proj1", Scope: "project"},
		{ID: "b", Score: 1.0, ProjectID: "proj2", Scope: "similarity_shareable"},
	}
	similar := map[string]float64{"proj2": 0.8}
	boosted := retrieval.ApplyProjectBoost(results, "proj1", similar, false)

	// "a" (same project) が "b" (similar) より高いはず
	if boosted[0].ID != "a" {
		t.Errorf("same project should rank first, got %q", boosted[0].ID)
	}
}

func TestProjectBoost_SimilarProject(t *testing.T) {
	results := []retrieval.RankedResult{
		{ID: "a", Score: 1.0, ProjectID: "proj3", Scope: "global"},
		{ID: "b", Score: 1.0, ProjectID: "proj2", Scope: "similarity_shareable"},
	}
	similar := map[string]float64{"proj2": 0.8}
	boosted := retrieval.ApplyProjectBoost(results, "proj1", similar, false)

	// "b" (similar project boost) が "a" (other global) より高いはず
	if boosted[0].ID != "b" {
		t.Errorf("similar project should rank first, got %q", boosted[0].ID)
	}
}

// --- FTS 検索テスト ---

func TestFTSSearch_NoResults(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.FTSSearch(ctx, "completely nonexistent term xyz123", 10)
	if err != nil {
		t.Fatalf("FTSSearch error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent term, got %d", len(results))
	}
}

func TestFTSSearch_Basic(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "SQLite WAL mode is important for performance", "Use WAL mode", "decision", 0.8, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj1", "Go context cancellation pattern", "Use context.WithTimeout", "pattern", 0.7, "project")

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.FTSSearch(ctx, "SQLite WAL", 10)
	if err != nil {
		t.Fatalf("FTSSearch error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'SQLite WAL'")
	}
	if results[0].ID != "chunk1" {
		t.Errorf("expected chunk1 to rank first, got %q", results[0].ID)
	}
}

func TestFTSSearch_EmptyQuery(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	// 空クエリは空スライスを返す（エラーなし）
	results, err := r.FTSSearch(ctx, "", 10)
	if err != nil {
		t.Fatalf("FTSSearch empty query should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty query should return 0 results, got %d", len(results))
	}
}

// --- SessionStart retrieval テスト ---

func TestSessionStartRetrieval_Empty(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.SessionStart(ctx, "proj1", nil, 4, false)
	if err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty DB, got %d", len(results))
	}
}

func TestSessionStartRetrieval_WithChunks(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestProject(t, sqlDB, "proj2", "/test/project2")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "important decision about architecture", "arch decision", "decision", 0.9, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj1", "minor note", "minor", "fact", 0.2, "project")
	insertTestChunk(t, sqlDB, "chunk3", "proj2", "global constraint", "constraint", "constraint", 0.8, "global")

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.SessionStart(ctx, "proj1", nil, 4, false)
	if err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	// chunk1 (same project, high importance) が最上位のはず
	if results[0].ChunkID != "chunk1" {
		t.Errorf("expected chunk1 first (same project + high importance), got %q", results[0].ChunkID)
	}
}

func TestSessionStartRetrieval_RespectsLimit(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	for i := 0; i < 10; i++ {
		insertTestChunk(t, sqlDB,
			fmt.Sprintf("chunk%d", i), "proj1",
			fmt.Sprintf("content %d", i), fmt.Sprintf("summary %d", i),
			"fact", 0.5, "project",
		)
	}

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.SessionStart(ctx, "proj1", nil, 3, false)
	if err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

// --- UserPrompt retrieval テスト ---

func TestUserPromptRetrieval_EmptyDB(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.UserPrompt(ctx, "proj1", nil, "some query about Go", 5, false)
	if err != nil {
		t.Fatalf("UserPrompt error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty DB, got %d", len(results))
	}
}

func TestUserPromptRetrieval_FTSOnly(t *testing.T) {
	// embedding worker なし（degraded mode）でも FTS で動作する
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "SQLite FTS5 full text search implementation", "FTS5 usage", "decision", 0.8, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj1", "Go goroutine best practices", "goroutine tips", "pattern", 0.7, "project")

	r := retrieval.New(sqlDB, nil) // embedder = nil (degraded mode)
	ctx := context.Background()

	results, err := r.UserPrompt(ctx, "proj1", nil, "full text search", 5, false)
	if err != nil {
		t.Fatalf("UserPrompt FTSOnly error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result from FTS")
	}
	if results[0].ChunkID != "chunk1" {
		t.Errorf("expected chunk1 (FTS match) first, got %q", results[0].ChunkID)
	}
}

func TestUserPromptRetrieval_WithEmbedding(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "vector similarity search", "vec search", "decision", 0.8, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj1", "completely different topic about cooking", "cooking", "fact", 0.5, "project")

	// chunk1 の embedding を挿入
	insertTestEmbedding(t, sqlDB, "chunk1", []float32{1.0, 0.0, 0.0})
	insertTestEmbedding(t, sqlDB, "chunk2", []float32{0.0, 1.0, 0.0})

	// query は chunk1 に近い embedding
	embedder := &mockEmbedder{
		embeddings: [][]float32{{0.9, 0.1, 0.0}},
	}

	r := retrieval.New(sqlDB, embedder)
	ctx := context.Background()

	results, err := r.UserPrompt(ctx, "proj1", nil, "vector search", 5, false)
	if err != nil {
		t.Fatalf("UserPrompt WithEmbedding error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestUserPromptRetrieval_ProjectBoostWins(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project1")
	insertTestProject(t, sqlDB, "proj2", "/test/project2")

	// proj2 の chunk（FTS スコア高め）と proj1 の chunk（FTS スコア低め）
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "SQLite query optimization tips", "sqlite", "decision", 0.6, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj2", "SQLite query optimization best practices", "sqlite", "decision", 0.9, "global")

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.UserPrompt(ctx, "proj1", nil, "SQLite query", 5, false)
	if err != nil {
		t.Fatalf("UserPrompt error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	// proj1 の chunk が project boost により上位に来るはず
	if results[0].ChunkID != "chunk1" {
		t.Errorf("expected chunk1 (same project boost) first, got %q", results[0].ChunkID)
	}
}

// --- 日本語 FTS テスト ---

func TestFTSSearch_Japanese(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "ワーカーの起動に失敗しました", "ワーカー起動エラー", "failure", 0.8, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj1", "SQLite の FTS5 を使った全文検索", "全文検索の実装", "decision", 0.7, "project")

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	// 「ワーカー」で検索 → 1件ヒット
	results, err := r.FTSSearch(ctx, "ワーカー", 10)
	if err != nil {
		t.Fatalf("FTSSearch Japanese error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'ワーカー', got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != "chunk1" {
		t.Errorf("expected chunk1, got %q", results[0].ID)
	}

	// 「全文検索」で検索 → 1件ヒット
	results2, err := r.FTSSearch(ctx, "全文検索", 10)
	if err != nil {
		t.Fatalf("FTSSearch Japanese 2 error: %v", err)
	}
	if len(results2) != 1 {
		t.Errorf("expected 1 result for '全文検索', got %d", len(results2))
	}
	if len(results2) > 0 && results2[0].ID != "chunk2" {
		t.Errorf("expected chunk2, got %q", results2[0].ID)
	}
}

func TestFTSSearch_ShortTokenFiltered(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project")
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "Go is a programming language", "Go lang", "fact", 0.5, "project")
	insertTestChunk(t, sqlDB, "chunk2", "proj1", "ワーカーの起動に失敗", "ワーカー失敗", "failure", 0.8, "project")

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	// 「失敗」(2文字) で検索 → 0件（trigram は 3 文字未満をフィルタ）
	results, err := r.FTSSearch(ctx, "失敗", 10)
	if err != nil {
		t.Fatalf("FTSSearch ShortToken error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for '失敗' (2-char, filtered), got %d", len(results))
	}

	// 「Go is」(両方3文字未満) で検索 → 0件
	results2, err := r.FTSSearch(ctx, "Go is", 10)
	if err != nil {
		t.Fatalf("FTSSearch ShortToken2 error: %v", err)
	}
	if len(results2) != 0 {
		t.Errorf("expected 0 results for 'Go is' (all tokens < 3 chars), got %d", len(results2))
	}
}

// --- FormatContext テスト ---

func TestFormatContext_Empty(t *testing.T) {
	results := []retrieval.Result{}
	got := retrieval.FormatContext(results)
	if got != "" {
		t.Errorf("empty results should return empty string, got %q", got)
	}
}

func TestFormatContext_NonEmpty(t *testing.T) {
	results := []retrieval.Result{
		{
			ChunkID:    "c1",
			Content:    "Use WAL mode for SQLite",
			Summary:    "WAL mode is important",
			Kind:       "decision",
			Importance: 0.8,
		},
	}
	got := retrieval.FormatContext(results)
	if got == "" {
		t.Error("non-empty results should return non-empty string")
	}
	// 種別と重要度が含まれるか確認
	if len(got) < 10 {
		t.Errorf("formatted context too short: %q", got)
	}
}

// --- Isolation / scope テスト ---

// TestSessionStart_IsolatedProject は isolated プロジェクトで SessionStart を呼んだ場合に
// 自プロジェクトのチャンクのみ返ることを確認する。
func TestSessionStart_IsolatedProject(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project1")
	insertTestProject(t, sqlDB, "proj2", "/test/project2")

	// proj1 のチャンク
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "proj1 local decision", "p1 decision", "decision", 0.8, "project")
	// proj2 の global スコープチャンク（通常なら流入するが isolated では来ない）
	insertTestChunk(t, sqlDB, "chunk2", "proj2", "globally shared knowledge", "global fact", "fact", 0.9, "global")

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	// isolated=true で呼び出す
	results, err := r.SessionStart(ctx, "proj1", nil, 4, true)
	if err != nil {
		t.Fatalf("SessionStart isolated error: %v", err)
	}

	// proj1 のチャンクのみ返ること
	for _, res := range results {
		if res.ProjectID != "proj1" {
			t.Errorf("isolated project: unexpected chunk from project %q (chunkID=%q)", res.ProjectID, res.ChunkID)
		}
	}

	// chunk1 が含まれること
	found := false
	for _, res := range results {
		if res.ChunkID == "chunk1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected chunk1 (own project) in isolated results")
	}
}

// TestSessionStart_SimilarityShareable は類似プロジェクトの similarity_shareable チャンクが
// 返り、非類似プロジェクトの shareable は返らないことを確認する。
func TestSessionStart_SimilarityShareable(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	insertTestProject(t, sqlDB, "proj1", "/test/project1")
	insertTestProject(t, sqlDB, "proj2", "/test/project2") // 類似プロジェクト
	insertTestProject(t, sqlDB, "proj3", "/test/project3") // 非類似プロジェクト

	// proj1 のチャンク
	insertTestChunk(t, sqlDB, "chunk1", "proj1", "proj1 own content", "own", "fact", 0.5, "project")
	// proj2 の similarity_shareable チャンク（類似プロジェクトなので返るはず）
	insertTestChunk(t, sqlDB, "chunk2", "proj2", "similar project shareable content", "similar shareable", "pattern", 0.7, "similarity_shareable")
	// proj3 の similarity_shareable チャンク（非類似プロジェクトなので返らないはず）
	insertTestChunk(t, sqlDB, "chunk3", "proj3", "unrelated project shareable content", "unrelated shareable", "pattern", 0.8, "similarity_shareable")

	// proj2 のみ類似プロジェクト
	similarProjects := map[string]float64{"proj2": 0.8}

	r := retrieval.New(sqlDB, nil)
	ctx := context.Background()

	results, err := r.SessionStart(ctx, "proj1", similarProjects, 10, false)
	if err != nil {
		t.Fatalf("SessionStart SimilarityShareable error: %v", err)
	}

	chunkIDs := make(map[string]bool)
	for _, res := range results {
		chunkIDs[res.ChunkID] = true
	}

	// chunk2（類似プロジェクトの shareable）は含まれるはず
	if !chunkIDs["chunk2"] {
		t.Error("expected chunk2 (similar project's similarity_shareable) in results")
	}

	// chunk3（非類似プロジェクトの shareable）は含まれないはず
	if chunkIDs["chunk3"] {
		t.Error("unexpected chunk3 (non-similar project's similarity_shareable) in results")
	}
}

// TestApplyProjectBoost_ScopeAware は scope に応じた boost/penalty が正しく適用されることを確認する。
func TestApplyProjectBoost_ScopeAware(t *testing.T) {
	results := []retrieval.RankedResult{
		{ID: "same-proj", Score: 1.0, ProjectID: "proj1", Scope: "project"},
		{ID: "similar-global", Score: 1.0, ProjectID: "proj2", Scope: "global"},
		{ID: "similar-shareable", Score: 1.0, ProjectID: "proj2", Scope: "similarity_shareable"},
		{ID: "similar-project", Score: 1.0, ProjectID: "proj2", Scope: "project"},
		{ID: "other-global", Score: 1.0, ProjectID: "proj3", Scope: "global"},
		{ID: "other-shareable", Score: 1.0, ProjectID: "proj3", Scope: "similarity_shareable"},
		{ID: "other-project", Score: 1.0, ProjectID: "proj3", Scope: "project"},
	}
	similarProjects := map[string]float64{"proj2": 0.8}

	boosted := retrieval.ApplyProjectBoost(results, "proj1", similarProjects, false)

	// スコアをマップに変換
	scores := make(map[string]float64)
	for _, rr := range boosted {
		scores[rr.ID] = rr.Score
	}

	// 同プロジェクト: +2.0
	if scores["same-proj"] != 3.0 {
		t.Errorf("same project: expected score 3.0, got %f", scores["same-proj"])
	}

	// 類似プロジェクト global: +1.0
	if scores["similar-global"] != 2.0 {
		t.Errorf("similar global: expected score 2.0, got %f", scores["similar-global"])
	}

	// 類似プロジェクト similarity_shareable: +1.0
	if scores["similar-shareable"] != 2.0 {
		t.Errorf("similar shareable: expected score 2.0, got %f", scores["similar-shareable"])
	}

	// 類似プロジェクト project scope: -3.0
	if scores["similar-project"] != -2.0 {
		t.Errorf("similar project scope: expected score -2.0, got %f", scores["similar-project"])
	}

	// 他プロジェクト global: boost なし、penalty なし
	if scores["other-global"] != 1.0 {
		t.Errorf("other global: expected score 1.0, got %f", scores["other-global"])
	}

	// 他プロジェクト similarity_shareable: -1.0
	if scores["other-shareable"] != 0.0 {
		t.Errorf("other shareable: expected score 0.0, got %f", scores["other-shareable"])
	}

	// 他プロジェクト project scope: -3.0
	if scores["other-project"] != -2.0 {
		t.Errorf("other project scope: expected score -2.0, got %f", scores["other-project"])
	}
}

// TestApplyProjectBoost_Isolated は isolated=true の場合に他プロジェクトのスコアが -999 になることを確認する。
func TestApplyProjectBoost_Isolated(t *testing.T) {
	results := []retrieval.RankedResult{
		{ID: "own", Score: 1.0, ProjectID: "proj1", Scope: "project"},
		{ID: "other-global", Score: 1.0, ProjectID: "proj2", Scope: "global"},
		{ID: "other-project", Score: 1.0, ProjectID: "proj2", Scope: "project"},
	}

	boosted := retrieval.ApplyProjectBoost(results, "proj1", nil, true)

	scores := make(map[string]float64)
	for _, rr := range boosted {
		scores[rr.ID] = rr.Score
	}

	// 自プロジェクト: スコアそのまま
	if scores["own"] != 1.0 {
		t.Errorf("isolated own: expected score 1.0, got %f", scores["own"])
	}

	// 他プロジェクト（global であっても）: -999
	if scores["other-global"] != -999 {
		t.Errorf("isolated other-global: expected score -999, got %f", scores["other-global"])
	}
	if scores["other-project"] != -999 {
		t.Errorf("isolated other-project: expected score -999, got %f", scores["other-project"])
	}
}
