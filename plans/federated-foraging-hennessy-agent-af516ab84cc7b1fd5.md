# 批評レポート: memoria マルチプロジェクト scope ポリシー

**目的**: 「コンテンツは閉じる、技術は開く」の原則に基づき、chunk の scope 分類を techScore / contentScore ヒューリスティックで自動化し、isolation_mode でプロジェクト隔離を可能にする計画の欠陥を列挙。

---

## 問題点リスト

### 1. [致命的] inferScope() の「技術」判定ロジックが未定義

**状況**: 計画では「技術キーワード（func, import, error:, github.com/ 等）→ techScore++」と記載されているが、**判定ロジックの詳細が完全に欠落している**。

**具体的な欠陥**:
- 「どのキーワードが「技術」か」を判定するリストが存在しない
- 現在の `inferScope()` は以下の3種類のみ:
  - "global" → 即座に "global"
  - "similarity_shareable" → 即座に "similarity_shareable"
  - default → "project"
  
  これは単なる substring match。スコアリングとは別物。

- 提案の「techScore / contentScore」方式では、**スコア計算ロジックの実装が0**状態。

**影響**: 実装者は「何を技術と見なすか」「contentScore >= 3 とは何か」を自分で決めなければならない。設計が不完全。

---

### 2. [致命的] 「contentScore >= 3: project（ベトー）」が暴力的すぎる

**状況**: 計画では「contentScore >= 3 の場合は絶対に project スコープ」とされている。

**問題**:
- **ベトーの粒度が粗すぎる**: contentScore が 2.9 なら similarity_shareable になるが、3.0 を超えたら問答無用に project になる。閾値が 1 離れるだけで全く違う結果になる。

- **既存 scope inference と矛盾**:  現在の enricher.go では "constraint" や "decision" が kind に割り当てられると、それは意思決定のマークにしかならない。scope は別にはじかれる。新しいロジックでは kind=decision/constraint で contentScore に加点するとしているが、これは **enricher の層を越えて scope に強制する** ことになり、責任分離が崩れる。

- **プロジェクト属性 (isolation_mode) との相互作用が未定義**: 「isolated プロジェクトではスコープを強制的に project に上書き」とあるが、この上書きのタイミング・優先順位が不明確。は、inferScope() が contentScore で project を決定した場合と、isolation_mode による強制が衝突しないか？

---

### 3. [致命的] isolation_mode の設定 UI/UX が計画に存在しない

**状況**: 計画では「projects テーブルに isolation_mode カラムを追加」とあるが、以下が完全に欠落。

**欠落項目**:
1. **設定方法**: `memoria config set --isolation-mode PROJECT_ID` のようなコマンドは存在しないか？
2. **初期値**: 新規プロジェクトは isolation_mode = false or true か？
3. **変更時の既存 chunk 処理**: isolation_mode を有効にした場合、既存チャンク（現在 scope = global など）をどう処理するのか？再計算か、放置か？
4. **Hook 実行時の判定タイミング**: SessionStart hook 実行時に isolation_mode を読む必要があるが、その責任は誰が負うのか（retrieval.go? hook.go?）

**影響**: 実装者が「どこに scope 上書きロジックを組むのか」を決められない。リスクが凝結する。

---

### 4. [重大] 既存データの再分類が計画に含まれていない

**状況**: 計画では「11ファイル修正」とあり、マイグレーション M04 が言及されているが、**既存チャンクの scope を再計算する メカニズムがない**。

**具体的な問題**:
- memoria は M15 完了時点で既にかなりのセッション・チャンクを蓄積している可能性がある。
- 現在の enricher では、ほぼ全チャンクが scope = "project" のままである（"global" や "similarity_shareable" はキーワードマッチベースなので稀）。
- 新しい inferScope() ロジックは既存チャンク には**一切作用しない**（新規チャンクだけ対象）。

**結果**:
- セッション開始時の retrieval は新規チャンク + 古いチャンク が混在。
- 古いチャンク は scope = "project" で固定されているため、UserPrompt での "-3.0 ペナルティ" は作用しない。
- 1000+ チャンクがあれば、新ロジックの効果は無視されるレベルになる。

**対策の欠落**: 
- `memoria memory reindex-scope` のようなコマンド？
- バッチスクリプト？
- マイグレーション？

計画では何も言及されていない。

---

### 5. [重大] UserPrompt の "-3.0 ペナルティ" は実際には機能しない可能性が高い

**状況**: 計画では「UserPrompt で他プロジェクトの project スコープは -3.0 ペナルティ」。

**分析**:
- 現在の `ApplyProjectBoost()` は以下のロジック:
  ```
  same project: +2.0
  similar project: +1.0
  (その他: +0.0)
  ```

- ペナルティ -3.0 は「他プロジェクトの project scope」を検出してから初めて作用する。

**問題**:
1. **FTS と vector では既にフィルタリングされている可能性**: UserPrompt で FTS や vector で top-20 を絞るステップがあるが、ここで「他プロジェクトの project」がそもそも上位 20 に入らない可能性がある。

   - 現在の検索数が "LIMIT 500" なので候補は多いが、FTS/vector だけでは「同一プロジェクト」優先にはならない。
   - RRF で統合後、ペナルティを足しても、すでに低スコアなら -3.0 を足してもほぼ変わらない。

2. **-3.0 の値は恣意的**: スコアスケールが FTS / vector / project boost で異なるため、-3.0 が「どれほど強い」ペナルティかが不明確。

3. **scope = similarity_shareable はペナルティがない**: 計画では「他プロジェクトの project スコープは -3.0」とあるが、similarity_shareable はペナルティなし。これは意図的か、見落としか？

---

### 6. [重大] Enricher と isolation の統合が責任境界を侵す

**状況**: 計画では「isolated プロジェクトではチャンクの scope を強制的に project に上書き」。

**問題**:
- `ingest/enricher.go` の `Enrich()` は現在、chunk content のみを入力で、プロジェクト情報を**受け取らない**。

  ```go
  func Enrich(content string) EnrichedChunk
  ```

- isolation_mode による上書きを実装するには、以下のいずれかが必要:
  1. `Enrich()` シグネチャを `Enrich(content string, projectID string, db *sql.DB)` に拡張する。
  2. Enrich 後に別ステップで上書きする（例: `applyIsolationMode(enriched, projectID, db)`）。

- **計画では「enricher に統合」と書かれているが、実装方法が不明確**。

**影響**: 1. を選ぶと enricher が DB に依存するようになり、テスト複雑度が上がる。2. を選ぶと enricher の責任が曖昧になる。

---

### 7. [重大] Fingerprint + similarity の既存システムとの相互作用が未考慮

**状況**: M13 で既に project similarity が実装されている（`project_similarity` テーブル）。計画では「isolation_mode でプロジェクト隔離」となっているが、similarity とのインタラクションが未定義。

**具体的な問題**:
- isolated = true のプロジェクトは、他プロジェクトとの similarity を計算するべきか？
- SessionStart で `GetSimilarProjects()` を呼ぶが、isolated プロジェクトの場合は空リストを返すのか？
- isolation_mode が変更された場合、既存の `project_similarity` レコードをどう処理するのか（削除か、放置か）？

**計画では言及なし**。

---

### 8. [重大] techScore / contentScore の加点基準が曖昧

**状況**: 計画では以下と記載:
- 技術キーワード（func, import, error:, github.com/ 等）→ techScore++
- ドメインキーワード（decided, requirement, 仕様 等）→ contentScore++
- kind=pattern/failure → techScore に加点
- kind=decision/constraint → contentScore に加点

**問題**:
1. **加点値が不明**: "techScore++" は +1 か？ +0.5 か？複数マッチしたら累積か？

2. **keyword list が不完全**: 
   - "func" は Go 言語固有。他言語（Python の `def`、JS の `function` など）をどう処理するのか？
   - "error:" は日本語ドメイン。英語の "error" との区別は？
   - "github.com/" は実装詳細（VCS 名）。他の VCS（gitlab.com など）は？

3. **既存キーワード体系との衝突**:
   - 現在の enricher.go には kindPatterns と globalWords / similarWords がある。
   - 新しいロジックは全く別の techScore / contentScore を導入するが、既存キーワードはどう取り扱うのか？
   - 例: "global" という単語は既存では scope = "global" を返すが、新ロジックではどう扱う（techScore か contentScore か）？

**計画では詳細なキーワードリストが欠落**。

---

### 9. [中大] threshold の設定値が恣意的で根拠がない

**計画では以下と記載**:
- contentScore >= 3: project
- techScore > contentScore && >= 3: global
- techScore > contentScore && >= 2: similarity_shareable
- default: project

**問題**:
1. **なぜこの値か？**: 
   - contentScore >= 3 なぜ 3 なのか。2 ではいけないのか？
   - "techScore >= 3" はなぜ必要か？既に "techScore > contentScore" で distinction しているのに。
   - similarity_shareable は "techScore >= 2" だが、global は "techScore >= 3"。なぜ 1 差なのか。

2. **スケール設定が未定義**:
   - techScore / contentScore の最大値は何か？（例えば、キーワード数が多いと無限に増え続けるのか？）
   - クランプが必要か？

3. **テスト戦略の欠落**:
   - 計画で「enricher_test.go, retrieval_test.go」の修正が言及されているが、具体的なテストケースが示されていない。
   - threshold のテストはどう設計する？（明白な失敗ケースを作れるか？）

---

### 10. [中大] マイグレーション M04 の詳細が不明

**状況**: 計画では「migrations/0004」と記載。

**欠落事項**:
- `isolation_mode` カラムをどのテーブルに追加するのか（projects テーブル確定か？）
- デフォルト値は NULL か FALSE か？
- 既存 projects レコードへの migration アルゴリズム。
- down migration（ロールバック）はどう実装するのか。

**現在の migration 構造** (CLAUDE.md 参照): `migrations/0001_initial.sql` と `migrations/0002_embedding_blob.sql` が存在。0004 はスキップして 0003 は何か？

---

### 11. [軽微] 計画ファイルに 11 ファイル修正と記載だが、列挙が不正確

**計画では「11ファイル」**:
```
migrations/0004, enricher.go, retrieval.go, boost.go, hook.go, project.go, 
checkpoint.go, session_end.go, enricher_test.go, retrieval_test.go, RETRIEVAL.ja.md
```

**数えてみると 11 ファイル目**までの責任が曖昧:
- `project.go`: 何のファイルか。`internal/project/project.go` か `internal/project/similarity.go` か？
- `checkpoint.go, session_end.go`: ingest worker のハンドラか。isolation_mode をどこで読むのか？

**影響**: 実装の「範囲」が不明確になり、漏れが生じやすい。

---

## 前提の脆弱性

1. **「コンテンツは閉じる、技術は開く」という設計原則は実装可能か未検証**
   - techScore と contentScore の分布が未知。実装後に「全てのチャンクが contentScore >= 3 になった」なら全て project スコープになり、全く効果がない。

2. **inferScope() のスコアリングは統計的に安定するか不明**
   - 現在の enricher.kindPatterns は substring match で「バグフィルタ」「決定」を判定している。
   - 新しいロジックでも同じ方法では、false positive / false negative が多すぎて、スコアが意味をなさない可能性。

3. **既存チャンク のスコープ再計算なしで新ロジックは機能するか不明**
   - 古いチャンクが全て project スコープなら、新ロジックは新規セッションでしか効果がない。
   - 1 年分の既存メモリが「全て project」なら、isolation_mode や similarity boost は無力。

4. **isolation_mode の「設定」と「有効化」のタイムラグ**
   - isolation_mode を true に設定した後、いつから有効になるのか。
   - その時点で既に project スコープで保存されたチャンクは何もしないのか。

---

## 見落とされたリスク

### R1: Retrieval Performance Regression

- 現在の UserPrompt は FTS + vector で候補 20 件 × 2 = 40 件を取得。
- 新しい ペナルティロジックは RRF 後の boost.go で適用。
- **ペナルティが弱すぎると、他プロジェクトのチャンク が上位 5 件に混入し続ける可能性**。
- 実装後、実ユーザーセッションで「何か関係ないチャンク が出始めた」という不満が出るリスク。

### R2: Migration / Rollback の複雑性

- isolation_mode を migration で追加した後、実際に使ってみたら「スコープ再計算が必要」と気付いた場合。
- rollback して data loss が発生する可能性。
- テスト DB での事前検証が必須だが、計画に明記されていない。

### R3: Enricher の複雑度爆発

- 現在の Enrich() は ~250 行で単純。
- techScore / contentScore 実装 + isolation_mode override + 詳細なキーワードリストを足すと、300+ 行にはすぐ達する。
- さらに kind / importance / summary への影響を考えると、相互作用による bug リスクが高い。
- **テストカバレッジが 100% 近くでも bug を逃す可能性**。

### R4: Hook Performance Impact

- SessionStart / UserPrompt hook は 2〜5 秒のタイムアウト制約がある。
- 新しいロジックで isolation_mode を判定するため、projects テーブルを参照する必要がある。
- FTS + vector + RRF + project boost に加えて isolation_mode チェックが入ると、retrieval 時間が伸びる可能性。
- **テスト環境では 3 秒で返るが、本番の大規模 DB では 10+ 秒かかる** リスク。

### R5: UX の混乱

- isolation_mode という「新しい概念」をユーザーに説明する必要がある。
- 「このプロジェクトは isolated」という状態は、チャンク の scope に反映されるのか、されないのか、ユーザーに分かるのか。
- 例: 「shared memory」画面で、isolation_mode = true のプロジェクトのチャンク は見えるのか、見えないのか。

---

## ⚠️ 最も危険な 1 点

**既存チャンク の scope 再分類メカニズムが計画に存在しない**。

### 理由

1. **計画全体の有効性の根本的脅威**
   - memoria は M15 完了時点で既に数百〜数千のチャンク を蓄積している可能性が高い。
   - 新しいロジックは新規チャンク にのみ作用するため、既存データは全く変わらない。
   - 結果、「コンテンツは閉じる、技術は開く」という設計原則は、古いデータには適用されず、新しいデータだけに適用される混在状態になる。

2. **テスト環境では見つからない bug**
   - 開発時は新規セッションから始めるため、新ロジックの効果を観測できる。
   - だが本番ユーザーは既存チャンク が混在した環境で使用。
   - その環境では「isolation_mode が効いていない」「similarity boost が弱い」と見える。

3. **修正時のデータ喪失リスク**
   - 後から「既存チャンク も再計算しよう」と決めた場合、バッチスクリプトで全チャンク の scope を上書きする必要がある。
   - その際に content_hash の重複排除などが干渉し、データ消失の可能性。
   - migration は一度実行されたら "done" と見なされ、rollback が難しい。

4. **ビジネス上の失敗につながる**
   - プロジェクト隔離機能を「新実装」としてユーザーに提供したが、既存セッションには効かない。
   - ユーザーは「なぜ古いメモリは isolation されないのか」と混乱。
   - 「期待値と実際の動作がズレている」という不信感が発生。

### 推奨対応

1. **計画の即座の修正**: 「既存チャンク の scope 再計算戦略」を明確に定義する。
   - オプション A: migration で一度だけ batch recompute する。
   - オプション B: background job で段階的に再計算する。
   - オプション C: 新規チャンク のみ対応し、既存は放置（ドキュメント で明記）。

2. **実装前のレビュー**: 選定した戦略について、データ喪失リスク・パフォーマンス・ロールバック可能性を詳細に検証する。

3. **テスト戦略の拡張**: 既存チャンク と新規チャンク の混在環境でのテスト。

---

## 小括

計画は「原則として良い」（コンテンツ隔離は妥当）だが、**実装の詳細が極めて不完全**。特に以下の 3 点が致命的:

1. inferScope() の techScore / contentScore ロジックが未定義
2. isolation_mode の設定 UI/UX が未定義  
3. 既存チャンク の再分類メカニズムが完全に欠落

これらを明確に定義してから実装に進むことを強く推奨します。

