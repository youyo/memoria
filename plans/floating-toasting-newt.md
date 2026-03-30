# PR #1 未対応コメント修正プラン

## Context

PR #1（`fix: restructure plugin/memoria to official Claude Code plugin format`）に対して youyo と Copilot から指摘があり、うち1件（ルート `skills/memoria/SKILL.md` の削除）は対応済みだが、残り3点が未対応。加えて、スキルファイルの配置が公式仕様と異なることが判明。この修正で PR を公式仕様準拠＆マージ可能な状態にする。

## 修正対象（4点）

### 1. スキルファイルを公式仕様の配置に移動

**問題:** 現在 `plugin/memoria/skills/memoria.md`（フラット）だが、公式仕様は `skills/<name>/SKILL.md`（ネスト構造）。
**修正:** `plugin/memoria/skills/memoria.md` → `plugin/memoria/skills/memoria/SKILL.md` に移動。

**操作:**
- `plugin/memoria/skills/memoria.md` を削除
- `plugin/memoria/skills/memoria/SKILL.md` を作成（同一内容）

### 2. `docs/specs/plugin/memoria/manifest.json` → 有効な JSON に変換

**問題:** プレーンテキストに書き換えられたが `.json` 拡張子のまま。JSON として無効。
**修正:** 有効な JSON 構造に書き換え、ディレクトリ構造も公式仕様に合わせる。

```json
{
  "note": "This file is a design reference only. The actual plugin uses the standard Claude Code plugin structure.",
  "structure": [
    "plugin/memoria/",
    ".claude-plugin/plugin.json          # Plugin metadata",
    "hooks/hooks.json                    # Hook definitions",
    "skills/memoria/SKILL.md             # Agent skill",
    "README.md                           # Plugin documentation"
  ]
}
```

**ファイル:** `docs/specs/plugin/memoria/manifest.json`

### 3. README のリンク切れ修正

**問題:** `skills/memoria/SKILL.md` は削除済みだが、README.md / README.ja.md がまだ参照している。
**修正:** パスを `plugin/memoria/skills/memoria/SKILL.md` に更新。

**ファイル:**
- `README.md`: `skills/memoria/SKILL.md` → `plugin/memoria/skills/memoria/SKILL.md`
- `README.ja.md`: `skills/memoria/SKILL.md` → `plugin/memoria/skills/memoria/SKILL.md`

### 4. `docs/specs/skills/memoria/SKILL.md` の重複解消

**問題:** `docs/specs/skills/memoria/SKILL.md` と `plugin/memoria/skills/memoria.md` が完全同一内容。
**修正:** `plugin/memoria/skills/memoria/SKILL.md` を正本とし、`docs/specs/skills/memoria/SKILL.md` を削除。README のリンクは修正3で plugin 側を指すようにしている。

**ファイル:** `docs/specs/skills/memoria/SKILL.md` を削除

## 作業手順

1. PR ブランチ `copilot/check-claudecode-plugin-installation` をチェックアウト
2. `plugin/memoria/skills/memoria.md` → `plugin/memoria/skills/memoria/SKILL.md` に移動
3. `docs/specs/plugin/memoria/manifest.json` を有効な JSON に書き換え
4. `README.md` / `README.ja.md` のスキルパス参照を更新
5. `docs/specs/skills/memoria/SKILL.md` を削除
6. コミット＆プッシュ

## 検証

- `python3 -m json.tool docs/specs/plugin/memoria/manifest.json` で JSON 有効性確認
- `grep -r 'skills/memoria/SKILL.md'` で旧パス参照が残っていないことを確認（plugin 配下のみ正当）
- `grep -r 'skills/memoria.md'` でフラット配置の参照が残っていないことを確認
- `docs/specs/skills/memoria/SKILL.md` が存在しないことを確認
- `plugin/memoria/skills/memoria/SKILL.md` が存在することを確認
