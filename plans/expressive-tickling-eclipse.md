# fix: HookUserPromptCmd の `sql: database is closed` エラー修正

## Context

`memoria hook user-prompt` で **2,318件の `sql: database is closed` エラー**が発生。
hook は発火しているが、enqueue が全て失敗しており user-prompt の ingest が一切記録されていない。

### 根本原因

`internal/cli/hook.go:195-214` で enqueue を **fire-and-forget goroutine** で実行している。
`writeHookOutput()` が先に return → main() が終了 → プロセス exit 時に DB がGCされ、
goroutine 内の `queue.Enqueue()` が `sql: database is closed` で失敗する。

```
Run() → RunWithReader() → go func(){ enqueue }() → writeHookOutput() → return → main() 終了
                                ↑ goroutine がまだ実行されていない → DB closed
```

## 修正方針

**enqueue を同期実行に変更する**。理由:
- enqueue は SQLite INSERT 1件（< 1ms）で、hook の 4秒タイムアウト内で十分
- 非同期にする実益がない（retrieval が 4秒のボトルネック、enqueue は誤差）
- `HookStopCmd` / `HookSessionEndCmd` は既に同期 enqueue で正常動作している

## 変更ファイル

### `internal/cli/hook.go`

**変更1**: `HookUserPromptCmd.RunWithReader()` (L195-214)
- goroutine を除去し、enqueue を `writeHookOutput()` の前に同期実行
- enqueue 失敗は stderr に出すだけで hook 応答には影響させない（現行動作と同じ）

```go
// Before (L194-216):
// UserPrompt の非同期 ingest（hook 応答を block しない）
go func() {
    enqCtx, enqCancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer enqCancel()
    // ... enqueue ...
}()
return writeHookOutput(w, "UserPromptSubmit", additionalContext)

// After:
// UserPrompt の ingest enqueue（SQLite INSERT なので高速）
enqCtx, enqCancel := context.WithTimeout(context.Background(), 2*time.Second)
defer enqCancel()
// ... enqueue（同期）...
return writeHookOutput(w, "UserPromptSubmit", additionalContext)
```

## 検証

1. `go test ./internal/cli/... -run Hook` — 既存テストが通ること
2. `make test` — 全テスト green
3. 実動作: memoria worker start → Claude Code で user-prompt hook が発火 → ingest.log に `database is closed` が出ないこと
4. `tail -f ~/.local/state/memoria/logs/ingest.log` でエラーが消えていること
