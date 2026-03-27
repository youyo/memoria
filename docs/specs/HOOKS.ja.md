# memoria Hook 契約詳細設計

## 対象 hook

- SessionStart
- UserPromptSubmit
- Stop
- SessionEnd

Claude Code の hook は stdin に JSON を渡し、SessionStart と UserPromptSubmit は stdout の追加文脈注入を利用できる。memoria はこの hook 契約に従い、停止制御ではなく memory 補助に専念する。

## 共通方針

- すべての hook コマンドは stdin JSON 入力
- SessionStart / UserPromptSubmit は stdout に JSON を返す
- Stop / SessionEnd は stdout 空
- 基本 exit 0
- stderr はログ用途のみ

## SessionStart

### 役割

- embedding worker を ensure
- 現在プロジェクトを解決
- same project / similar project / global から relevant memories を取得
- 短い `additionalContext` を返す

### 入力

- `session_id`
- `cwd`
- `transcript_path`
- `source` (`startup`, `resume`, `clear`, `compact`)

### 出力

```json
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "..."
  }
}
```

### 失敗時

- retrieval 失敗でも空コンテキストを返す
- セッション開始は妨げない

## UserPromptSubmit

### 役割

- prompt を embedding
- FTS + vector + project/similarity boost で検索
- `additionalContext` を返す

### 入力

- `session_id`
- `cwd`
- `prompt`

### 出力

```json
{
  "hookSpecificOutput": {
    "hookEventName": "UserPromptSubmit",
    "additionalContext": "..."
  }
}
```

## Stop

### 役割

- `last_assistant_message` を受け取る
- 軽い判定で checkpoint 保存対象か判断
- `checkpoint_ingest` job を enqueue
- ingest worker を ensure

### 設計意図

SessionEnd は必ずしも保証されないため、重要な意思決定や制約を Stop で早取りする。

### 入力

- `session_id`
- `cwd`
- `last_assistant_message`

### 出力

なし

## SessionEnd

### 役割

- transcript path を受け取る
- `session_end_ingest` job を enqueue
- ingest worker を ensure

### 設計意図

SessionEnd 自体は timeout 制約が厳しいため、重い処理は行わず queue 投入だけに限定する。

### 入力

- `session_id`
- `cwd`
- `transcript_path`
- `reason`

### 出力

なし

## timeout 方針

- SessionStart: 2〜5 秒程度を許容
- UserPromptSubmit: 2〜5 秒程度
- Stop: 1〜2 秒程度
- SessionEnd: 1 秒未満を目標

## fallback 方針

- retrieval 失敗時: 空の `additionalContext`
- enqueue 失敗時: stderr に記録、hook は継続
- block 用途には使わない
