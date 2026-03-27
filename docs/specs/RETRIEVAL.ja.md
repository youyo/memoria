# memoria Retrieval 詳細設計

## 設計原則

memoria の retrieval は、単純な semantic search ではなく、プロジェクト文脈を強く意識する。

優先順位:

```text
same project > similar project > global
```

ただし UserPromptSubmit では relevance も重要であるため、完全な project filter にはしない。

## プロジェクト識別

### なぜ cwd だけでは不足か

- サブディレクトリ差
- monorepo
- 一時ディレクトリ
- パス移動

このため、`project_id` は repo root を優先して生成する。

### project_id 生成

1. git repo root があればそれを使う
2. なければ cwd を使う
3. 正規化した root path を hash 化する

## similar project

### 目的

今のプロジェクトと直接同一ではないが、構成や技術スタックが近いプロジェクトの知見を借りる。

### fingerprint 材料

- repo root / repo name
- primary language
- framework
- build / package manager
- CLI / web / infra などの project kind
- `go.mod`, `pyproject.toml`, `.goreleaser.yaml`, `skills/`, `.claude/` などの存在
- project summary

### fingerprint の表現

- `fingerprint_json`: 構造化表現
- `fingerprint_text`: embedding 対象の自然言語表現

### similarity 計算

1. ルールベースで候補抽出
2. fingerprint embedding で類似度計算
3. `project_similarity` にキャッシュ

## SessionStart retrieval

### 目的

セッション開始時に、現在プロジェクトに強く関係する recent / important memories を薄く注入する。

### バケツ

- same project
- similar project
- global

### 推奨配分

- same project: 2 件
- similar project: 1 件
- global: 1 件

### 重みの考え方

- same project boost を最優先
- importance / recency を加味
- semantic は弱くてよい

## UserPromptSubmit retrieval

### 目的

ユーザープロンプトに直接関係する memory を返す。

### 手順

1. prompt を embedding
2. FTS top-N を取得
3. vector top-N を取得
4. RRF で統合
5. project / similar project / global boost を加算
6. top-N を Claude に渡す

## スコア構成

### UserPromptSubmit

概念的には以下。

```text
semantic relevance + fts relevance + same project boost + similar project boost + recency + importance
```

### SessionStart

概念的には以下。

```text
same project boost + similar project boost + importance + recency + weak semantic
```

## LLM enrichment と retrieval の関係

chunk 保存時に LLM が以下を付与する。

- `summary`
- `kind`
- `importance`
- `scope`
- `project_transferability`
- `keywords`
- `applies_to`

特に `scope` と `project_transferability` は similar project retrieval の精度に効く。

## 出力整形

### SessionStart

- 2〜4 件
- 1 行要約
- 400〜800 文字程度

### UserPromptSubmit

- 3〜5 件
- summary 優先
- 600〜1200 文字程度
