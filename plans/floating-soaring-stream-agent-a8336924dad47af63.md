# セッション終了時のfailure誤分類問題の調査報告

## 問題の要約

ユーザーが指摘した observation ID "8fbac2c0" では、セッション終了時（exitコマンド実行時）に、チャンク化されたコンテンツが **kind="failure"** として誤分類されている問題があります。

## 根本原因の分析

### 1. enricher.go の inferKind() ロジック

**現在の失敗判定キーワード (行61):**
```go
{"failure", []string{"failed", "失敗", "エラー", "error", "バグ", "bug", "crash", "不具合", "exception"}}
```

**問題点:**
- "exit" という単語は失敗キーワードに含まれていない ✓ (これは正しい)
- しかし、セッション終了時の会話に **"error", "exception", "crash"** などのキーワードが含まれると、
  その会話全体がfailureと分類される ✗ (これが問題)

### 2. チャンク化の処理フロー

**chunker.go の Chunk() 関数:**
- User + Assistant ペアを1つのチャンクにまとめる
- tool ターンは直前の U/A ペアに包含される
- MaxChunkBytes を超える場合は分割される

**シナリオ例:**
```
ターン1: User: "How to fix this error?"
ターン2: Assistant: "The error code is..."
↓ (チャンク化)
チャンク1: "User: How to fix this error?\n\nAssistant: The error code is..."
↓ (enrichment)
kind = "failure" ← "error" キーワードで誤分類
```

### 3. 影響を受けるケース

1. **セッション終了前の最終ターン**
   - ユーザー: "I got an error" / アシスタント: "Let me help"
   - 結果: 最後のチャンク = kind="failure"

2. **エラー処理に関する会話**
   - ユーザー: "Exception occurred"
   - 結果: kind="failure"

3. **明示的な失敗ではなく、単に失敗について説明した会話**
   - 「このバグを修正した」(過去)
   - 「エラーハンドリングのベストプラクティス」(技術情報)
   - → すべてkind="failure"と判定

## コード調査結果

### enricher.go (行27-76)

**inferKind() の実装:**
```go
func inferKind(lower string) string {
	for _, kp := range kindPatterns {
		for _, kw := range kp.keywords {
			if strings.Contains(lower, kw) {
				return kp.kind  // ← 最初のマッチで即座に返却
			}
		}
	}
	return "fact"
}
```

**問題:**
- 単純な substring マッチング（strings.Contains）を使用
- キーワードの優先度やコンテキストを考慮していない
- "error" → 文脈に関わらずfailure判定

### chunker.go (行25-79)

**Chunk() の実装:**
```go
// user + assistant ペアを1チャンク
// tool ターンは直前のペアに包含
```

- チャンク化自体は正しい
- ただし、セッション終了時の最後のターンもチャンク化される

### session_end.go (行137-177)

```go
// 各チャンクに enrichment を適用
for _, rawChunk := range rawChunks {
    enriched := ingest.Enrich(rawChunk.Content)
    // ← enriched.Kind が "failure" になる可能性
}
```

## 問題パターンの明確化

### パターン1: 技術的エラー説明の誤分類

```
入力: "User: How do I handle database errors?\n\nAssistant: Use try-catch blocks"
kind判定: "failure" ← "error" キーワードで誤分類
期待: "fact" または "pattern"
```

### パターン2: セッション終了時の文脈喪失

```
最後のチャンク: "User: Command failed\n\nAssistant: Session ending"
kind判定: "failure" ← "failed" キーワード
期待: "fact" (単なる会話のまとめ)
```

### パターン3: 過去形での失敗言及

```
入力: "User: We failed to optimize the index\n\nAssistant: Let's try a different approach"
kind判定: "failure" ← "failed" キーワード
期待: "decision" または "pattern" (過去の失敗から学んだ)
```

## キーワードマッチングの根本的な限界

**現在の方式:**
```go
if strings.Contains(lower, "error") {
    return "failure"  // ← 過度に単純
}
```

**問題ケース:**
- "error handling" → failure? → NO (技術情報)
- "debugging errors" → failure? → NO (学習)
- "The test error was fixed" → failure? → NO (解決済み)
- "Connection error occurred" → failure? → YES (実際の問題)

## 改善方向の検討

### 方向1: キーワードの絞り込み (低コスト)
- "failed", "crash", "exception" は保持
- "error" は削除 (too generic)
- より具体的な失敗表現を追加:
  - "broken", "not working", "malfunction"
  - "stop working", "unable to", "cannot open"

### 方向2: コンテキスト認識 (中程度コスト)
- キーワード周辺のテキストを分析
- "error handling", "error message" → technology, fact
- "encountered error" → failure
- "fixed the error" → decision, pattern

### 方向3: LLM推定 (高コスト)
- Enrich時にLLMを呼び出す
- 正確だが処理速度が低下

### 方向4: ユーザーガイドラインの提供 (非実装)
- ユーザーに対して、セッション終了時の記述方法を指導

## 観察と提案

### 即座に実装可能な改善

1. **"error" キーワードの削除**
   - 過度にgenericなため、多くの誤分類の原因
   - "failed", "crash", "exception" で十分

2. **より具体的な失敗表現の追加**
   - "broken", "malfunction", "not working"
   - "unable to", "cannot open"

3. **セッション終了特有の処理検討**
   - Reason フィールド ("exit", "crash" など) の活用
   - セッション終了時は自動的にfactに正規化?

### 実装の複雑さ

**低:**
- "error" キーワード削除: 1行変更

**中:**
- キーワード見直し: 5-10行変更
- 正規表現マッチング導入: 50行追加

**高:**
- LLM推定: 新機能
- コンテキスト分析: 複雑なロジック

## テストケース観察

enricher_test.go にはfailure判定テストがあるが、exitコマンドやセッション終了後のコンテンツを含むテストは**存在しない**。

```go
func TestEnrichKindFailure(t *testing.T) {
	result := ingest.Enrich("The test failed with error: connection refused")
	if result.Kind \!= "failure" {
		t.Errorf("expected kind=failure, got %s", result.Kind)
	}
}
// exitコマンド後のコンテンツをテストしていない
```

## 次のアクション

1. **根本原因の確認**
   - 実際のトランスクリプトで観察ID 8fbac2c0のコンテンツを確認
   - どのターンが kind="failure" と分類されたか

2. **改善案の実装優先度**
   - "error" キーワード削除の影響範囲を調査
   - テストケース追加

3. **セッション終了時の特別扱い検討**
   - Reason フィールドを enrichment に渡すか?
   - session_end.go で特別処理を追加するか?


---

## 詳細コード分析サマリー

### ファイル: internal/ingest/enricher.go

**関連行:**
- 13: EnrichedChunk.Kind フィールド定義
- 29-51: Enrich() メイン関数
- 54-64: kindPatterns 配列定義
- 66-76: inferKind() 実装

**キーワードマッチング優先度:**
1. decision (line 58)
2. constraint (line 59)
3. todo (line 60)
4. failure (line 61) ← **問題の根源**
5. preference (line 62)
6. pattern (line 63)
7. fact (デフォルト)

**重要:** failure のキーワード に "error" が含まれている (line 61)

### ファイル: internal/ingest/chunker.go

**関連行:**
- 29-79: Chunk() 関数 (turn → RawChunk変換)
- 25-28: ChunkInput と RawChunk 構造体定義

**処理フロー:**
```
1. turns を iterate
2. user ターンを探す (startIdx)
3. その後の tool/assistant を収集 (endIdx まで)
4. parts に "User: " + content を追加
5. strings.Join(parts, "\n\n") でコンテンツ生成
6. MaxChunkBytes を超える場合は分割
```

**重要:** チャンク生成時に user + assistant の会話文全体が1つのコンテンツになる

### ファイル: internal/worker/session_end.go

**関連行:**
- 143-177: Handle() 関数内のchunk処理ループ
- 153: ingest.Enrich(rawChunk.Content) が呼ばれる ← ここでfailure誤分類が発生

**処理フロー:**
```
1. TranscriptPath から ParseTranscript() で []Turn を取得
2. Chunk(input) で []RawChunk に変換
3. 各 RawChunk について:
   a. Enrich() を呼び出し
   b. kind を確認して InsertChunk() で DB保存
```

### ファイル: internal/ingest/transcript.go

**関連行:**
- 49-124: ParseTranscript() 実装
- 16-21: Turn 構造体 (Role, Content, CreatedAt)

**重要:** トランスクリプトは JSONL 形式で、各行が1つのターンを表す

---

## 失敗シナリオの具体例

### 例1: セッション終了前のトラブルシューティング会話

**トランスクリプト内容:**
```jsonl
{"type":"user","message":{"role":"user","content":"I'm getting an error message"},...}
{"type":"assistant","message":{"role":"assistant","content":"Try checking the logs"},...}
```

**チャンク化後:**
```
Content: "User: I'm getting an error message\n\nAssistant: Try checking the logs"
```

**enrichment結果:**
```
kind: "failure"  ← "error" キーワードでマッチ ✗
期待: "fact" または "help-seeking"
```

### 例2: バグ修正に関する説明

**トランスクリプト内容:**
```jsonl
{"type":"user","message":{"role":"user","content":"We fixed the bug with exception handling"},...}
{"type":"assistant","message":{"role":"assistant","content":"Good approach"},...}
```

**チャンク化後:**
```
Content: "User: We fixed the bug with exception handling\n\nAssistant: Good approach"
```

**enrichment結果:**
```
kind: "failure"  ← "bug" + "exception" キーワードでマッチ ✗
期待: "decision" (過去のアクション)
```

### 例3: 技術教育

**トランスクリプト内容:**
```jsonl
{"type":"user","message":{"role":"user","content":"How to handle database errors?"},...}
{"type":"assistant","message":{"role":"assistant","content":"Use try-catch blocks"},...}
```

**チャンク化後:**
```
Content: "User: How to handle database errors?\n\nAssistant: Use try-catch blocks"
```

**enrichment結果:**
```
kind: "failure"  ← "error" キーワードでマッチ ✗
期待: "pattern" または "knowledge"
```

---

## 整理された問題リスト

| # | 問題 | 根因 | ファイル | 行番号 |
|---|------|------|--------|-------|
| 1 | "error" キーワードが過度にgeneric | inferKind 実装 | enricher.go | 61, 70 |
| 2 | コンテキスト認識なし | strings.Contains のみ使用 | enricher.go | 70 |
| 3 | セッション終了時の会話が失敗扱い | Reason フィールドの未活用 | session_end.go | 153 |
| 4 | テストカバレッジ不足 | exitケースなし | enricher_test.go | N/A |

---

## 修正案の詳細

### 案1: "error" キーワード削除 (推奨)

**変更:**
```go
// before
{"failure", []string{"failed", "失敗", "エラー", "error", "バグ", "bug", "crash", "不具合", "exception"}}

// after
{"failure", []string{"failed", "失敗", "バグ", "bug", "crash", "不具合", "exception"}}
```

**効果:**
- "error" は "error handling", "error message" など技術用語として使われることが多い
- 削除により誤分類を減らせる
- "failed" と "exception" で実際の失敗を捉える

**影響:**
- 既存テストの変更必要: TestEnrichKindFailure() など

### 案2: Reason フィールドの活用

**session_end.go の変更:**
```go
// enrichment の前に
enriched := ingest.Enrich(rawChunk.Content)

// 追加: "exit" "crash" など正常終了の場合は kind を上書き
if payload.Reason == "exit" && enriched.Kind == "failure" {
    enriched.Kind = "fact"
}
```

**効果:**
- セッション正常終了時（Reason="exit"）の不要な failure 分類を防ぐ
- 明示的なエラー終了（Reason="crash" など）は failure のまま

### 案3: コンテキスト認識 (応急対応)

**enricher.go の拡張:**
```go
func inferKindWithContext(lower string) string {
    // "error" は以下のコンテキストでは failure と判定しない
    if strings.Contains(lower, "error") {
        if strings.Contains(lower, "handling") ||
           strings.Contains(lower, "message") ||
           strings.Contains(lower, "fixed") {
            // fact にダウングレード
            // ... 継続判定
        } else {
            return "failure"
        }
    }
    // ... 他のキーワード判定
}
```

**効果:**
- より正確な分類
- 実装複雑度は中程度

---

## テスト追加の提案

```go
// enricher_test.go に追加

func TestEnrichKindExitSession(t *testing.T) {
    result := ingest.Enrich("User: exit\n\nAssistant: Goodbye")
    if result.Kind == "failure" {
        t.Errorf("exit session should not be failure, got %s", result.Kind)
    }
}

func TestEnrichKindErrorHandling(t *testing.T) {
    result := ingest.Enrich("How to handle database errors?")
    if result.Kind == "failure" {
        t.Errorf("error handling discussion should not be failure, got %s", result.Kind)
    }
}

func TestEnrichKindBugFixed(t *testing.T) {
    result := ingest.Enrich("We fixed the bug with error handling")
    if result.Kind == "failure" {
        t.Errorf("fixed bug should not be failure, got %s", result.Kind)
    }
}
```

