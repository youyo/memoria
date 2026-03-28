# M13: プロジェクト識別 + Similarity (`project-fingerprint`)

## 概要

プロジェクトの特性（言語・フレームワーク・ツールチェーン等）を自動抽出してフィンガープリントを生成し、プロジェクト間の類似度をコサイン類似度で計算・キャッシュする。retrieval の same/similar project boost の精度向上が目的。

## 依存関係

- M12 完了（`internal/retrieval/` に `ApplyProjectBoost` 実装済み）
- `projects` テーブル: `fingerprint_json`, `fingerprint_text`, `fingerprint_updated_at`, `similarity_updated_at` カラム存在
- `project_similarity` テーブル: `project_id`, `similar_project_id`, `similarity`, `computed_at` カラム存在
- `project_embeddings` テーブル: `project_id`, `model`, `embedding_json`, `created_at` カラム存在
- `internal/embedding/client.go`: `Embed(ctx, texts)` 実装済み
- `internal/queue/job.go`: `JobTypeProjectRefresh`, `JobTypeProjectSimilarityRefresh` 定義済み

## 実装ファイル

```
internal/fingerprint/
├── fingerprint.go        # フィンガープリント生成ロジック
└── fingerprint_test.go   # TDD テスト

internal/project/
├── project.go            # 既存（Resolver）
├── project_test.go       # 既存
├── similarity.go         # Similarity 計算 + キャッシュ + TTL 確認
└── similarity_test.go    # TDD テスト

internal/ingest/
└── worker.go             # 既存 - project_refresh / project_similarity_refresh ハンドラ追加

internal/project/
└── refresh.go            # RefreshFingerprint + EnsureFreshFingerprint
```

## アーキテクチャ

```
SessionStart / UserPrompt hook
  └── project.Resolver.Resolve()
        └── EnsureFreshFingerprint() ← TTL チェック（24h）
              ├── TTL 切れ → queue.Enqueue(project_refresh)
              └── OK → GetSimilarProjects() ← TTL チェック（7d）
                        ├── TTL 切れ → queue.Enqueue(project_similarity_refresh)
                        └── OK → map[string]float64 返す

ingest worker
  ├── project_refresh → fingerprint.Generate() → DB 保存 → embedding → project_embeddings 保存
  └── project_similarity_refresh → 全 project_embeddings 取得 → コサイン類似度 → project_similarity 保存
```

## fingerprint 材料

| フィールド | 検出方法 |
|---|---|
| `primary_language` | ファイル拡張子カウント（.go, .py, .ts, .js, .rs, .java, .rb, .cs, .cpp, .c 等） |
| `framework` | 設定ファイル存在確認（go.mod, pyproject.toml, package.json, Cargo.toml 等） |
| `project_kind` | git ls-files / ファイル構造から推定（cli, web, library, infra, etc） |
| `build_tool` | Makefile, Dockerfile, .github/, 等の存在 |
| `key_files` | go.mod, pyproject.toml, package.json, Cargo.toml, .goreleaser.yaml, skills/, .claude/ |
| `repo_name` | filepath.Base(rootPath) |

## fingerprint_text の形式

```
Go CLI project using Kong framework. Build tools: make, docker. Key files: go.mod, Makefile, .goreleaser.yaml. Repository: memoria.
```

## 実装ステップ（TDD）

### Step 1: `internal/fingerprint` パッケージ

**Red**: `fingerprint_test.go` にテストを書く

```go
// ファイル拡張子からプライマリ言語を検出
func TestDetectPrimaryLanguage(t *testing.T)
// フレームワーク・ビルドツールを検出
func TestDetectFrameworks(t *testing.T)
// fingerprint_text を生成
func TestGenerateFingerprintText(t *testing.T)
// JSON 表現を生成
func TestGenerateFingerprintJSON(t *testing.T)
// Generate 統合テスト（テンポラリディレクトリ）
func TestGenerate(t *testing.T)
```

**Green**: `fingerprint.go` に実装

**Refactor**: 検出精度と可読性を向上

### Step 2: `internal/project/similarity.go`

**Red**: `similarity_test.go` にテストを書く

```go
// project_similarity テーブルから SimilarProjects を取得
func TestGetSimilarProjects(t *testing.T)
// TTL チェック（7d）
func TestIsSimilarityFresh(t *testing.T)
// project_similarity を保存
func TestUpsertSimilarity(t *testing.T)
// project_embeddings を保存
func TestUpsertProjectEmbedding(t *testing.T)
// EnsureFreshFingerprint（TTL 24h チェック）
func TestEnsureFreshFingerprint(t *testing.T)
```

**Green**: `similarity.go` + `refresh.go` に実装

### Step 3: ingest worker へのジョブハンドラ追加

`internal/ingest/worker.go` の handleJob に case 追加:

```go
case queue.JobTypeProjectRefresh:
    return w.handleProjectRefresh(ctx, job)
case queue.JobTypeProjectSimilarityRefresh:
    return w.handleProjectSimilarityRefresh(ctx, job)
```

### Step 4: hook 統合

`SessionStart` / `UserPrompt` で `EnsureFreshFingerprint` と `GetSimilarProjects` を呼ぶ。

## TTL 定数

```go
const (
    FingerprintTTL = 24 * time.Hour
    SimilarityTTL  = 7 * 24 * time.Hour
)
```

## DB 操作詳細

### EnsureFreshFingerprint

```sql
SELECT fingerprint_updated_at FROM projects WHERE project_id = ?
```

- NULL または 24h 超過 → `project_refresh` をキュー投入（非同期、hook は block しない）

### GetSimilarProjects

```sql
SELECT similar_project_id, similarity
FROM project_similarity
WHERE project_id = ?
  AND julianday('now') - julianday(computed_at) < 7
```

- 存在しない or 0件 → `project_similarity_refresh` をキュー投入

### project_refresh ジョブハンドラ

payload: `{"project_id": "xxx", "project_root": "/path/to/project"}`

1. `fingerprint.Generate(ctx, projectRoot)` を呼ぶ
2. `projects` テーブルを UPDATE（`fingerprint_json`, `fingerprint_text`, `fingerprint_updated_at`）
3. `embedding.Client.Embed(ctx, []string{fingerprintText})` を呼ぶ
4. `project_embeddings` を UPSERT

### project_similarity_refresh ジョブハンドラ

payload: `{"project_id": "xxx"}`

1. 全 `project_embeddings` を取得
2. 対象プロジェクトの embedding を取得
3. 各プロジェクトとのコサイン類似度を計算
4. `project_similarity` に UPSERT（computed_at 更新）

## テスト方針

- `fingerprint.Generate` はテンポラリディレクトリを使用してファイル存在を検証
- DB 操作テストは `internal/testutil` の `NewTestDB()` を使用
- ingest worker テストはモック embedder を使用

## 注意事項

- hook は block しない設計: TTL 切れ時はキュー投入のみ（同期更新なし）
- `EnsureFreshFingerprint` はエラーを返しても hook は継続（best effort）
- fingerprint.Generate は git ls-files がない環境でも動作（os.ReadDir フォールバック）
- コサイン類似度は `internal/retrieval/vector.go` の `CosineSimilarity` を再利用

## 実装後の検証

```bash
make build
make test
make lint
```
