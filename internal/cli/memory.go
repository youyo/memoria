package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"

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
	Query   string `arg:"" help:"検索クエリ"`
	Limit   int    `help:"最大件数" default:"10" short:"n"`
	Project string `help:"プロジェクト ID でフィルタ" optional:""`
	Kind    string `help:"kind でフィルタ (decision/constraint/todo/failure/fact/preference/pattern)" optional:""`
}

// Run は memory search を実行する。
func (c *MemorySearchCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	ctx := context.Background()

	ret := retrieval.New(database.SQL(), nil) // embedder nil = FTS only
	ftsResults, err := ret.FTSSearch(ctx, c.Query, c.Limit*3)
	if err != nil {
		return fmt.Errorf("fts search: %w", err)
	}

	// フィルタ適用
	var results []retrieval.Result
	for _, rr := range ftsResults {
		r := retrieval.Result{
			ChunkID:    rr.ID,
			Content:    rr.Content,
			Summary:    rr.Summary,
			Kind:       rr.Kind,
			Importance: rr.Importance,
			Scope:      rr.Scope,
			ProjectID:  rr.ProjectID,
			CreatedAt:  rr.CreatedAt,
			Score:      rr.Score,
		}
		if c.Kind != "" && r.Kind != c.Kind {
			continue
		}
		if c.Project != "" && r.ProjectID != c.Project {
			continue
		}
		results = append(results, r)
		if len(results) >= c.Limit {
			break
		}
	}
	if results == nil {
		results = []retrieval.Result{}
	}

	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	default:
		if len(results) == 0 {
			fmt.Fprintln(*w, "no results")
			return nil
		}
		for _, r := range results {
			text := r.Summary
			if text == "" {
				text = r.Content
			}
			if len(text) > 100 {
				text = text[:100] + "..."
			}
			fmt.Fprintf(*w, "[%s] score:%.3f | %s\n", r.Kind, r.Score, text)
		}
		return nil
	}
}

// ChunkDetail は memory get の出力構造体。
type ChunkDetail struct {
	ChunkID    string  `json:"chunk_id"`
	ProjectID  string  `json:"project_id"`
	Content    string  `json:"content"`
	Summary    string  `json:"summary,omitempty"`
	Kind       string  `json:"kind"`
	Importance float64 `json:"importance"`
	Scope      string  `json:"scope"`
	CreatedAt  string  `json:"created_at"`
}

// MemoryGetCmd は memory get コマンド。
type MemoryGetCmd struct {
	ID string `arg:"" help:"メモリ ID (chunk_id)"`
}

// Run は memory get を実行する。
func (c *MemoryGetCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	ctx := context.Background()

	const query = `
SELECT chunk_id, project_id, content, COALESCE(summary,''), kind, importance, scope, created_at
FROM chunks
WHERE chunk_id = ?`

	var chunk ChunkDetail
	err = database.SQL().QueryRowContext(ctx, query, c.ID).Scan(
		&chunk.ChunkID,
		&chunk.ProjectID,
		&chunk.Content,
		&chunk.Summary,
		&chunk.Kind,
		&chunk.Importance,
		&chunk.Scope,
		&chunk.CreatedAt,
	)
	if err == sql.ErrNoRows {
		switch globals.Format {
		case "json":
			fmt.Fprintln(*w, `{"error":"not found","chunk_id":"`+c.ID+`"}`)
		default:
			fmt.Fprintf(*w, "not found: %s\n", c.ID)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("query chunk: %w", err)
	}

	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(chunk)
	default:
		fmt.Fprintf(*w, "chunk_id:   %s\n", chunk.ChunkID)
		fmt.Fprintf(*w, "project_id: %s\n", chunk.ProjectID)
		fmt.Fprintf(*w, "kind:       %s\n", chunk.Kind)
		fmt.Fprintf(*w, "importance: %.2f\n", chunk.Importance)
		fmt.Fprintf(*w, "scope:      %s\n", chunk.Scope)
		fmt.Fprintf(*w, "created_at: %s\n", chunk.CreatedAt)
		if chunk.Summary != "" {
			fmt.Fprintf(*w, "summary:    %s\n", chunk.Summary)
		}
		fmt.Fprintf(*w, "content:\n%s\n", chunk.Content)
		return nil
	}
}

// MemoryListCmd は memory list コマンド。
type MemoryListCmd struct {
	Limit   int    `help:"最大件数" default:"20" short:"n"`
	Project string `help:"プロジェクト ID でフィルタ" optional:""`
	Kind    string `help:"kind でフィルタ (decision/constraint/todo/failure/fact/preference/pattern)" optional:""`
}

// Run は memory list を実行する。
func (c *MemoryListCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	ctx := context.Background()

	query := `SELECT chunk_id, project_id, content, COALESCE(summary,''), kind, importance, scope, created_at FROM chunks`
	var args []interface{}
	var conditions []string

	if c.Project != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, c.Project)
	}
	if c.Kind != "" {
		conditions = append(conditions, "kind = ?")
		args = append(args, c.Kind)
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, cond := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += cond
		}
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, c.Limit)

	rows, err := database.SQL().QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("list chunks: %w", err)
	}
	defer rows.Close()

	var chunks []ChunkDetail
	for rows.Next() {
		var chunk ChunkDetail
		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.ProjectID,
			&chunk.Content,
			&chunk.Summary,
			&chunk.Kind,
			&chunk.Importance,
			&chunk.Scope,
			&chunk.CreatedAt,
		); err != nil {
			return fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}
	if chunks == nil {
		chunks = []ChunkDetail{}
	}

	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(chunks)
	default:
		if len(chunks) == 0 {
			fmt.Fprintln(*w, "no chunks")
			return nil
		}
		for _, chunk := range chunks {
			text := chunk.Summary
			if text == "" {
				text = chunk.Content
			}
			if len(text) > 80 {
				text = text[:80] + "..."
			}
			fmt.Fprintf(*w, "%s [%s] %s\n", chunk.ChunkID[:8], chunk.Kind, text)
		}
		return nil
	}
}

// MemoryStatsOutput は memory stats の出力構造体。
type MemoryStatsOutput struct {
	ChunksTotal   int    `json:"chunks_total"`
	SessionsTotal int    `json:"sessions_total"`
	JobsPending   int    `json:"jobs_pending"`
	DBSizeBytes   int64  `json:"db_size_bytes"`
	DBPath        string `json:"db_path"`
}

// MemoryStatsCmd は memory stats コマンド。
type MemoryStatsCmd struct{}

// Run は memory stats を実行する。
func (c *MemoryStatsCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	ctx := context.Background()
	sqlDB := database.SQL()
	dbPath := database.Path()

	var out MemoryStatsOutput
	out.DBPath = dbPath

	// chunks 件数
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM chunks").Scan(&out.ChunksTotal); err != nil {
		out.ChunksTotal = 0
	}

	// sessions 件数
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&out.SessionsTotal); err != nil {
		out.SessionsTotal = 0
	}

	// jobs queued 件数
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE status = 'queued'").Scan(&out.JobsPending); err != nil {
		out.JobsPending = 0
	}

	// DB ファイルサイズ
	if fi, err := os.Stat(dbPath); err == nil {
		out.DBSizeBytes = fi.Size()
	}

	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	default:
		fmt.Fprintf(*w, "chunks_total:   %d\n", out.ChunksTotal)
		fmt.Fprintf(*w, "sessions_total: %d\n", out.SessionsTotal)
		fmt.Fprintf(*w, "jobs_pending:   %d\n", out.JobsPending)
		fmt.Fprintf(*w, "db_size_bytes:  %d\n", out.DBSizeBytes)
		fmt.Fprintf(*w, "db_path:        %s\n", out.DBPath)
		return nil
	}
}

// MemoryReindexCmd は memory reindex コマンド。
// 既存の JSON blob 形式の embedding を float32 バイナリ（BLOB）形式に変換する。
type MemoryReindexCmd struct {
	BatchSize int  `help:"バッチサイズ" default:"100"`
	DryRun    bool `help:"実際に書き込まず変換数のみ表示する" name:"dry-run"`
}

// Run は memory reindex を実行する。
func (c *MemoryReindexCmd) Run(globals *Globals, w *io.Writer, lazyDB *LazyDB) error {
	database, err := lazyDB.Get()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
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
