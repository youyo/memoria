---
title: FTS5 日本語検索対応 — trigram トークナイザー移行
project: memoria
author: planning-agent
created: 2026-03-30
status: Draft
complexity: M
---

# FTS5 日本語検索対応: trigram トークナイザー移行

## Context

memoria の FTS5 全文検索テーブル `chunks_fts` はデフォルトの `unicode61` トークナイザーを使用している。`unicode61` は空白区切りでトークン化するため、日本語のように空白なしで連続する文字列を正しくトークン化できず、「ワーカー」「健全性チェック」などの日本語クエリで `no results` になる。

`trigram` トークナイザーに変更することで、3文字以上の部分文字列マッチが可能になり、日本語検索が正常に動作する。modernc.org/sqlite（Pure Go）での動作は検証済み。

## スコープ

### 実装範囲
- マイグレーション 0003: `chunks_fts` を trigram トークナイザーで再作成
- `FTSSearch()`: bm25 → 位置ベーススコアに変更（trigram は bm25 非対応）
- `buildFTSQuery()`: 3文字未満トークンのフィルタリング追加
- テスト: 日本語 FTS テスト追加、スキーマバージョン更新

### スコープ外
- ベクトル検索の変更（変更なし）
- RRF マージロジックの変更（変更なし）
- 既存の英語テストの変更（そのまま動作する）

## 変更ファイル一覧

| ファイル | 操作 | 概要 |
|---------|------|------|
| `internal/db/migrations/0003_trigram_fts.sql` | 新規 | trigram マイグレーション |
| `internal/retrieval/retrieval.go` | 修正 | FTSSearch + buildFTSQuery |
| `internal/retrieval/retrieval_test.go` | 修正 | 日本語テスト追加 |
| `internal/db/migrate_test.go` | 修正 | スキーマバージョン 2→3 |

## 実装手順

### Step 1: マイグレーション `0003_trigram_fts.sql`

トリガー削除 → FTS テーブル再作成 → トリガー再作成 → インデックス再構築。

```sql
-- 1. トリガー削除
DROP TRIGGER IF EXISTS chunks_fts_insert;
DROP TRIGGER IF EXISTS chunks_fts_delete;
DROP TRIGGER IF EXISTS chunks_fts_update;

-- 2. FTS テーブル再作成（trigram）
DROP TABLE IF EXISTS chunks_fts;
CREATE VIRTUAL TABLE chunks_fts USING fts5(
    content, summary, keywords,
    content='chunks', content_rowid='rowid',
    tokenize='trigram'
);

-- 3. トリガー再作成（0001 と同一）
-- INSERT / DELETE / UPDATE の3トリガー

-- 4. インデックス再構築
INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild');
```

### Step 2: `retrieval.go` — FTSSearch のスコアリング変更

**変更理由**: trigram トークナイザーは `bm25()` 非対応。

- SQL から `bm25(chunks_fts) as fts_score` を削除
- `ORDER BY c.created_at DESC` に変更（recency ベース）
- Scan から `ftsScore` を削除
- 結果に位置ベーススコアを付与: `rr.Score = 1.0 / float64(i+1)`

RRF マージで vector search と合成されるため、FTS 単体の精密なランキングは不要。

### Step 3: `retrieval.go` — buildFTSQuery の 3 文字フィルタ

**変更理由**: trigram は 3 文字未満のクエリでエラーになる。

- `unicode/utf8` をインポート
- `escapeFTSToken` 後、引用符を除いた内容が `utf8.RuneCountInString(inner) < 3` ならスキップ
- 全トークンがフィルタされた場合は空文字列を返す（vector search にフォールバック）

### Step 4: テスト追加・更新

- `migrate_test.go`: スキーマバージョン `2 → 3` に更新
- `retrieval_test.go`: 日本語 FTS テスト追加

## テスト設計書

### 正常系

| ID | 入力 | 期待出力 |
|----|------|---------|
| J1 | chunk=「ワーカーの起動に失敗しました」, query=「ワーカー」 | 1件ヒット |
| J2 | chunk=「SQLite の FTS5 を使った全文検索」, query=「全文検索」 | 1件ヒット |
| J3 | chunk=「SQLite の FTS5」, query=「SQLite」 | 1件ヒット（英語6文字） |
| J4 | chunk=「ワーカーの起動に失敗」, query=「起動に失敗」 | 1件ヒット |

### 異常系・エッジケース

| ID | 入力 | 期待出力 | 理由 |
|----|------|---------|------|
| E1 | query=「Go is」 | 0件（空） | 全トークン < 3文字 |
| E2 | query=「失敗」 | 0件（空） | 2文字 < 3文字最小 |
| E3 | query=「AI Go ワーカー」 | 「ワーカー」のみで検索 | 短いトークンをフィルタ |

## リスク評価

| リスク | 重大度 | 対策 |
|--------|--------|------|
| trigram で bm25 不可 | 中 | 位置ベーススコア + RRF で補完 |
| 3文字未満の英語トークン（Go, AI等）がFTS検索不可 | 低 | vector search でカバー |
| trigram インデックスサイズ増大 | 低 | 個人用DB、データ量小 |
| 部分文字列マッチで false positive 増加 | 低 | RRF + vector で精度補完 |

## 検証手順

```bash
# 1. 全テスト実行
go test ./... -v

# 2. 日本語検索テスト
go test ./internal/retrieval/ -run TestFTSSearch_Japanese -v

# 3. 手動検証（実装後）
memoria memory search "ワーカー" --limit 5
memoria memory search "起動" --limit 5
```

## チェックリスト
- [x] 観点1: 実装実現可能性（手順の一貫性、ファイル網羅）
- [x] 観点2: TDDテスト設計（正常系4件、異常系3件）
- [x] 観点3: アーキテクチャ整合性（既存マイグレーションパターン踏襲）
- [x] 観点4: リスク評価と対策（4項目）
- [x] 観点5: シーケンス図 — N/A（単純なスキーマ変更+クエリ修正、複雑なフローなし）

---
## Next Action

> **このプランが承認されました。以下を順番に実行してください:**
>
> 1. `Skill(devflow:implement)` — このプランに基づいて実装を開始
>
> ユーザーの追加指示は不要です。即座に実行してください。
