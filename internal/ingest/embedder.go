package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/youyo/memoria/internal/retrieval"
)

// EmbedClient は embedding worker への通信インターフェース。
// embedding.Client の必要なメソッドのみを定義し、テスト時にモック可能にする。
type EmbedClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Embedder は chunk_embeddings への保存を抽象化するインターフェース。
// テスト時にモックに差し替え可能。
type Embedder interface {
	EmbedChunks(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error
}

// ChunkEmbedder は EmbedClient を使って chunk_embeddings テーブルに保存する。
type ChunkEmbedder struct {
	client EmbedClient
}

// NewChunkEmbedder は ChunkEmbedder を生成する。
func NewChunkEmbedder(client EmbedClient) *ChunkEmbedder {
	return &ChunkEmbedder{client: client}
}

// EmbedChunks は chunkIDs の content を一括で embed して chunk_embeddings に保存する。
//
// 処理フロー:
// 1. chunkIDs が空なら即返す
// 2. chunk_embeddings に既存の chunk_id を確認し、未 embed のみに絞り込む
// 3. chunks テーブルから content を取得
// 4. client.Embed() でバッチ embedding
// 5. 結果を JSON 文字列に変換して chunk_embeddings に INSERT OR IGNORE（冪等）
//
// embedding worker が応答しない場合はエラーを返す。
// 呼び出し側（HandleCheckpoint/HandleSessionEnd）は embedding エラーをスキップ（warn ログのみ）する設計。
func (e *ChunkEmbedder) EmbedChunks(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error {
	if len(chunkIDs) == 0 {
		return nil
	}

	// 既に embedding 済みの chunk_id を確認
	embeddedSet, err := queryEmbeddedChunkIDs(ctx, db, chunkIDs)
	if err != nil {
		return fmt.Errorf("query existing embeddings: %w", err)
	}

	// 未 embed の chunk_id を絞り込む
	pendingIDs := make([]string, 0, len(chunkIDs))
	for _, id := range chunkIDs {
		if !embeddedSet[id] {
			pendingIDs = append(pendingIDs, id)
		}
	}
	if len(pendingIDs) == 0 {
		return nil
	}

	// chunks テーブルから content を取得（順序保持）
	contents, orderedIDs, err := fetchChunkContents(ctx, db, pendingIDs)
	if err != nil {
		return fmt.Errorf("fetch chunk contents: %w", err)
	}
	if len(contents) == 0 {
		return nil
	}

	// バッチ embedding
	// TODO: MVP では全件一括。件数が多い場合はバッチサイズ上限を設けること（将来の拡張ポイント）
	embeddings, err := e.client.Embed(ctx, contents)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	if len(embeddings) != len(orderedIDs) {
		return fmt.Errorf("embedding count mismatch: expected %d, got %d", len(orderedIDs), len(embeddings))
	}

	// 結果を chunk_embeddings に保存（JSON + blob の両形式で保存）
	for i, chunkID := range orderedIDs {
		jsonBytes, err := json.Marshal(embeddings[i])
		if err != nil {
			return fmt.Errorf("marshal embedding for chunk %s: %w", chunkID, err)
		}
		blob := retrieval.Float32SliceToBytes(embeddings[i])

		const insertQuery = `
INSERT OR IGNORE INTO chunk_embeddings (chunk_id, model, embedding_json, embedding_blob, created_at)
VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`

		if _, err := db.ExecContext(ctx, insertQuery, chunkID, modelName, string(jsonBytes), blob); err != nil {
			return fmt.Errorf("insert chunk_embedding for chunk %s: %w", chunkID, err)
		}
	}

	return nil
}

// queryEmbeddedChunkIDs は chunkIDs のうち chunk_embeddings テーブルに既に存在する chunk_id の集合を返す。
func queryEmbeddedChunkIDs(ctx context.Context, db *sql.DB, chunkIDs []string) (map[string]bool, error) {
	if len(chunkIDs) == 0 {
		return map[string]bool{}, nil
	}

	// placeholders を動的生成
	args := make([]any, len(chunkIDs))
	for i, id := range chunkIDs {
		args[i] = id
	}

	query := buildInQuery("SELECT chunk_id FROM chunk_embeddings WHERE chunk_id IN", len(chunkIDs))
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query chunk_embeddings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool, len(chunkIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan chunk_id: %w", err)
		}
		result[id] = true
	}
	return result, rows.Err()
}

// fetchChunkContents は chunkIDs の content を chunks テーブルから取得する。
// 返り値は (contents, orderedIDs) で、chunkIDs の順序と一致する。
// chunks テーブルに存在しない ID は無視する。
func fetchChunkContents(ctx context.Context, db *sql.DB, chunkIDs []string) ([]string, []string, error) {
	if len(chunkIDs) == 0 {
		return nil, nil, nil
	}

	args := make([]any, len(chunkIDs))
	for i, id := range chunkIDs {
		args[i] = id
	}

	query := buildInQuery("SELECT chunk_id, content FROM chunks WHERE chunk_id IN", len(chunkIDs))
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	// chunk_id → content のマップを構築
	contentMap := make(map[string]string, len(chunkIDs))
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, nil, fmt.Errorf("scan chunk: %w", err)
		}
		contentMap[id] = content
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}

	// chunkIDs の順序を保持して結果を構築
	orderedIDs := make([]string, 0, len(chunkIDs))
	contents := make([]string, 0, len(chunkIDs))
	for _, id := range chunkIDs {
		if content, ok := contentMap[id]; ok {
			orderedIDs = append(orderedIDs, id)
			contents = append(contents, content)
		}
	}
	return contents, orderedIDs, nil
}

// buildInQuery は "SELECT ... WHERE col IN (?, ?, ...)" 形式のクエリを構築する。
func buildInQuery(prefix string, count int) string {
	placeholders := make([]byte, 0, count*3-1)
	for i := 0; i < count; i++ {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
	}
	return prefix + " (" + string(placeholders) + ")"
}
