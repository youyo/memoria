# Plan: Embedding バッチ分割 + Enricher Summary 品質改善

## Context

2つの問題を修正する:

1. **Embedding 422 エラー**: Python embedding worker は1リクエスト最大64テキストのバリデーションがあるが、Go 側の `EmbedChunks` は件数制限なしに一括送信するため、64件超で 422 Unprocessable Content が発生する
2. **Summary 品質**: `makeSummary()` が content の先頭100文字をそのまま切り出すため、`User: ` プレフィックスや `<task-notification>` 等の XML タグが summary に混入し、検索・表示品質が低い

---

## Task 1: Embedding バッチ分割

### 変更ファイル
- `internal/ingest/embedder.go`
- `internal/ingest/embedder_test.go`

### 実装内容

1. 定数 `EmbedBatchSize = 64` を追加
2. `EmbedChunks()` の L77-78（単一 `Embed()` 呼び出し）をバッチループに置換:
   ```go
   var allEmbeddings [][]float32
   for i := 0; i < len(contents); i += EmbedBatchSize {
       end := i + EmbedBatchSize
       if end > len(contents) {
           end = len(contents)
       }
       batch, err := e.client.Embed(ctx, contents[i:end])
       if err != nil {
           return fmt.Errorf("embed chunks (batch %d-%d): %w", i, end, err)
       }
       allEmbeddings = append(allEmbeddings, batch...)
   }
   ```
3. `embeddings` 変数を `allEmbeddings` に置換し、後続の保存ロジックはそのまま

### テスト追加

`TestEmbedChunks_BatchSplit` — 70件（>64）の chunk を挿入し、mock の `Embed()` が2回呼ばれること、各呼び出しの texts が64件以下であること、全70件が正しく保存されることを検証

---

## Task 2: Summary 品質改善

### 変更ファイル
- `internal/ingest/enricher.go`

### 実装内容

`makeSummary()` を以下のように改善:

1. **XML タグ除去**: `<task-notification>`, `<command-message>`, `<command-name>`, `<tool-use-id>` 等の XML タグとその内容、または閉じタグまでの内容を除去
   - 正規表現: `<[^>]+>` でタグ自体を除去（内容は保持）
2. **プレフィックス除去**: 先頭の `User: `, `Assistant: `, `A: ` を strip
3. **ノイズ行除去**: `[Tool: ...]` 形式のツール呼び出し行を除去
4. **空白正規化**: 連続改行・空白を単一スペースに正規化
5. **トリム後に先頭100文字で切り出し**（100文字のまま維持 — summary は検索インデックス用であり、表示用ではないため十分）

```go
func makeSummary(content string) string {
    s := content
    // XML タグ除去
    s = reXMLTag.ReplaceAllString(s, "")
    // プレフィックス除去
    s = strings.TrimPrefix(s, "User: ")
    s = strings.TrimPrefix(s, "Assistant: ")
    s = strings.TrimPrefix(s, "A: ")
    // Tool 行除去
    s = reToolLine.ReplaceAllString(s, "")
    // 空白正規化
    s = reWhitespace.ReplaceAllString(strings.TrimSpace(s), " ")
    // 切り出し
    runes := []rune(s)
    if len(runes) <= 100 {
        return s
    }
    return string(runes[:100]) + "..."
}
```

コンパイル済み正規表現を `var` ブロックに定義:
```go
var (
    reXMLTag    = regexp.MustCompile(`<[^>]+>`)
    reToolLine  = regexp.MustCompile(`(?m)^\[Tool: [^\]]*\]\s*$`)
    reWhitespace = regexp.MustCompile(`\s+`)
)
```

### テスト更新

`internal/ingest/enricher_test.go`:
- `TestEnrichSummaryLong` の `isValidSummary` はそのまま動作する（100文字+省略記号のチェック）
- 新規テストケース追加:
  - `TestMakeSummaryStripsXMLTags` — `<task-notification>` 等が除去されること
  - `TestMakeSummaryStripsUserPrefix` — `User: ` が除去されること
  - `TestMakeSummaryStripsToolLines` — `[Tool: ...]` 行が除去されること
  - `TestMakeSummaryNormalizesWhitespace` — 連続空白が正規化されること

`makeSummary` を公開関数 `MakeSummary` にする必要があるかもしれないが、テストは `Enrich()` 経由で検証可能なので非公開のままでよい。テストは `Enrich("User: <task-notification>...</task-notification>\n\n実際の内容")` のように入力し `result.Summary` を検証する。

---

## 検証方法

```bash
# テスト実行
go test ./internal/ingest/ -v -run "TestEmbedChunks_BatchSplit|TestEnrichSummary|TestMakeSummary"

# 全テスト
make test

# 実動作確認: 手動で大量 chunk を持つセッションを再 ingest
# embedding.log に 422 が出ないことを確認
memoria hook session-end <<< '{"session_id":"...", "cwd":"...", "transcript_path":"..."}'
tail -f ~/.local/state/memoria/logs/embedding.log
```
