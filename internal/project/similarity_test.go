package project_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/youyo/memoria/internal/project"
)

func TestGetSimilarProjects_Empty(t *testing.T) {
	sqlDB := openTestDB(t)
	sim := project.NewSimilarityManager(sqlDB)

	ctx := context.Background()
	result, err := sim.GetSimilarProjects(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("GetSimilarProjects: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestGetSimilarProjects_WithData(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	// 2つのプロジェクトをセットアップ
	r := project.NewResolver(sqlDB)
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	id1, err := r.Resolve(ctx, tmpDir1)
	if err != nil {
		t.Fatalf("Resolve 1: %v", err)
	}
	id2, err := r.Resolve(ctx, tmpDir2)
	if err != nil {
		t.Fatalf("Resolve 2: %v", err)
	}

	// project_similarity にデータを挿入
	sim := project.NewSimilarityManager(sqlDB)
	if err := sim.UpsertSimilarity(ctx, id1, id2, 0.85); err != nil {
		t.Fatalf("UpsertSimilarity: %v", err)
	}

	// 取得確認
	result, err := sim.GetSimilarProjects(ctx, id1)
	if err != nil {
		t.Fatalf("GetSimilarProjects: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected similar projects, got empty map")
	}
	if score, ok := result[id2]; !ok {
		t.Errorf("expected id2 %q in result", id2)
	} else if score < 0.84 || score > 0.86 {
		t.Errorf("similarity = %.4f, want ~0.85", score)
	}
}

func TestUpsertSimilarity_Idempotent(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	id1, _ := r.Resolve(ctx, tmpDir1)
	id2, _ := r.Resolve(ctx, tmpDir2)

	sim := project.NewSimilarityManager(sqlDB)

	// 同じペアを2回 upsert する
	if err := sim.UpsertSimilarity(ctx, id1, id2, 0.7); err != nil {
		t.Fatalf("UpsertSimilarity 1st: %v", err)
	}
	if err := sim.UpsertSimilarity(ctx, id1, id2, 0.9); err != nil {
		t.Fatalf("UpsertSimilarity 2nd: %v", err)
	}

	// 最新の値（0.9）が反映されていることを確認
	result, err := sim.GetSimilarProjects(ctx, id1)
	if err != nil {
		t.Fatalf("GetSimilarProjects: %v", err)
	}
	if score, ok := result[id2]; !ok {
		t.Error("id2 not in result")
	} else if score < 0.89 || score > 0.91 {
		t.Errorf("similarity = %.4f after update, want ~0.9", score)
	}

	// project_similarity の行数が1件のみであることを確認
	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM project_similarity WHERE project_id = ? AND similar_project_id = ?", id1, id2).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("project_similarity count = %d, want 1", count)
	}
}

func TestUpsertProjectEmbedding(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir := t.TempDir()
	projectID, _ := r.Resolve(ctx, tmpDir)

	sim := project.NewSimilarityManager(sqlDB)

	vec := []float32{0.1, 0.2, 0.3}
	if err := sim.UpsertProjectEmbedding(ctx, projectID, "test-model", vec); err != nil {
		t.Fatalf("UpsertProjectEmbedding: %v", err)
	}

	// DB から取得して確認
	var embJSON string
	if err := sqlDB.QueryRow("SELECT embedding_json FROM project_embeddings WHERE project_id = ?", projectID).Scan(&embJSON); err != nil {
		t.Fatalf("query project_embeddings: %v", err)
	}
	var got []float32
	if err := json.Unmarshal([]byte(embJSON), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("embedding length = %d, want 3", len(got))
	}
}

func TestIsSimilarityFresh_NoData(t *testing.T) {
	sqlDB := openTestDB(t)
	sim := project.NewSimilarityManager(sqlDB)

	ctx := context.Background()
	fresh := sim.IsSimilarityFresh(ctx, "nonexistent-id", 7*24*time.Hour)
	if fresh {
		t.Error("IsSimilarityFresh should return false for nonexistent project")
	}
}

func TestIsSimilarityFresh_WithData(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	id1, _ := r.Resolve(ctx, tmpDir1)
	id2, _ := r.Resolve(ctx, tmpDir2)

	sim := project.NewSimilarityManager(sqlDB)
	if err := sim.UpsertSimilarity(ctx, id1, id2, 0.8); err != nil {
		t.Fatalf("UpsertSimilarity: %v", err)
	}

	// 直後は fresh
	fresh := sim.IsSimilarityFresh(ctx, id1, 7*24*time.Hour)
	if !fresh {
		t.Error("IsSimilarityFresh should return true for freshly upserted data")
	}

	// 0 秒 TTL は stale
	stale := sim.IsSimilarityFresh(ctx, id1, 0)
	if stale {
		t.Error("IsSimilarityFresh should return false for 0 TTL")
	}
}

func TestIsFingerprintFresh_NoData(t *testing.T) {
	sqlDB := openTestDB(t)
	sim := project.NewSimilarityManager(sqlDB)

	ctx := context.Background()
	fresh := sim.IsFingerprintFresh(ctx, "nonexistent-id", 24*time.Hour)
	if fresh {
		t.Error("IsFingerprintFresh should return false for nonexistent project")
	}
}

func TestIsFingerprintFresh_WithData(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir := t.TempDir()
	projectID, _ := r.Resolve(ctx, tmpDir)

	sim := project.NewSimilarityManager(sqlDB)

	// fingerprint_updated_at を設定
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := sqlDB.ExecContext(ctx, "UPDATE projects SET fingerprint_updated_at = ? WHERE project_id = ?", now, projectID)
	if err != nil {
		t.Fatalf("UPDATE: %v", err)
	}

	// 直後は fresh
	if !sim.IsFingerprintFresh(ctx, projectID, 24*time.Hour) {
		t.Error("IsFingerprintFresh should return true for freshly updated fingerprint")
	}

	// 0 秒 TTL は stale
	if sim.IsFingerprintFresh(ctx, projectID, 0) {
		t.Error("IsFingerprintFresh should return false for 0 TTL")
	}
}

func TestGetProjectEmbedding_NotFound(t *testing.T) {
	sqlDB := openTestDB(t)
	sim := project.NewSimilarityManager(sqlDB)

	ctx := context.Background()
	_, found, err := sim.GetProjectEmbedding(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("GetProjectEmbedding: %v", err)
	}
	if found {
		t.Error("should return found=false for nonexistent project")
	}
}

func TestGetAllProjectEmbeddings(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	id1, _ := r.Resolve(ctx, tmpDir1)
	id2, _ := r.Resolve(ctx, tmpDir2)

	sim := project.NewSimilarityManager(sqlDB)
	sim.UpsertProjectEmbedding(ctx, id1, "model-a", []float32{0.1, 0.2})
	sim.UpsertProjectEmbedding(ctx, id2, "model-a", []float32{0.3, 0.4})

	all, err := sim.GetAllProjectEmbeddings(ctx)
	if err != nil {
		t.Fatalf("GetAllProjectEmbeddings: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("GetAllProjectEmbeddings() returned %d items, want >= 2", len(all))
	}
}

func TestUpdateFingerprintDB(t *testing.T) {
	sqlDB := openTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir := t.TempDir()
	projectID, _ := r.Resolve(ctx, tmpDir)

	sim := project.NewSimilarityManager(sqlDB)
	err := sim.UpdateFingerprintDB(ctx, projectID, `{"repo_name":"test"}`, "test project using Go.", "Go", "cli")
	if err != nil {
		t.Fatalf("UpdateFingerprintDB: %v", err)
	}

	// DB から確認
	var fpJSON, fpText, lang string
	if err := sqlDB.QueryRow(
		"SELECT fingerprint_json, fingerprint_text, primary_language FROM projects WHERE project_id = ?",
		projectID,
	).Scan(&fpJSON, &fpText, &lang); err != nil {
		t.Fatalf("query: %v", err)
	}
	if fpJSON == "" {
		t.Error("fingerprint_json is empty")
	}
	if fpText == "" {
		t.Error("fingerprint_text is empty")
	}
	if lang != "Go" {
		t.Errorf("primary_language = %q, want Go", lang)
	}
}
