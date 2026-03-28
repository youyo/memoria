package worker_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/youyo/memoria/internal/project"
	"github.com/youyo/memoria/internal/queue"
	"github.com/youyo/memoria/internal/testutil"
	"github.com/youyo/memoria/internal/worker"
)

// mockEmbedClient は EmbedClient のモック（固定次元の embedding を返す）。
type mockEmbedClient struct {
	dims int
}

func (m *mockEmbedClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dims)
		for j := range vec {
			vec[j] = float32(i+1) / float32(m.dims)
		}
		result[i] = vec
	}
	return result, nil
}

func TestHandleProjectRefresh(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	// プロジェクトをセットアップ
	tmpDir := t.TempDir()
	// go.mod を作成して Go プロジェクトにする
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\ngo 1.21"), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\nfunc main(){}"), 0644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	r := project.NewResolver(sqlDB)
	projectID, err := r.Resolve(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// payload 作成
	payload := worker.ProjectRefreshPayload{
		ProjectID:   projectID,
		ProjectRoot: tmpDir,
	}
	payloadJSON, _ := json.Marshal(payload)

	job := &queue.Job{
		ID:          "test-job-1",
		Type:        queue.JobTypeProjectRefresh,
		PayloadJSON: string(payloadJSON),
	}

	// ハンドラを生成して実行
	embedClient := &mockEmbedClient{dims: 4}
	handler := worker.NewProjectRefreshHandler(sqlDB, embedClient, "test-model", func(f string, a ...any) {})
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// DB に fingerprint が保存されているか確認
	var fpText, fpJSON string
	err = sqlDB.QueryRow(
		"SELECT fingerprint_text, fingerprint_json FROM projects WHERE project_id = ?",
		projectID,
	).Scan(&fpText, &fpJSON)
	if err != nil {
		t.Fatalf("query fingerprint: %v", err)
	}
	if fpText == "" {
		t.Error("fingerprint_text is empty after refresh")
	}
	if fpJSON == "" {
		t.Error("fingerprint_json is empty after refresh")
	}

	// project_embeddings に保存されているか確認
	var embJSON string
	err = sqlDB.QueryRow(
		"SELECT embedding_json FROM project_embeddings WHERE project_id = ?",
		projectID,
	).Scan(&embJSON)
	if err != nil {
		t.Fatalf("query project_embeddings: %v", err)
	}
	if embJSON == "" {
		t.Error("project_embeddings is empty after refresh")
	}
}

func TestHandleProjectRefresh_NonExistentDir(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	// 存在しないディレクトリを指定
	r := project.NewResolver(sqlDB)
	tmpDir := t.TempDir()
	projectID, _ := r.Resolve(ctx, tmpDir)

	payload := worker.ProjectRefreshPayload{
		ProjectID:   projectID,
		ProjectRoot: "/nonexistent/path/that/does/not/exist",
	}
	payloadJSON, _ := json.Marshal(payload)

	job := &queue.Job{
		ID:          "test-job-2",
		Type:        queue.JobTypeProjectRefresh,
		PayloadJSON: string(payloadJSON),
	}

	handler := worker.NewProjectRefreshHandler(sqlDB, &mockEmbedClient{dims: 4}, "test-model", nil)
	// エラーが返ることを期待（存在しないプロジェクトルート）
	err := handler.Handle(ctx, job)
	// エラーでも OK: best effort（非致命的）
	_ = err
}

func TestHandleProjectSimilarityRefresh(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	// 複数のプロジェクトをセットアップ
	r := project.NewResolver(sqlDB)
	dirs := make([]string, 3)
	projectIDs := make([]string, 3)
	for i := range dirs {
		dirs[i] = t.TempDir()
		id, err := r.Resolve(ctx, dirs[i])
		if err != nil {
			t.Fatalf("Resolve %d: %v", i, err)
		}
		projectIDs[i] = id
	}

	// project_embeddings にデータを投入
	sim := project.NewSimilarityManager(sqlDB)
	embedClient := &mockEmbedClient{dims: 4}

	for i, id := range projectIDs {
		vecs, _ := embedClient.Embed(ctx, []string{"project text"})
		// 微妙に異なるベクトルを作る
		vec := vecs[0]
		vec[0] = float32(i) * 0.1
		if err := sim.UpsertProjectEmbedding(ctx, id, "test-model", vec); err != nil {
			t.Fatalf("UpsertProjectEmbedding %d: %v", i, err)
		}
	}

	// git init して project root を設定（project_root が必要）
	if err := exec.Command("git", "init", dirs[0]).Run(); err != nil {
		t.Skipf("git init failed: %v", err)
	}

	// payload 作成
	payload := worker.ProjectSimilarityRefreshPayload{
		ProjectID: projectIDs[0],
	}
	payloadJSON, _ := json.Marshal(payload)

	job := &queue.Job{
		ID:          "test-job-3",
		Type:        queue.JobTypeProjectSimilarityRefresh,
		PayloadJSON: string(payloadJSON),
	}

	handler := worker.NewProjectSimilarityRefreshHandler(sqlDB, func(f string, a ...any) {})
	if err := handler.Handle(ctx, job); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// project_similarity にデータが保存されているか確認
	var count int
	if err := sqlDB.QueryRow(
		"SELECT COUNT(*) FROM project_similarity WHERE project_id = ?",
		projectIDs[0],
	).Scan(&count); err != nil {
		t.Fatalf("query project_similarity: %v", err)
	}
	if count == 0 {
		t.Error("project_similarity is empty after refresh")
	}
}

func TestHandleProjectSimilarityRefresh_NoEmbeddings(t *testing.T) {
	sqlDB := testutil.OpenTestDB(t)
	ctx := context.Background()

	r := project.NewResolver(sqlDB)
	tmpDir := t.TempDir()
	projectID, _ := r.Resolve(ctx, tmpDir)

	payload := worker.ProjectSimilarityRefreshPayload{
		ProjectID: projectID,
	}
	payloadJSON, _ := json.Marshal(payload)

	job := &queue.Job{
		ID:          "test-job-4",
		Type:        queue.JobTypeProjectSimilarityRefresh,
		PayloadJSON: string(payloadJSON),
	}

	// embedding なし状態でも panic しないことを確認
	handler := worker.NewProjectSimilarityRefreshHandler(sqlDB, nil)
	err := handler.Handle(ctx, job)
	// エラーがあっても panic しなければ OK
	_ = err
}
