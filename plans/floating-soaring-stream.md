# memoria UX 改善プラン

## Context

memoria を実際に使ってみて発見された 3 つの UX 問題を修正する。

1. **`memory list` の ID で `memory get` できない** — list はUUIDの先頭8文字しか表示しないが、get は完全一致のみ
2. **exit 時に failure が誤記録される** — enricher のキーワード "error" が汎用的すぎて誤分類
3. **session start hook のコンテキスト品質** — 上記2の影響でノイズの多いメモリが注入される

## Bug 1: memory list/get ID 不一致

### 現状
- `internal/cli/memory.go:260` — `chunk.ChunkID[:8]` で8文字に切り詰め表示
- `internal/cli/memory.go:126` — `WHERE chunk_id = ?` で完全一致検索のみ
- chunk_id は UUID（36文字）

### 修正方針: get で前方一致検索をサポート

list の表示を36文字にすると可読性が下がるため、get 側で前方一致（prefix match）を実装する。

### 変更内容

**`internal/cli/memory.go`**

1. `MemoryGetCmd.Run()` の SQL を `WHERE chunk_id LIKE ? || '%'` に変更
2. 複数マッチ時はエラーメッセージで候補を表示（`ambiguous ID: N matches found`）
3. 0件時は既存の `not found` メッセージを維持

```go
// 変更前
const query = `SELECT ... FROM chunks WHERE chunk_id = ?`

// 変更後
const query = `SELECT ... FROM chunks WHERE chunk_id LIKE ? || '%'`
// + 複数マッチ時のハンドリング
```

**`internal/cli/memory_get_test.go`**
- 前方一致で取得できるテスト追加
- 複数マッチ時の ambiguous エラーテスト追加

## Bug 2: failure 誤分類

### 現状
- `internal/ingest/enricher.go:61` — failure キーワードに `"error"` が含まれる
- `"error"` は "エラー" と同じく汎用的だが、英語の "error" は会話中に頻出（例: "error handling", "no errors found"）
- exit 時の会話に "error" が含まれるだけで failure に分類される

### 修正方針: キーワードの精度向上

**`internal/ingest/enricher.go`**

1. `"error"` を削除し、より具体的なキーワードに置換:
   - `"error occurred"`, `"got error"`, `"error:"`, `"エラーが"`, `"エラー発生"`
2. `"failed"` も同様に精度向上を検討:
   - `"failed to"`, `"test failed"`, `"build failed"` など文脈付きに
3. 既存の `"エラー"` は日本語として十分具体的なので維持

```go
// 変更前
{"failure", []string{"failed", "失敗", "エラー", "error", "バグ", "bug", "crash", "不具合", "exception"}},

// 変更後
{"failure", []string{
    "failed to", "test failed", "build failed",
    "失敗", "エラーが", "エラー発生",
    "got error", "error occurred", "error:",
    "バグ", "bug", "crash", "不具合", "exception",
}},
```

**`internal/ingest/enricher_test.go`**
- "error handling" が failure にならないことを確認するテスト
- "error occurred" が failure になることを確認するテスト
- 通常のセッション終了会話が fact になることを確認

## Bug 3: session start hook コンテキスト品質

### 現状
- hook 自体の実装は正常（graceful degradation あり）
- 問題は Bug 2 の影響で **ノイズの多い chunk が高 importance で保存** されていること
- system metadata（タスク通知等）が decision として保存される問題も確認済み（observation 22547）

### 修正方針: enricher にコンテンツフィルタを追加

**`internal/ingest/enricher.go`**

1. `Enrich()` の冒頭で system metadata パターンを検出してスキップ or importance を下げる:
   - `<task-notification>`, `<task-id>` を含むコンテンツ → importance 0.1 に制限
   - `<system-reminder>` を含むコンテンツ → importance 0.1 に制限
2. これにより session start hook で返されるコンテキストの品質が向上

```go
// Enrich() 冒頭に追加
if isSystemMetadata(content) {
    return EnrichResult{
        Kind:       "fact",
        Importance: 0.1,
        Scope:      "project",
        Keywords:   nil,
        Summary:    "",
    }
}

func isSystemMetadata(content string) bool {
    lower := strings.ToLower(content)
    return strings.Contains(lower, "<task-notification>") ||
        strings.Contains(lower, "<task-id>") ||
        strings.Contains(lower, "<system-reminder>")
}
```

**`internal/ingest/enricher_test.go`**
- system metadata が低 importance で保存されるテスト

## 対象ファイル一覧

| ファイル | 変更内容 |
|---------|---------|
| `internal/cli/memory.go` | get で前方一致検索 |
| `internal/cli/memory_get_test.go` | 前方一致・ambiguous テスト |
| `internal/ingest/enricher.go` | キーワード精度向上 + system metadata フィルタ |
| `internal/ingest/enricher_test.go` | 誤分類防止テスト |

## 検証手順

1. `make test` — 全テスト green
2. `make build` — ビルド成功
3. `./bin/memoria memory list --limit 5` → 8文字IDを確認
4. `./bin/memoria memory get <8文字ID>` → 前方一致で取得成功
5. enricher テストで "error handling" が fact、"error occurred" が failure であることを確認
6. enricher テストで `<task-notification>` コンテンツが importance 0.1 であることを確認
