# README セットアップ手順の簡素化

## Context

`memoria config init` と `memoria config print-hook` は不要（config 未作成でもデフォルト値で動作、hook 設定はプラグインが自動提供）。
Homebrew は「予定」ではなく既にリリース済み。セットアップ手順を2ステップに簡素化する。

## 修正内容

### README.md

```markdown
## Installation

### Homebrew

\```bash
brew install youyo/tap/memoria
\```

### Go install

\```bash
go install github.com/youyo/memoria@latest
\```

## Setup

In Claude Code:

\```text
/plugin
\```

Add marketplace: `youyo/memoria`, then enable the plugin.

That's it. Database, migrations, and workers are initialized automatically on first use.
```

### README.ja.md

```markdown
## インストール

### Homebrew

\```bash
brew install youyo/tap/memoria
\```

### Go

\```bash
go install github.com/youyo/memoria@latest
\```

## セットアップ

Claude Code 上で:

\```text
/plugin
\```

マーケットプレイスに `youyo/memoria` を追加し、プラグインを有効化します。

データベース・マイグレーション・worker は初回利用時に自動で初期化されます。
```

## 変更点

- `config init` / `config print-hook` ステップを削除
- Homebrew を「予定」→ 正式に第一選択肢として記載
- 「自動初期化」の説明を1行追加
- Go install は Homebrew の後に記載（推奨順）

## ファイル

- `README.md`
- `README.ja.md`
