// Package retrieval は memoria の 3 層 retrieval を実装する。
// same project > similar project > global の優先順位で chunks を検索し、
// FTS5 + Vector (cosine similarity) + RRF + project boost を統合する。
package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Embedder は embedding を取得するためのインターフェース。
// 本番では embedding.Client が実装し、テストではモックを使う。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Result は retrieval 結果の 1 件を表す。
type Result struct {
	ChunkID    string
	Content    string
	Summary    string
	Kind       string
	Importance float64
	Scope      string
	ProjectID  string
	CreatedAt  string
	Score      float64
}

// RankedResult は RRF・project boost 計算に使う中間表現。
type RankedResult struct {
	ID        string
	Score     float64
	ProjectID string
	// 完全なデータ（最終結果構築用）
	Content    string
	Summary    string
	Kind       string
	Importance float64
	Scope      string
	CreatedAt  string
}

// Retriever は memoria の retrieval エンジン。
type Retriever struct {
	db      *sql.DB
	embedder Embedder // nil の場合は FTS only (degraded mode)
}

// New は Retriever を生成する。
// embedder が nil の場合は FTS only で動作する。
func New(db *sql.DB, embedder Embedder) *Retriever {
	return &Retriever{db: db, embedder: embedder}
}

// SessionStart は project boost + importance + recency で chunks を取得する。
// similarProjects は project_id -> similarity スコアのマップ（nil 可）。
// maxResults は上限件数。
// isolated=true の場合は自プロジェクトのチャンクのみ返す（global も流入しない）。
func (r *Retriever) SessionStart(ctx context.Context, projectID string, similarProjects map[string]float64, maxResults int, isolated bool) ([]Result, error) {
	if maxResults <= 0 {
		maxResults = 4
	}

	var (
		rows *sql.Rows
		err  error
	)

	if isolated {
		// isolated プロジェクト: 自プロジェクトのチャンクのみ
		const query = `
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope, c.project_id, c.created_at,
       3.0
       + c.importance
       + (1.0 / (julianday('now') - julianday(c.created_at) + 1)) AS score
FROM chunks c
WHERE c.project_id = ?
ORDER BY score DESC
LIMIT ?`
		rows, err = r.db.QueryContext(ctx, query, projectID, maxResults*3)
	} else if len(similarProjects) == 0 {
		// 類似プロジェクトなし: same project + global のみ
		const query = `
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope, c.project_id, c.created_at,
       CASE WHEN c.project_id = ? THEN 3.0 ELSE 0.0 END
       + c.importance
       + (1.0 / (julianday('now') - julianday(c.created_at) + 1)) AS score
FROM chunks c
WHERE c.project_id = ?
   OR c.scope = 'global'
ORDER BY score DESC
LIMIT ?`
		rows, err = r.db.QueryContext(ctx, query, projectID, projectID, maxResults*3)
	} else {
		// 類似プロジェクトあり: same project + global + similarity_shareable（類似プロジェクトから）
		// 類似プロジェクト ID を展開して IN 句を構築
		similarIDs := make([]string, 0, len(similarProjects))
		for id := range similarProjects {
			similarIDs = append(similarIDs, id)
		}
		placeholders := strings.Repeat("?,", len(similarIDs))
		placeholders = placeholders[:len(placeholders)-1] // 末尾の "," を除去

		queryStr := fmt.Sprintf(`
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope, c.project_id, c.created_at,
       CASE WHEN c.project_id = ? THEN 3.0 ELSE 0.0 END
       + c.importance
       + (1.0 / (julianday('now') - julianday(c.created_at) + 1)) AS score
FROM chunks c
WHERE c.project_id = ?
   OR c.scope = 'global'
   OR (c.scope = 'similarity_shareable' AND c.project_id IN (%s))
ORDER BY score DESC
LIMIT ?`, placeholders)

		args := make([]any, 0, 2+len(similarIDs)+1)
		args = append(args, projectID, projectID)
		for _, id := range similarIDs {
			args = append(args, id)
		}
		args = append(args, maxResults*3)
		rows, err = r.db.QueryContext(ctx, queryStr, args...)
	}

	if err != nil {
		return nil, fmt.Errorf("session start query: %w", err)
	}
	defer rows.Close()

	var candidates []RankedResult
	for rows.Next() {
		var rr RankedResult
		var summary sql.NullString
		if err := rows.Scan(
			&rr.ID, &rr.Content, &summary, &rr.Kind, &rr.Importance, &rr.Scope, &rr.ProjectID, &rr.CreatedAt,
			&rr.Score,
		); err != nil {
			return nil, fmt.Errorf("scan session start row: %w", err)
		}
		rr.Summary = summary.String
		candidates = append(candidates, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// similar project boost を適用
	boosted := ApplyProjectBoost(candidates, projectID, similarProjects, isolated)

	// 上位 maxResults 件に絞る
	if len(boosted) > maxResults {
		boosted = boosted[:maxResults]
	}

	return toResults(boosted), nil
}

// UserPrompt は FTS + vector + project boost + RRF で chunks を検索する。
// embedder が nil の場合は FTS only で動作する（degraded mode）。
// isolated=true の場合は scope-aware boost で他プロジェクトチャンクが実質除外される。
func (r *Retriever) UserPrompt(ctx context.Context, projectID string, similarProjects map[string]float64, prompt string, maxResults int, isolated bool) ([]Result, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, nil
	}

	const candidateN = 20

	// FTS 検索
	ftsResults, err := r.FTSSearch(ctx, prompt, candidateN)
	if err != nil {
		// FTS 失敗時は空スライスで継続
		ftsResults = nil
	}

	var lists [][]RankedResult
	if len(ftsResults) > 0 {
		lists = append(lists, ftsResults)
	}

	// Vector 検索（embedder が利用可能な場合のみ）
	if r.embedder != nil {
		vecResults, err := r.vectorSearch(ctx, prompt, candidateN)
		if err == nil && len(vecResults) > 0 {
			lists = append(lists, vecResults)
		}
		// vector 検索失敗は無視（degraded mode 継続）
	}

	if len(lists) == 0 {
		return nil, nil
	}

	// RRF で統合
	merged := MergeRRF(lists, 60)

	// project boost を適用
	boosted := ApplyProjectBoost(merged, projectID, similarProjects, isolated)

	// isolated の場合は score が -999 のチャンク（他プロジェクト）を除外
	if isolated {
		filtered := boosted[:0]
		for _, rr := range boosted {
			if rr.Score > -100 {
				filtered = append(filtered, rr)
			}
		}
		boosted = filtered
	}

	// 上位 maxResults 件に絞る
	if len(boosted) > maxResults {
		boosted = boosted[:maxResults]
	}

	return toResults(boosted), nil
}

// FTSSearch は FTS5 で全文検索し、RankedResult のスライスを返す。
func (r *Retriever) FTSSearch(ctx context.Context, query string, limit int) ([]RankedResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}

	ftsQuery := buildFTSQuery(q)
	if ftsQuery == "" {
		return nil, nil
	}

	const sqlQuery = `
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope, c.project_id, c.created_at
FROM chunks_fts
JOIN chunks c ON chunks_fts.rowid = c.rowid
WHERE chunks_fts MATCH ?
ORDER BY c.created_at DESC
LIMIT ?`

	rows, err := r.db.QueryContext(ctx, sqlQuery, ftsQuery, limit)
	if err != nil {
		// FTS クエリが無効な場合（特殊文字等）は空を返す
		return nil, nil
	}
	defer rows.Close()

	var results []RankedResult
	for rows.Next() {
		var rr RankedResult
		var summary sql.NullString
		if err := rows.Scan(
			&rr.ID, &rr.Content, &summary, &rr.Kind, &rr.Importance, &rr.Scope, &rr.ProjectID, &rr.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan fts row: %w", err)
		}
		rr.Summary = summary.String
		results = append(results, rr)
	}
	// trigram トークナイザーは bm25 非対応のため、位置ベーススコアを付与
	for i := range results {
		results[i].Score = 1.0 / float64(i+1)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fts rows error: %w", err)
	}
	return results, nil
}

// vectorSearch は embedding を使って cosine similarity 検索を行う。
// embedding_blob が存在する場合はバイナリ高速パスを使用し、
// ない場合は embedding_json からの JSON パースにフォールバックする。
func (r *Retriever) vectorSearch(ctx context.Context, query string, limit int) ([]RankedResult, error) {
	// prompt を embed
	vecs, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	queryVec := vecs[0]

	// chunk_embeddings を取得して Go 側で cosine similarity を計算
	// embedding_blob が存在する場合はバイナリ高速パス、なければ JSON フォールバック
	const sqlQuery = `
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope, c.project_id, c.created_at,
       ce.embedding_blob, ce.embedding_json
FROM chunks c
JOIN chunk_embeddings ce ON c.chunk_id = ce.chunk_id
LIMIT 500`

	rows, err := r.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("vector search query: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		rr            RankedResult
		embeddingBlob []byte
		embeddingJSON string
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		var summary sql.NullString
		var blob []byte
		var embJSON string
		if err := rows.Scan(
			&c.rr.ID, &c.rr.Content, &summary, &c.rr.Kind, &c.rr.Importance, &c.rr.Scope, &c.rr.ProjectID, &c.rr.CreatedAt,
			&blob, &embJSON,
		); err != nil {
			return nil, fmt.Errorf("scan vector row: %w", err)
		}
		c.rr.Summary = summary.String
		c.embeddingBlob = blob
		c.embeddingJSON = embJSON
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector rows error: %w", err)
	}

	// cosine similarity を計算
	for i := range candidates {
		var vec []float32
		var parseErr error

		if len(candidates[i].embeddingBlob) > 0 {
			// 高速パス: バイナリデコード
			vec, parseErr = BytesToFloat32Slice(candidates[i].embeddingBlob)
		} else {
			// フォールバック: JSON パース（後方互換）
			vec, parseErr = parseFloat32Slice(candidates[i].embeddingJSON)
		}

		if parseErr != nil || len(vec) == 0 {
			continue
		}
		candidates[i].rr.Score = float64(CosineSimilarity(queryVec, vec))
	}

	// スコア降順ソート
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].rr.Score > candidates[j].rr.Score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]RankedResult, len(candidates))
	for i, c := range candidates {
		results[i] = c.rr
	}
	return results, nil
}

// FormatContext は []Result を additionalContext 用のテキストに整形する。
func FormatContext(results []Result) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 過去のメモリ\n\n")
	for _, r := range results {
		text := r.Summary
		if text == "" {
			text = r.Content
		}
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("**[%s]** (重要度: %.1f)\n%s\n\n", r.Kind, r.Importance, text))
	}
	return sb.String()
}

// buildFTSQuery は query 文字列を FTS5 MATCH 用のクエリに変換する。
// 特殊文字をエスケープし、スペース区切りでトークンを AND 結合する。
func buildFTSQuery(query string) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = escapeFTSToken(t)
		if t == "" {
			continue
		}
		// trigram トークナイザーは 3 文字未満のクエリを受け付けない
		inner := strings.Trim(t, `"`)
		if utf8.RuneCountInString(inner) < 3 {
			continue
		}
		escaped = append(escaped, t)
	}
	if len(escaped) == 0 {
		return ""
	}
	return strings.Join(escaped, " ")
}

// escapeFTSToken は FTS5 で特殊な意味を持つ文字を除去・エスケープする。
func escapeFTSToken(token string) string {
	// FTS5 の特殊文字: " ^ * : ( )
	// シンプルにダブルクォートで囲む
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '"', '^', '*', ':', '(', ')', '[', ']', '{', '}', '!', '?':
			return -1
		}
		return r
	}, token)
	if cleaned == "" {
		return ""
	}
	return `"` + cleaned + `"`
}

// toResults は []RankedResult を []Result に変換する。
func toResults(rrs []RankedResult) []Result {
	results := make([]Result, len(rrs))
	for i, rr := range rrs {
		results[i] = Result{
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
	}
	return results
}
