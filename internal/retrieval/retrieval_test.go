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
		{ID: "a", Score: 1.0, ProjectID: "proj1"},
		{ID: "b", Score: 1.0, ProjectID: "proj2"},
	}
	similar := map[string]float64{"proj2": 0.8}
	boosted := retrieval.ApplyProjectBoost(results, "proj1", similar)

	// "a" (same project) が "b" (similar) より高いはず
	if boosted[0].ID != "a" {
		t.Errorf("same project should rank first, got %q", boosted[0].ID)
	}
}

func TestProjectBoost_SimilarProject(t *testing.T) {
	results := []retrieval.RankedResult{
		{ID: "a", Score: 1.0, ProjectID: "proj3"},
		{ID: "b", Score: 1.0, ProjectID: "proj2"},
	}
	similar := map[string]float64{"proj2": 0.8}
	boosted := retrieval.ApplyProjectBoost(results, "proj1", similar)

	// "b" (similar project boost) が "a" (global) より高いはず
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

	results, err := r.SessionStart(ctx, "proj1", nil, 4)
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

	results, err := r.SessionStart(ctx, "proj1", nil, 4)
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

	results, err := r.SessionStart(ctx, "proj1", nil, 3)
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

	results, err := r.UserPrompt(ctx, "proj1", nil, "some query about Go", 5)
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

	results, err := r.UserPrompt(ctx, "proj1", nil, "full text search", 5)
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

	results, err := r.UserPrompt(ctx, "proj1", nil, "vector search", 5)
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

	results, err := r.UserPrompt(ctx, "proj1", nil, "SQLite query", 5)
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
