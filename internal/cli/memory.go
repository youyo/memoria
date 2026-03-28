package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	"github.com/youyo/memoria/internal/db"
	"github.com/youyo/memoria/internal/retrieval"
)

// MemoryCmd はメモリ操作サブコマンドグループを定義する。
type MemoryCmd struct {
	Search  MemorySearchCmd  `cmd:"" help:"メモリを検索する"`
	Get     MemoryGetCmd     `cmd:"" help:"メモリを ID で取得する"`
	List    MemoryListCmd    `cmd:"" help:"メモリ一覧を表示する"`
	Stats   MemoryStatsCmd   `cmd:"" help:"メモリ統計を表示する"`
	Reindex MemoryReindexCmd `cmd:"" help:"メモリのインデックスを再構築する"`
}

// MemorySearchCmd は memory search コマンド。
type MemorySearchCmd struct {
	Query string `arg:"" help:"検索クエリ"`
}

// Run は memory search を実行する。
func (c *MemorySearchCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryGetCmd は memory get コマンド。
type MemoryGetCmd struct {
	ID string `arg:"" help:"メモリ ID"`
}

// Run は memory get を実行する。
func (c *MemoryGetCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryListCmd は memory list コマンド。
type MemoryListCmd struct{}

// Run は memory list を実行する。
func (c *MemoryListCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryStatsCmd は memory stats コマンド。
type MemoryStatsCmd struct{}

// Run は memory stats を実行する。
func (c *MemoryStatsCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryReindexCmd は memory reindex コマンド。
// 既存の JSON blob 形式の embedding を float32 バイナリ（BLOB）形式に変換する。
type MemoryReindexCmd struct {
	BatchSize int  `help:"バッチサイズ" default:"100"`
	DryRun    bool `help:"実際に書き込まず変換数のみ表示する" name:"dry-run"`
}

// Run は memory reindex を実行する。
func (c *MemoryReindexCmd) Run(globals *Globals, w *io.Writer, database *db.DB) error {
	ctx := context.Background()
	sqlDB := database.SQL()

	chunkCount, err := reindexChunkEmbeddings(ctx, sqlDB, c.BatchSize, c.DryRun)
	if err != nil {
		return fmt.Errorf("chunk embeddings reindex: %w", err)
	}

	projectCount, err := reindexProjectEmbeddings(ctx, sqlDB, c.BatchSize, c.DryRun)
	if err != nil {
		return fmt.Errorf("project embeddings reindex: %w", err)
	}

	if c.DryRun {
		fmt.Fprintf(*w, "dry-run: chunk_embeddings=%d 件, project_embeddings=%d 件を変換予定\n", chunkCount, projectCount)
	} else {
		fmt.Fprintf(*w, "完了: chunk_embeddings=%d 件, project_embeddings=%d 件を blob に変換\n", chunkCount, projectCount)
	}
	return nil
}

// reindexChunkEmbeddings は chunk_embeddings テーブルの JSON blob を blob 形式に変換する。
// 変換件数を返す。
func reindexChunkEmbeddings(ctx context.Context, sqlDB *sql.DB, batchSize int, dryRun bool) (int, error) {
	const selectQuery = `
SELECT chunk_id, embedding_json
FROM chunk_embeddings
WHERE embedding_blob IS NULL
LIMIT ?`

	total := 0
	for {
		rows, err := sqlDB.QueryContext(ctx, selectQuery, batchSize)
		if err != nil {
			return total, fmt.Errorf("query chunk_embeddings: %w", err)
		}

		type row struct {
			chunkID       string
			embeddingJSON string
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.chunkID, &r.embeddingJSON); err != nil {
				rows.Close()
				return total, fmt.Errorf("scan chunk_embedding: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("rows error: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		if !dryRun {
			for _, r := range batch {
				vec, err := embeddingJSONToFloat32(r.embeddingJSON)
				if err != nil {
					// パース失敗はスキップ（破損データ）
					continue
				}
				blob := retrieval.Float32SliceToBytes(vec)
				const updateQuery = `UPDATE chunk_embeddings SET embedding_blob = ? WHERE chunk_id = ?`
				if _, err := sqlDB.ExecContext(ctx, updateQuery, blob, r.chunkID); err != nil {
					return total, fmt.Errorf("update chunk_embedding blob for %s: %w", r.chunkID, err)
				}
				total++
			}
		} else {
			total += len(batch)
		}

		if len(batch) < batchSize {
			break
		}
	}
	return total, nil
}

// reindexProjectEmbeddings は project_embeddings テーブルの JSON blob を blob 形式に変換する。
// 変換件数を返す。
func reindexProjectEmbeddings(ctx context.Context, sqlDB *sql.DB, batchSize int, dryRun bool) (int, error) {
	const selectQuery = `
SELECT project_id, embedding_json
FROM project_embeddings
WHERE embedding_blob IS NULL
LIMIT ?`

	total := 0
	for {
		rows, err := sqlDB.QueryContext(ctx, selectQuery, batchSize)
		if err != nil {
			return total, fmt.Errorf("query project_embeddings: %w", err)
		}

		type row struct {
			projectID     string
			embeddingJSON string
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.projectID, &r.embeddingJSON); err != nil {
				rows.Close()
				return total, fmt.Errorf("scan project_embedding: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("rows error: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		if !dryRun {
			for _, r := range batch {
				vec, err := embeddingJSONToFloat32(r.embeddingJSON)
				if err != nil {
					continue
				}
				blob := retrieval.Float32SliceToBytes(vec)
				const updateQuery = `UPDATE project_embeddings SET embedding_blob = ? WHERE project_id = ?`
				if _, err := sqlDB.ExecContext(ctx, updateQuery, blob, r.projectID); err != nil {
					return total, fmt.Errorf("update project_embedding blob for %s: %w", r.projectID, err)
				}
				total++
			}
		} else {
			total += len(batch)
		}

		if len(batch) < batchSize {
			break
		}
	}
	return total, nil
}

// embeddingJSONToFloat32 は JSON 文字列から []float32 を復元する。
func embeddingJSONToFloat32(s string) ([]float32, error) {
	var vec []float32
	if err := json.Unmarshal([]byte(s), &vec); err != nil {
		return nil, fmt.Errorf("parse embedding json: %w", err)
	}
	return vec, nil
}
