---
title: /memoria:timeline スキル追加
project: memoria
author: planning-agent
created: 2026-03-31
status: Draft
complexity: L
---

# /memoria:timeline スキル追加

## Context

claude-mem の `timeline` ツール相当の機能を memoria のプラグインスキルとして追加する。これにより memoria が claude-mem の代替として機能する範囲が広がる。

claude-mem の timeline: 特定の observation ID を anchor に前後のコンテキストを取得する MCP ツール。

memoria の既存資産:
- `memoria memory list` — 時系列降順で chunk を一覧表示（`--project`, `--kind`, `--limit` フィルタ対応）
- `memoria memory get <chunk_id>` — 特定の chunk を ID で取得
- `memoria memory search <query>` — FTS + vector 検索
- `skills/search/SKILL.md` — `/memoria:search` スキル（実装済み）

## 変更ファイル

| ファイル | 操作 | 概要 |
|---------|------|------|
| `skills/timeline/SKILL.md` | 新規 | `/memoria:timeline` スキル定義 |

## 実装手順

### Step 1: `skills/timeline/SKILL.md` 新規作成

`memoria memory list` を活用し、日付・kind・プロジェクトでフィルタ可能なタイムラインビュー。

```yaml
---
name: timeline
description: Show a chronological timeline of memoria's stored memories. Use when reviewing what happened in recent sessions, understanding project history over time, or getting an overview of stored knowledge. Supports filtering by kind and project.
argument-hint: [--kind <kind>] [--limit <n>]
allowed-tools: Bash(memoria *)
---

Show a chronological timeline of memories stored in memoria.

## Execution

Run the appropriate command based on arguments:

Default (recent 20 entries):
\`\`\`bash
memoria memory list --limit 20
\`\`\`

With kind filter:
\`\`\`bash
memoria memory list --kind $ARGUMENTS[0] --limit 20
\`\`\`

With limit:
\`\`\`bash
memoria memory list --limit $ARGUMENTS[0]
\`\`\`

Parse $ARGUMENTS flexibly:
- If a number is given, use it as --limit
- If a kind name is given (decision/constraint/todo/failure/fact/preference/pattern), use it as --kind
- If both are given, apply both
- If "all" is given, use --limit 100

Present results as a formatted timeline with timestamps, kinds, and summaries.
For each entry, show the chunk_id prefix (first 8 chars) so the user can drill down with `/memoria:search` or `memoria memory get`.

## When to use

- Reviewing what happened in recent sessions
- Getting an overview of stored decisions and constraints
- Understanding the chronological flow of a project
- Checking what memories exist before searching for specific ones

## Output format

Present as a clean timeline:
\`\`\`
[2026-03-30] 🔴 decision  abc12345  stakeholder と議論し requirement を decided
[2026-03-30] 🟡 pattern   def67890  func handleError() で panic: を workaround した
[2026-03-29] 🔵 fact      ghi24680  SQLite の FTS5 を使った全文検索の実装
\`\`\`
```

## CI 修正（v0.2.0 で発生）

`internal/cli/daemon_test.go` の `TestDaemonSubcommand_Help` が CI で 120秒タイムアウト。idle timeout 廃止で daemon が永久実行になり、`--help` でも `Run()` が呼ばれて daemon が起動してしまう。

修正: テストで daemon 実行を避けるため、`--help` の結果を parse だけで確認するか、テスト自体にタイムアウトを設定。

## 変更ファイル（追加）

| ファイル | 操作 | 概要 |
|---------|------|------|
| `internal/cli/daemon_test.go` | 修正 | TestDaemonSubcommand_Help のタイムアウト対策 |

## 検証手順

```bash
# 1. CI 修正確認
go test ./internal/cli/ -v -run TestDaemon -timeout 30s

# 2. プラグイン再インストール後
# Claude Code で /memoria:timeline を実行
# Claude Code で /memoria:timeline decision を実行
# Claude Code で /memoria:timeline 50 を実行
```

---
## Next Action

> **このプランが承認されました。以下を順番に実行してください:**
>
> 1. `Skill(devflow:implement)` — このプランに基づいて実装を開始
>
> ユーザーの追加指示は不要です。即座に実行してください。
