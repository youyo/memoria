# memoria ユーザビリティ改善プラン

## Context

`memoria memory list` で表示される8文字短縮IDで `memoria memory get` できない問題、および session-start hook が `<task-notification>` XML やシステムメタデータを「重要な decision」として返す品質問題を修正する。

実際に使ってみて発覚した「使いづらさ」の根本原因を修正する。

---

## 問題1: memory get が短縮IDに対応していない

- `memory list` は `ChunkID[:8]` で8文字表示（`internal/cli/memory.go:260`）
- `memory get` は `WHERE chunk_id = ?` で完全一致（`internal/cli/memory.go:129`）
- → list で見えるIDをコピペしても get できない

### 修正

**ファイル**: `internal/cli/memory.go`

- `MemoryGetCmd.Run()` のクエリを `WHERE chunk_id LIKE ?||'%'` に変更（git の短縮ハッシュ方式）
- 複数件ヒット時はエラーメッセージで候補一覧を表示
- 0件時は現行通り "not found"

---

## 問題2: ゴミチャンクが高重要度で保存される

session-start hook の出力に以下のようなノイズが含まれる：
```
**[decision]** (重要度: 0.9)
User: <task-notification><task-id>aff04ab02bbd0a8dd</task-id>...
```

### 根本原因

1. **pre-enrichment フィルタリングがゼロ** — `<task-notification>` XML、tool-use-id、システムパスなどが全てチャンクとして保存される
2. **enricher のキーワードマッチが雑** — XML 内に「決定」等があれば decision 扱い
3. **importance スコアが積み上がりすぎ** — base 0.3 + kind 0.3 + keyword 0.2 = 0.8 が容易に発生

### 修正

**ファイル**: `internal/ingest/enricher.go`

チャンク内容のノイズ判定関数 `isNoise(content string) bool` を追加：
- `<task-notification>`, `<tool-use-id>`, `<command-name>`, `<command-message>` 等の Claude Code 内部XMLタグを検出
- `<system-reminder>` タグを検出
- 内容の大半がXMLタグで構成されている場合（タグ除去後の文字数が元の30%未満）
- `/Users/.../.claude/plugins/` 等のシステムパスのみの内容

**ファイル**: `internal/ingest/chunker.go`

- `Chunk()` 関数内で `isNoise()` チェックを追加し、ノイズチャンクをスキップ

---

## 問題3: FormatContext のサマリーフォールバック

**ファイル**: `internal/retrieval/retrieval.go`

`FormatContext()` で `Summary` が空の場合に生の `Content` にフォールバックし、XMLタグがそのまま出力される（`retrieval.go:378-380`）。

### 修正

- フォールバック時に `Content` にも `makeSummary()` 相当のクリーニング（XMLタグ除去、プレフィックス除去）を適用
- クリーニング関数を `internal/ingest/enricher.go` から export するか、`retrieval` パッケージ内に同等のヘルパーを追加

---

## 実装順序

1. **問題1**: memory get 短縮ID対応（独立、即効性高い）
2. **問題2**: ノイズフィルタリング追加（enricher + chunker）
3. **問題3**: FormatContext クリーニング（retrieval）

## 対象ファイル

| ファイル | 変更内容 |
|---|---|
| `internal/cli/memory.go` | get の prefix match 対応 |
| `internal/ingest/enricher.go` | `isNoise()` 関数追加、export 用クリーニング関数 |
| `internal/ingest/chunker.go` | `Chunk()` にノイズスキップ追加 |
| `internal/retrieval/retrieval.go` | `FormatContext()` のフォールバッククリーニング |

## 検証

```bash
# 問題1: 短縮IDで get できることを確認
memoria memory list
memoria memory get <8文字ID>

# 問題2+3: テスト実行
make test

# 問題2: ノイズフィルタのユニットテスト追加
go test ./internal/ingest/ -run TestIsNoise

# E2E: session-start hook の出力にXMLノイズが含まれないことを確認
echo '{"session_id":"test","cwd":"/tmp"}' | memoria hook session-start
```
