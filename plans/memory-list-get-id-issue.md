# 調査報告: memoria の `memory list` / `memory get` ID 不一致問題

**調査日**: 2026-03-31  
**対象**: memoria プロジェクト  
**調査徹底度**: very thorough  
**状況**: 問題の根本原因を特定

---

## 要約

### 問題
`memory list` コマンドで表示される chunk_id を、そのまま `memory get` で使用できない。

### 原因
`memory list` のテキスト出力で chunk_id を **意図的に最初の8文字のみに切り詰めて表示**しているため。

### エビデンス
**ファイル**: `internal/cli/memory.go` の 260 行目

```go
fmt.Fprintf(*w, "%s [%s] %s\n", chunk.ChunkID[:8], chunk.Kind, text)
//                                ^^^^^^^^^^^^^^^^^^^^
//                        8文字のスライス操作で切り詰め
```

---

## 詳細調査結果

### 1. chunk_id の実装フロー

```
checkpoint.go (116行)
    ↓
chunkID := uuid.New().String()
    ↓
例: "12345678-abcd-1234-5678-9abcdefghijk" （36文字の UUID）
    ↓
chunks テーブルに INSERT （完全な36文字を保存）
    ↓
DB: chunk_id = "12345678-abcd-1234-5678-9abcdefghijk"
```

### 2. memory list の出力フロー

#### テキスト形式（問題あり）
```go
// memory.go 260行
fmt.Fprintf(*w, "%s [%s] %s\n", chunk.ChunkID[:8], chunk.Kind, text)
```

出力例:
```
12345678 [decision] これは決定です
87654321 [fact] プロジェクト情報
```

**問題**: "12345678" は不完全な ID で、DB に問い合わせても一致しない

#### JSON形式（問題なし）
```go
// memory.go 246行
return enc.Encode(chunks)
```

出力例:
```json
[
  {
    "chunk_id": "12345678-abcd-1234-5678-9abcdefghijk",
    ...
  }
]
```

**理由**: ChunkDetail 構造体をそのまま JSON エンコードするため、切り詰めない

### 3. memory get の実装

```go
// memory.go 123-126行
const query = `
SELECT chunk_id, project_id, content, ...
FROM chunks
WHERE chunk_id = ?`

err = database.SQL().QueryRowContext(ctx, query, c.ID).Scan(...)
```

**重要**: WHERE 句で **完全な chunk_id** と照合している

- ユーザーが "12345678" を渡す → DB で一致するレコードなし
- ユーザーが "12345678-abcd-..." を渡す → 正常に取得

### 4. DB スキーマ

```sql
CREATE TABLE IF NOT EXISTS chunks (
    chunk_id                TEXT PRIMARY KEY,
    ...
);
```

chunk_id は TEXT型で、UUID の36文字すべてをキーとして保存。

### 5. テストの状況

#### memory_list_test.go
- テストは存在するが、**モック DB で検証**
- テキスト形式の出力を詳しく検証していない
- JSON形式は完全な ID を返すため問題を検出できない

#### memory_get_test.go
- memory list との連携テストがない
- 単独で "not found" ケースのみテスト

**結果**: 統合テストがないため、この不一致の問題が検出されていない

---

## 再現シナリオ

```bash
# ステップ 1: memory list を実行
$ memoria memory list
12345678 [decision] 重要な決定
87654321 [constraint] プロジェクト構成

# ステップ 2: 表示された ID をコピーして memory get に渡す
$ memoria memory get 12345678

# ステップ 3: 失敗
not found: 12345678

# ステップ 4: JSON形式なら成功する
$ memoria memory list --format json
[{"chunk_id":"12345678-abcd-...", ...}]

$ memoria memory get 12345678-abcd-...
chunk_id:   12345678-abcd-...
...
```

---

## 設計の意図（推測）

chunk_id を8文字に切り詰める理由:
1. **可視性**: 36文字の UUID は視認性が悪い
2. **ターミナル幅**: 限られたスペースで複数列を表示する必要性
3. **ユーザー体験**: short ID の方が見やすい

しかし、**短さと機能性のトレードオフが解決されていない**

---

## 関連コードマップ

| ファイル | 行 | 説明 |
|---------|----|----|
| `internal/cli/memory.go` | 260 | **問題の箇所**: memory list テキスト出力で `ChunkID[:8]` |
| `internal/cli/memory.go` | 246 | memory list JSON 出力（完全な ID を出力） |
| `internal/cli/memory.go` | 110-170 | memory get 実装（WHERE chunk_id = ?） |
| `internal/cli/memory.go` | 172-264 | memory list 実装全体 |
| `internal/worker/checkpoint.go` | 116 | chunk_id の生成: `uuid.New().String()` |
| `internal/db/migrations/0001_initial.sql` | 66 | chunks テーブルのスキーマ定義 |
| `internal/cli/memory_list_test.go` | - | memory list テスト（連携テストなし） |
| `internal/cli/memory_get_test.go` | - | memory get テスト（連携テストなし） |

---

## 改善案

### 案1: テキスト出力で完全な ID を表示（推奨）

**メリット**:
- ユーザーがそのまま ID をコピペできる
- memory list → memory get の流れが直感的
- 実装が最小限（1行の変更）

**デメリット**:
- ターミナル出力が長くなる
- 視認性がやや落ちる

**実装**:
```go
// 260行を以下に変更:
fmt.Fprintf(*w, "%s [%s] %s\n", chunk.ChunkID, chunk.Kind, text)
```

### 案2: 短い ID 表示 + フルオプション

```bash
memoria memory list              # 短い形式（8文字）
memoria memory list --full-id    # フル形式（UUID）
```

**メリット**:
- デフォルトはコンパクト
- 必要に応じてフル表示
- API が柔軟

**デメリット**:
- 実装がやや複雑
- ユーザーが2つのモード を意識する必要

### 案3: JSON形式の推奨に方針転換

- テキスト形式を廃止
- JSON形式（完全な ID）を標準に
- ユーザーに jq などの外部ツール活用を推奨

**メリット**:
- スクリーン幅の制約なし
- 完全な情報を提供

**デメリット**:
- ユーザーが JSON を扱う必要
- 既存の利用パターンを変更

### 案4: content_hash の短い形式を ID として使用

schema には既に `content_hash` (SHA-256) が存在:
- content_hash の最初の8文字を表示 ID として採用
- memory get でも8文字でマッチング可能に

**メリット**:
- 短い ID でもユーザーが使用可能
- 既存データを活用

**デメリット**:
- chunk_id と content_hash ID の2つの識別子が混在
- 複雑さが増す
- 歴史的な chunk_id との互換性問題

---

## 推奨される修正（最小限）

**案1** を推奨:

```go
// internal/cli/memory.go 260行を修正
fmt.Fprintf(*w, "%s [%s] %s\n", chunk.ChunkID, chunk.Kind, text)  // [:8] を削除
```

この1行の修正で:
- memory list で表示される ID が memory get で使用可能に
- ユーザーが混乱しなくなる
- テキスト形式と JSON形式の一貫性が保証される

---

## テストの改善

`memory_list_test.go` と `memory_get_test.go` に統合テストを追加:

```go
func TestMemoryListGetIntegration(t *testing.T) {
    // 1. メモリをインサート
    // 2. memory list で ID を取得
    // 3. その ID で memory get を実行
    // 4. 同じコンテンツが返されることを確認
}
```

このテストで、この問題の再発を防ぐ。

---

## まとめ

| 項目 | 内容 |
|-----|------|
| **問題** | memory list の ID が memory get で使用できない |
| **根本原因** | memory.go 260行でID を8文字に切り詰めている |
| **影響範囲** | ユーザーの対話的なコマンド使用フロー |
| **検出不足** | テストが連携シナリオをカバーしていない |
| **推奨修正** | テキスト出力で完全な chunk_id を表示 |
| **優先度** | 中程度（ユーザー体験に直結） |

