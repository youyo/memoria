---
title: Retrieval 時間減衰を指数減衰に改善
project: memoria
author: planning-agent
created: 2026-03-31
status: Draft
complexity: L
---

# Retrieval 時間減衰（Time Decay）の改善

## Context

sui-memory との比較で発見されたギャップ。memoriaの現在のrecency計算は双曲線減衰 `1/(days+1)` を使用しており、減衰が急すぎる。

**現在の問題**（`internal/retrieval/retrieval.go:80,92,113`）:
```sql
1.0 / (julianday('now') - julianday(c.created_at) + 1)
```
- 1日後: 0.50（半減）
- 7日後: 0.125
- 30日後: 0.032

1週間前の重要な意思決定が不当に低ランクになる。

**改善後**（半減期30日）:
```sql
1.0 / (1.0 + (julianday('now') - julianday(c.created_at)) / ?)
```
- 1日後: 0.97
- 7日後: 0.81
- 30日後: 0.50
- 60日後: 0.33
- 90日後: 0.25

人間の記憶に近い緩やかな減衰。SQLite標準関数のみで実現（`exp()` 不要）。

## 変更ファイル

| ファイル | 操作 | 概要 |
|---------|------|------|
| `internal/retrieval/retrieval.go` | 修正 | 3箇所のrecency式を差し替え、定数定義追加 |
| `internal/retrieval/retrieval_test.go` | 修正 | recencyスコアの期待値を更新 |

## 実装手順

### Step 1: 定数定義 + SQL修正

`internal/retrieval/retrieval.go` に定数を追加:
```go
const recencyHalfLifeDays = 30.0
```

3箇所の SQL を修正（行80, 92, 113）:
```sql
-- before
+ (1.0 / (julianday('now') - julianday(c.created_at) + 1)) AS score

-- after
+ (1.0 / (1.0 + (julianday('now') - julianday(c.created_at)) / ?)) AS score
```

`?` パラメータに `recencyHalfLifeDays` をバインド。各 `QueryContext` の引数に追加。

### Step 2: テスト更新

既存テストでrecencyスコアの期待値がある箇所を新しい減衰カーブに合わせて更新。

## テスト設計書

| ID | テスト | 期待結果 |
|----|--------|---------|
| T1 | 当日作成のchunk → recency ≈ 1.0 | score の recency 成分が約 1.0 |
| T2 | 30日前のchunk → recency ≈ 0.5 | 半減期が正しく機能 |

## リスク評価

| リスク | 重大度 | 対策 |
|--------|--------|------|
| 既存テストの期待値ずれ | 低 | テスト更新で対応 |
| スコアバランスの変化 | 低 | recencyの重みは元々小さい（boost +2.0〜3.0 に対し recency は 0〜1.0） |

## 検証手順

```bash
go test ./internal/retrieval/... -count=1 -v
go test ./... -count=1
```

---
## Next Action

> **このプランが承認されました。以下を順番に実行してください:**
>
> 1. `Skill(devflow:implement)` — このプランに基づいて実装を開始
>
> ユーザーの追加指示は不要です。即座に実行してください。
