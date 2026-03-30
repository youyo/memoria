---
title: CLI コマンド整理 — 不要コマンド削除 + completion zsh 実装 + worker restart 実装
project: memoria
author: planning-agent
created: 2026-03-31
status: Ready for Review
complexity: M
---

# CLI コマンド整理

## Context

memoriaのCLIに6つのスタブコマンド（"not implemented"）が残っている。
ユーザーが精査した結果、4つは不要（bash/fish completion, plugin list/doctor）、
2つは実装すべき（completion zsh, worker restart）と判断。
completion zsh は logvalet の実装パターンを踏襲する。

## スコープ

### 実装範囲
- 不要コマンド4つの削除（completion bash/fish, plugin list/doctor）
- `completion zsh` の本実装（logvalet パターン: 2層構造）
- `worker restart` の本実装（stop + start）
- 関連テストの修正・追加

### スコープ外
- bash / fish completion の実装
- plugin システムの設計・実装
- memory get の修正（調査の結果、完全実装済みと確認）

## 変更ファイル一覧

| ファイル | 操作 |
|---------|------|
| `internal/cli/plugin.go` | **削除** |
| `internal/cli/completion.go` | 大幅書き換え（bash/fish削除、zsh実装） |
| `internal/cli/root.go` | Plugin フィールド削除、CompletionCmd 簡素化 |
| `internal/cli/worker.go` | WorkerRestartCmd.Run() 実装 |
| `cmd/memoria/main.go` | kong.New+Parse に変更、補完インターセプト追加 |
| `internal/cli/cli_test.go` | Plugin テスト削除、Completion テスト修正 |
| `internal/cli/worker_test.go` | restart テスト書き換え |
| `internal/cli/completion_test.go` | **新規**: zsh 補完スクリプト生成テスト |
| `cmd/memoria/main_test.go` | **新規**: collectCompletions 統合テスト |

## 実装手順

### Step 1: 不要コマンド削除

1. `internal/cli/plugin.go` を削除
2. `internal/cli/root.go` から `Plugin PluginCmd` フィールドを削除
3. `internal/cli/completion.go` から `CompletionBashCmd`, `CompletionFishCmd` を削除
4. `internal/cli/root.go` の `CompletionCmd` から Bash/Fish フィールドを削除
5. テスト修正:
   - `cli_test.go`: `TestPluginSubcommands` 削除、`TestCompletionSubcommands` を zsh のみに変更
   - `TestNotImplementedCommands` から削除済み・実装済みコマンドを除外（空になれば関数ごと削除）

### Step 2: completion zsh 実装（層1: スクリプト生成）

**ファイル**: `internal/cli/completion.go`

- `completionScript(name string, short bool) string` — zsh補完スクリプト生成
- `CompletionZshCmd.Run()` — スクリプトを stdout に出力

zsh スクリプト内容:
```zsh
_memoria() {
  local -a completions
  completions=($(${words[1]} --completion-bash ${words[@]:1}))
  compadd -- $completions
}
compdef _memoria memoria
```

`--short` フラグは今回実装しない（スコープ外）。

**テスト**: `internal/cli/completion_test.go`
- compdef 含有確認
- --completion-bash 含有確認
- 引用符なし確認（`${words[@]:1}` が引用符で囲まれていない）

### Step 3: completion zsh 実装（層2: 動的補完候補生成）

**ファイル**: `cmd/memoria/main.go`

logvalet パターンを移植:

1. `kong.Parse()` → `kong.New()` + `parser.Parse()` に変更
   - `kong.New()` の error と `parser.Parse()` の error を個別にハンドリング
   - completion モード時は config reload をスキップしても問題ない（completion は config 非依存）
2. Parse 前に `handleCompletionBash(parser, os.Args[1:])` を挿入（config reload の前）
3. `collectCompletions(k *kong.Kong, args []string) ([]string, bool)`:
   - `--completion-bash` の位置検出
   - Kong `Model.Node` をコマンドツリーとして走査
   - フラグ候補: `node.AllFlags(true)` で親フラグ含む、Hidden/使用済み除外（`AllFlags(true)` の `true` は hidden フラグを除外する意味）
   - サブコマンド候補: `node.Children` から Hidden 除外
   - プレフィクスマッチング対応

**テスト**: `cmd/memoria/main_test.go`
- トップレベル: hook, worker, memory, config, completion, doctor, version が返る
- memory 配下: search, get, list, stats, reindex が返る
- リーフノード: フラグ（--format 等）が返る
- `--` 入力時: フラグのみ
- `--f` プレフィクス: --format が返る
- hidden (daemon) が含まれない
- `--completion-bash` なしで false

### Step 4: worker restart 実装

**ファイル**: `internal/cli/worker.go`

既存の `WorkerStartCmd.Run()` / `WorkerStopCmd.Run()` のシグネチャに合わせる:

```go
func (c *WorkerRestartCmd) Run(globals *Globals, w *io.Writer) error {
    stop := &WorkerStopCmd{}
    if err := stop.Run(globals, w); err != nil {
        return err
    }
    start := &WorkerStartCmd{}
    return start.Run(globals, w)
}
```

> Note: Start/Stop は内部で `config.Load()` / `openWorkerDB()` を直接呼んでいるため、
> Run() に cfg/lazyDB を渡す必要はない。

**テスト**: `worker_test.go` の既存テストを書き換え
- `parseForTest` で Kong パーサー経由のテスト（DB 不在時も graceful に動作することを確認）
- 出力に "stopped" or "was not running" + worker 起動関連メッセージが含まれることを検証

### Step 5: 検証

```bash
go vet ./...
go test ./... -count=1
memoria completion zsh          # スクリプト出力確認
memoria completion zsh --short  # alias 付き出力確認
eval "$(memoria completion zsh)" && memoria <TAB>  # 実際の補完動作
```

## テスト設計書

### 正常系

| ID | テスト | 入力 | 期待出力 |
|----|-------|------|---------|
| C1 | zsh スクリプト生成 | `completionScript("memoria", false)` | `compdef _memoria memoria` を含む |
| C2 | zsh に compdef 含有 | `completionScript("memoria")` | `compdef _memoria memoria` を含む |
| C3 | トップレベル補完 | `["--completion-bash", ""]` | `hook`, `worker`, `memory` 等を含む |
| C4 | サブコマンド補完 | `["--completion-bash", "memory"]` | `search`, `get`, `list` 等を含む |
| C5 | フラグ補完 | `["--completion-bash", "memory", "search", "--"]` | `--format` 等を含む |
| C6 | プレフィクス補完 | `["--completion-bash", "--f"]` | `--format` を含む |
| R1 | worker restart | 実行 | stop → start の順序実行 |

### 異常系・エッジケース

| ID | テスト | 入力 | 期待出力 |
|----|-------|------|---------|
| C7 | completion-bash なし | `["memory", "search"]` | `(nil, false)` |
| C8 | hidden コマンド除外 | `["--completion-bash", ""]` | `daemon` を含まない |
| C9 | 使用済みフラグ除外 | `["--completion-bash", "--format", "json", "memory", "--"]` | `--format` を含まない |
| C10 | 引用符なし確認 | `completionScript("memoria", false)` | `${words[@]:1}` が引用符で囲まれていない |

## リスク評価

| リスク | 重大度 | 対策 |
|--------|--------|------|
| plugin 削除で互換性破壊 | 低 | 全てスタブ、利用者なし |
| kong.Parse → kong.New+Parse 変更 | 中 | logvalet 実績あり。memoria 固有の config reload ロジック（main.go:51-64）は completion 前に位置するが、completion は config 非依存のため干渉なし |
| zsh 補完のシェル互換性 | 低 | logvalet で動作確認済みパターン |
| completion 削除後のエラーメッセージ | 低 | `memoria completion bash` 実行時は Kong デフォルトエラーで十分 |

## ドキュメント更新

- `CLAUDE.md` の CLI コマンド一覧から plugin を削除（該当箇所がある場合）
- completion の記述を zsh のみに更新

## チェックリスト

- [x] 観点1: 実装実現可能性 — 全ステップ具体的、依存関係明示、変更ファイル網羅
- [x] 観点2: TDD設計 — 正常系6件、異常系3件、Red→Green→Refactor 順序
- [x] 観点3: アーキテクチャ整合性 — logvalet パターン踏襲、Kong DI パターン維持
- [x] 観点4: リスク評価 — 3件特定、対策具体的
- [ ] 観点5: シーケンス図 — N/A（単純な CRUD/生成処理のため不要）

---

## Next Action

> **このプランが承認されたら:**
>
> 1. `Skill(devflow:implement)` — このプランに基づいて実装を開始
