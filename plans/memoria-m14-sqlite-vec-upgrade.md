# M14: ベクトル検索最適化 + memory reindex (`sqlite-vec-upgrade`)

## Meta

| 項目 | 値 |
|------|---|
| マイルストーン | M14 |
| スラッグ | sqlite-vec-upgrade |
| 依存 | M13 |
| ステータス | 実装中 |
| 作成日 | 2026-03-28 |

## 背景と判断

### sqlite-vec の採用可否

sqlite-vec は C 拡張ライブラリである。memoria は `modernc.org/sqlite`（Pure Go）を使用しているため、
C 拡張のロードは不可能。`mattn/go-sqlite3`（CGO）への移行は配布性の低下を招くため、M14 では行わない。

**決定**: Go 側でのベクトル検索を最適化する。sqlite-vec は将来 mattn/go-sqlite3 移行時に導入。

### 最適化方針

現状の問題:
- `vectorSearch()` が `chunk_embeddings` を全件取得（LIMIT 200）
- 各行の `embedding_json`（TEXT）を Go で JSON パースしてコサイン類似度を計算
- JSON パースのオーバーヘッドが大きい

改善案:
1. **float32 バイナリ形式でのストレージ追加** - JSON に加えて `embedding_blob` (BLOB) カラムを追加
   - BLOB は JSON より読み取りが速く（デシリアライズ不要）、ストレージも小さい
   - 後方互換性のため `embedding_json` は残す
2. **バイナリ高速パス** - `embedding_blob` が存在する場合は `encoding/binary` でデコード
3. **SIMD ヒント最適化** - float64 から float32 への変換を排除しネイティブ計算
4. **`memory reindex` 実装** - 既存 JSON blob を blob 形式に変換するコマンド

## スコープ

### In-scope

- `chunk_embeddings` テーブルに `embedding_blob` カラム追加（マイグレーション）
- `project_embeddings` テーブルに `embedding_blob` カラム追加（マイグレーション）
- `vectorSearch()` のバイナリ高速パス実装
- embedding 保存時に blob も書き込む（ingest worker 側）
- `memory reindex` コマンド実装（既存 JSON → blob 変換）
- ベンチマークテスト追加

### Out-of-scope

- sqlite-vec 導入（mattn/go-sqlite3 移行は M15+ 以降）
- KNN による SQL 側フィルタ（Pure Go のため不可）
- LIMIT 200 の廃止（blob 化でパフォーマンス改善後に検討）

## 実装詳細

### 1. マイグレーション（0002_embedding_blob.sql）

```sql
ALTER TABLE chunk_embeddings ADD COLUMN embedding_blob BLOB;
ALTER TABLE project_embeddings ADD COLUMN embedding_blob BLOB;
```

### 2. float32 ↔ binary 変換ユーティリティ（internal/retrieval/vector.go）

```go
// Float32SliceToBytes は []float32 を little-endian バイト列に変換する。
func Float32SliceToBytes(vec []float32) []byte

// BytesToFloat32Slice は little-endian バイト列を []float32 に変換する。
func BytesToFloat32Slice(b []byte) ([]float32, error)
```

### 3. vectorSearch 高速パス（internal/retrieval/retrieval.go）

```sql
-- blob が存在する場合は blob で、なければ JSON で取得
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope,
       c.project_id, c.created_at,
       ce.embedding_blob, ce.embedding_json
FROM chunks c
JOIN chunk_embeddings ce ON c.chunk_id = ce.chunk_id
LIMIT 500
```

- `embedding_blob` が非 NULL → `BytesToFloat32Slice` で高速デコード
- `embedding_blob` が NULL → `parseFloat32Slice`（JSON パース、後方互換）
- LIMIT を 200 → 500 に拡張（blob 化でパフォーマンス向上分を活用）

### 4. embedding 保存時の blob 書き込み（internal/embedding/store.go または ingest 側）

新規保存時は JSON + blob の両方を書き込む。

### 5. memory reindex コマンド（internal/cli/memory.go）

```
memoria memory reindex [--db <path>] [--batch-size <n>]
```

処理フロー:
1. DB を開く
2. `embedding_blob IS NULL` な `chunk_embeddings` 行を取得（バッチ処理）
3. `embedding_json` を JSON パースして float32 スライスに変換
4. `Float32SliceToBytes` で blob に変換
5. UPDATE で `embedding_blob` を書き込む
6. 進捗を stderr に表示
7. 完了サマリーを出力

同様に `project_embeddings` も処理。

## TDD 計画

### Red フェーズ（失敗テスト先書き）

1. `TestFloat32SliceToBytes_RoundTrip` - バイト変換のラウンドトリップ
2. `TestBytesToFloat32Slice_InvalidLength` - 無効なバイト列のエラー処理
3. `TestVectorSearch_BlobPath` - blob が存在する場合に blob パスを使う
4. `TestVectorSearch_JSONFallback` - blob が NULL の場合は JSON にフォールバック
5. `TestMemoryReindexCmd_Run` - reindex が JSON → blob 変換を実行する
6. `BenchmarkCosineSimilarity_JSON` vs `BenchmarkCosineSimilarity_Blob` - パフォーマンス比較

### Green フェーズ

最小限の実装でテストを通す。

### Refactor フェーズ

- `vectorSearch` の重複コード整理
- エラーメッセージの統一

## ファイル変更一覧

| ファイル | 変更種別 | 内容 |
|---------|---------|------|
| `internal/db/migrations/0002_embedding_blob.sql` | 新規 | blob カラム追加マイグレーション |
| `internal/retrieval/vector.go` | 更新 | Float32SliceToBytes / BytesToFloat32Slice 追加 |
| `internal/retrieval/retrieval.go` | 更新 | vectorSearch blob 高速パス |
| `internal/retrieval/retrieval_test.go` | 更新 | blob パステスト + ベンチマーク |
| `internal/ingest/worker.go` または embedding 関連 | 更新 | embedding 保存時に blob も書き込む |
| `internal/cli/memory.go` | 更新 | MemoryReindexCmd 実装 |
| `internal/cli/memory_reindex_test.go` | 新規 | reindex コマンドテスト |
| `plans/memoria-roadmap.md` | 更新 | M14 完了マーク |
| `CLAUDE.md` | 更新 | M14 完了状態を反映 |

## 受け入れ基準

- `go test ./... -timeout 120s` が全 green
- `make build` + `make lint` が成功
- `memoria memory reindex` が既存 JSON blob を blob 形式に変換できる
- `vectorSearch` が blob 存在時に JSON パースをスキップする
- ベンチマークで blob パスが JSON パスより高速であることを確認
