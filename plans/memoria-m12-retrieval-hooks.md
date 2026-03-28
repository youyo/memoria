# M12: SessionStart + UserPrompt retrieval hooks

## 概要

SessionStart と UserPromptSubmit hook を実装し、SQLite に蓄積された chunks から関連メモリを取得して Claude Code に注入する。FTS5 + Vector（cosine similarity）+ RRF + project boost を統合した 3 層 retrieval を提供する。

## スコープ

- `memoria hook session-start` 実装
- `memoria hook user-prompt` 実装
- `internal/retrieval/` パッケージ
- `config print-hook` コマンド実装
- TDD (Red → Green → Refactor)

## アーキテクチャ

```
internal/retrieval/
├── retrieval.go    # Retriever 本体・公開 API
├── fts.go          # FTS5 検索
├── vector.go       # cosine similarity 計算・vector 検索
├── rrf.go          # Reciprocal Rank Fusion
├── boost.go        # project / similar project boost
└── retrieval_test.go
```

## retrieval パッケージ設計

### Retriever 構造体

```go
type Retriever struct {
    db        *sql.DB
    embedder  Embedder  // interface: Embed(ctx, texts) ([][]float32, error)
}

type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Result struct {
    ChunkID   string
    Content   string
    Summary   string
    Kind      string
    Importance float64
    Scope     string
    ProjectID string
    CreatedAt string
    Score     float64
}
```

### SessionStart retrieval

クエリなし。project boost + importance + recency で引く。

```
SELECT chunks.* FROM chunks
WHERE scope = 'global'
   OR project_id = ?        -- same project
   OR project_id IN (SELECT similar_project_id FROM project_similarity WHERE project_id = ? AND similarity > 0.5)
ORDER BY
    CASE WHEN project_id = ? THEN 3.0 ELSE 0.0 END  -- same project boost
  + importance
  + (1.0 / (julianday('now') - julianday(created_at) + 1))  -- recency
DESC
LIMIT 4
```

推奨配分:
- same project: 2 件
- similar project: 1 件
- global: 1 件

### UserPromptSubmit retrieval

1. prompt を embedding（embedding worker 経由）
2. FTS top-20 を取得
3. vector top-20 を取得（cosine similarity、JSON blob ベース）
4. RRF で統合
5. project boost を加算
6. top-5 を返す

#### FTS クエリ

```sql
SELECT chunks.chunk_id, chunks.content, chunks.summary, chunks.kind,
       chunks.importance, chunks.scope, chunks.project_id, chunks.created_at,
       bm25(chunks_fts) as fts_score
FROM chunks_fts
JOIN chunks USING (rowid)
WHERE chunks_fts MATCH ?
ORDER BY fts_score
LIMIT 20
```

FTS query 生成: スペース区切りで各トークンを AND 結合、特殊文字はエスケープ。

#### Vector 検索（JSON blob ベース）

```sql
SELECT c.chunk_id, c.content, c.summary, c.kind, c.importance, c.scope, c.project_id, c.created_at,
       ce.embedding_json
FROM chunks c
JOIN chunk_embeddings ce ON c.chunk_id = ce.chunk_id
LIMIT 100
```

Go 側で cosine similarity を計算し上位 N 件を返す。

#### RRF 統合

```
rrf_score(d) = Σ 1 / (k + rank(d))
k = 60 (定番値)
```

FTS ランクと vector ランクを統合後、project boost を加算。

#### Project boost

```
same project: +2.0
similar project (similarity > 0.5): +1.0
global: 0.0
```

## SessionStart 出力形式

```json
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "## 過去のメモリ\n\n**[decision]** (重要度: 0.8)\nGo の context.WithTimeout を 1.5 秒で設定...\n\n**[constraint]** (重要度: 0.7)\n...\n"
  }
}
```

## UserPromptSubmit 出力形式

```json
{
  "hookSpecificOutput": {
    "hookEventName": "UserPromptSubmit",
    "additionalContext": "## 関連メモリ\n\n**[decision]** (重要度: 0.9)\n..."
  }
}
```

## config print-hook 出力

Claude Code の `.claude/settings.json` に貼り付ける hooks 設定断片を出力する。

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "memoria hook session-start"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "memoria hook user-prompt"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "memoria hook stop"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "memoria hook session-end"
          }
        ]
      }
    ]
  }
}
```

## Fallback 方針

- retrieval 失敗（DB エラー、embedding タイムアウト等）→ 空の `additionalContext` を返す
- embedding worker 未起動 → FTS のみで実行（degraded mode）
- 結果 0 件 → 空の `additionalContext`

## TDD ステップ

### Step 1: retrieval パッケージ（Red）
- `retrieval_test.go` にテストを先行作成
  - `TestFTSSearch_Basic`: FTS 検索が結果を返す
  - `TestVectorSearch_CosineSimilarity`: cosine similarity 計算が正しい
  - `TestRRF_Merge`: RRF が正しくスコアを統合
  - `TestProjectBoost_SameProject`: same project が優先される
  - `TestSessionStartRetrieval_Empty`: chunks なし時は空スライス
  - `TestSessionStartRetrieval_WithChunks`: chunks あり時に結果を返す
  - `TestUserPromptRetrieval_FTSOnly`: embedding なし（degraded mode）でも動く
  - `TestUserPromptRetrieval_WithEmbedding`: embedding + FTS の統合

### Step 2: retrieval 実装（Green）
- `internal/retrieval/` を実装

### Step 3: CLI hook 実装（Green）
- `HookSessionStartCmd.Run()` 実装
- `HookUserPromptCmd.Run()` 実装
- `ConfigPrintHookCmd.Run()` 実装

### Step 4: CLI hook テスト（Red → Green）
- `hook_session_start_test.go`
- `hook_user_prompt_test.go`
- `config_print_hook_test.go`

### Step 5: Refactor
- `TestNotImplementedCommands` から session-start / user-prompt / config print-hook を除外

## タイムアウト

- SessionStart: 4 秒（DB 検索 + フォーマット）
- UserPromptSubmit: 4 秒（embedding + DB 検索 + フォーマット）

## 実装上の注意

- embedding worker が未起動の場合は FTS only で degraded 動作
- vector_json は `[]float32` を JSON array として保存済み（M11 実装済み）
- chunks_fts は INSERT トリガーで自動同期済み
- hook は絶対に block しない → error は stderr に記録し nil を返す
