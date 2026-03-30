# memoria

memoria は、Claude Code 向けのプロジェクト認識型 LLM メモリシステムです。

コーディングセッションから知識を自動抽出し、構造化して保存し、後から再利用できるようにします。単一プロジェクトだけでなく、類似プロジェクト間での知見共有も前提にしています。

## ドキュメント

- English README: `README.md`
- 超詳細設計書: `docs/specs/SPEC.ja.md`
- CLI 設計: `docs/specs/CLI.ja.md`
- Hook 契約: `docs/specs/HOOKS.ja.md`
- スキーマ設計: `docs/specs/SCHEMA.ja.md`
- Worker 設計: `docs/specs/WORKERS.ja.md`
- Retrieval 設計: `docs/specs/RETRIEVAL.ja.md`
- スキル: `skills/memoria/SKILL.md`

## 特徴

- Claude Code hook による自動記憶
- 同一プロジェクト優先の retrieval
- 類似プロジェクトの知識共有
- SQLite ベースのローカル完結構成
- Go 製 ingest worker
- Python 製 embedding worker
- Claude Code plugin 同梱
- Agent skill 同梱

## アーキテクチャ

```text
Claude Code
  -> plugin (hooks + skill)
    -> memoria CLI (Go / Kong)
      -> ingest worker (Go)
      -> embedding worker (Python / uv / UDS)
      -> SQLite
```

## インストール

### Go

```bash
go install github.com/youyo/memoria@latest
```

### Homebrew（予定）

```bash
brew install youyo/tap/memoria
```

## セットアップ

### 1. 設定初期化

```bash
memoria config init
```

### 2. Hook 設定出力

```bash
memoria config print-hook
```

### 3. Claude Code plugin を導入

Claude Code 上で:

```text
/plugin
```

マーケットプレイスに `youyo/memoria` を追加し、プラグインを有効化します。

## 基本コマンド

```bash
memoria memory search "SessionEnd が信用できない理由"
memoria worker status
memoria doctor
```
