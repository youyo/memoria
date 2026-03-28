# M11: Ingest に embedding 統合 (`ingest-with-embedding`)

## 概要

M08（ingest worker loop）と M10（embedding integration）の成果を統合し、
chunk 保存後に embedding を呼び出して `chunk_embeddings` テーブルに保存する。
embedding worker が未起動でも ingest は正常動作する（embedding スキップ）。

## 状態（前提）

- `internal/embedding/client.go`: `Client.Embed(ctx, texts) → ([][]float32, error)` 実装済み
- `internal/worker/ensure_embedding.go`: `EnsureEmbedding(ctx, cfg)` 実装済み
- `internal/worker/checkpoint.go` / `session_end.go`: chunks テーブルへの書き込み実装済み
- `chunk_embeddings` テーブル: `chunk_id, model, embedding_json, created_at` 定義済み
- `config.EmbeddingConfig.Model = "cl-nagoya/ruri-v3-30m"` デフォルト値

## アーキテクチャ設計

### 新規ファイル

```
internal/ingest/embedder.go        — embedding 呼び出し + chunk_embeddings 保存
internal/ingest/embedder_test.go   — embedding モックを使ったテスト
```

### 変更ファイル

```
internal/worker/checkpoint.go      — Handle() 最後に embedder.EmbedChunks() 呼び出し追加
internal/worker/session_end.go     — Handle() 最後に embedder.EmbedChunks() 呼び出し追加
internal/worker/processor.go       — DefaultJobProcessor に Embedder を持たせる
CLAUDE.md                          — M11 完了を記録
```

## 詳細設計

### internal/ingest/embedder.go

```go
// Embedder インターフェース（テスト時にモック可能）
type Embedder interface {
    EmbedChunks(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error
}

// EmbedClient は embedding.Client の必要な部分のみを定義するインターフェース
type EmbedClient interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// ChunkEmbedder は EmbedClient を使って chunk_embeddings に保存する
type ChunkEmbedder struct {
    client EmbedClient
}

// NewChunkEmbedder は ChunkEmbedder を生成する
func NewChunkEmbedder(client EmbedClient) *ChunkEmbedder

// EmbedChunks は chunkIDs の content を一括で embed して chunk_embeddings に保存する。
// embedding worker が応答しない場合はエラーを返すが、
// 呼び出し側（HandleCheckpoint/HandleSessionEnd）はスキップ（warn ログのみ）する。
func (e *ChunkEmbedder) EmbedChunks(ctx context.Context, db *sql.DB, chunkIDs []string, modelName string) error
```

処理フロー（`EmbedChunks`）:
1. `chunkIDs` が空なら即返す
2. `chunk_embeddings` に既存の `chunk_id` を確認（部分的に既存の場合はスキップ）
3. 未 embed の `chunk_id` を絞り込み
4. `chunks` テーブルから `content` を取得
5. `client.Embed(ctx, contents)` を一括呼び出し（バッチ embedding）
6. 結果を `encoding/json` で JSON 文字列に変換
7. `chunk_embeddings` に `INSERT OR IGNORE`（冪等）

### internal/worker/checkpoint.go の変更

```go
// CheckpointHandler に Embedder を追加
type CheckpointHandler struct {
    db       *sql.DB
    embedder ingest.Embedder  // nil の場合は embedding スキップ
    model    string
}

// NewCheckpointHandlerWithEmbedder はシグネチャ変更なしの後方互換コンストラクタ
// NewCheckpointHandler(db) は従来通り embedder=nil で生成
```

`Handle()` の末尾：
```go
if h.embedder != nil && len(insertedChunkIDs) > 0 {
    if err := h.embedder.EmbedChunks(ctx, h.db, insertedChunkIDs, h.model); err != nil {
        h.logf("memoria: embedding skipped: %v\n", err)
        // embedding 失敗は非致命的: ingest は成功扱い
    }
}
```

### internal/worker/session_end.go の変更

同様に `SessionEndHandler` に `Embedder` を追加し、chunk INSERT 後に呼び出す。

### internal/worker/processor.go の変更

`DefaultJobProcessor` に `cfg *config.Config` を受け取り、
`embedding.New(socketPath)` で `ChunkEmbedder` を生成して Handler に渡す。

```go
type DefaultJobProcessor struct {
    checkpoint *CheckpointHandler
    sessionEnd *SessionEndHandler
}

// NewDefaultJobProcessorWithEmbedding は embedding 付きの DefaultJobProcessor を生成する
func NewDefaultJobProcessorWithEmbedding(db *sql.DB, cfg *config.Config) *DefaultJobProcessor

// NewDefaultJobProcessor は embedding なし（後方互換、テスト用）
func NewDefaultJobProcessor(db *sql.DB) *DefaultJobProcessor
```

## TDD ステップ

### Red フェーズ（テスト先行）

1. `internal/ingest/embedder_test.go` を作成
   - モック `EmbedClient` を定義
   - `EmbedChunks` が空 chunkIDs で即返すテスト
   - `EmbedChunks` が正常に `chunk_embeddings` に保存するテスト
   - `EmbedChunks` が既存 chunk_id をスキップするテスト
   - `EmbedChunks` が embedding エラーをそのまま返すテスト

2. `internal/worker/checkpoint_test.go` に embedding テスト追加
   - モック `Embedder` を使って EmbedChunks が呼ばれることを確認
   - `Embedder` が nil の場合は呼ばれないことを確認

3. `internal/worker/session_end_test.go` に embedding テスト追加
   - 同様

### Green フェーズ（最小実装）

1. `internal/ingest/embedder.go` を実装
2. `internal/worker/checkpoint.go` を変更
3. `internal/worker/session_end.go` を変更
4. `internal/worker/processor.go` を変更

### Refactor フェーズ

- `EmbedChunks` のバッチサイズ上限（例: 512件）を考慮
  - MVP では上限なし（全件一括）で OK。将来的な拡張ポイントとしてコメント記述

## embedding worker 未起動時のフォールバック

- `Handle()` 内で embedding エラーは **ログのみ / return nil**
- ingest 自体は成功扱い（chunk は保存済み）
- 後で `memoria worker start` + `memoria memory reindex`（M15）で再 embed 可能
- `logf` は daemon.go の `d.logf` を使う。Handler に `SetLogf()` を追加する

## テストカバレッジ目標

- `embedder.go`: 主要パスすべて（happy path + error path + skip path）
- `checkpoint.go` / `session_end.go`: embedding 呼び出し有無の両パターン

## ファイル一覧

| ファイル | 種別 |
|---------|------|
| `internal/ingest/embedder.go` | 新規 |
| `internal/ingest/embedder_test.go` | 新規 |
| `internal/worker/checkpoint.go` | 変更 |
| `internal/worker/checkpoint_test.go` | 変更 |
| `internal/worker/session_end.go` | 変更 |
| `internal/worker/session_end_test.go` | 変更 |
| `internal/worker/processor.go` | 変更 |
| `CLAUDE.md` | 変更 |
| `plans/memoria-roadmap.md` | 変更（M11 完了チェック） |

## 完了基準

- `go test ./... -timeout 120s` 全 green
- `make build` 成功
- `make lint` 成功（go vet エラーなし）
- `chunk_embeddings` テーブルへの保存が統合テストで確認済み
- embedding worker 未起動でも ingest が正常完了することをテストで確認済み
