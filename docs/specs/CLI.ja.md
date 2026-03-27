# memoria CLI 詳細設計

## 目的

memoria CLI は、Claude Code plugin から呼ばれる hook 契約面と、人間およびエージェントが直接操作する運用面の両方を提供する。

実処理はすべて `memoria` バイナリが担当し、Claude Code plugin は hook と skill の配布レイヤーとして扱う。

## 実装方針

- 言語: Go
- CLI framework: Kong
- completion 対応
- JSON 出力優先
- `--format text` で人間向け表示
- `--config`, `--verbose`, `--no-color`, `--format` のグローバルフラグを持つ

## トップレベルコマンド

```text
memoria hook ...
memoria worker ...
memoria memory ...
memoria config ...
memoria completion ...
memoria plugin ...
memoria doctor
memoria version
```

## hook サブコマンド

```text
memoria hook session-start
memoria hook user-prompt
memoria hook stop
memoria hook session-end
```

### 設計原則

- Claude Code plugin からのみ呼ばれることを前提とする
- stdin から JSON を受ける
- stdout は必要時のみ JSON を返す
- 基本的に exit code 0
- block 用途では使わない

## worker サブコマンド

```text
memoria worker start
memoria worker stop
memoria worker restart
memoria worker status
```

### 役割

- ingest worker と embedding worker の共有デーモン管理
- 多重起動防止
- 生存確認
- 強制終了時の cleanup

## memory サブコマンド

```text
memoria memory search <query>
memoria memory get <id>
memoria memory list
memoria memory stats
memoria memory reindex
```

### MVP で最低限必要なもの

- `search`
- `stats`

## config サブコマンド

```text
memoria config init
memoria config show
memoria config path
memoria config print-hook
```

### `print-hook` の役割

Claude Code の設定に貼り付ける hooks 設定断片を出力する。

## completion サブコマンド

```text
memoria completion bash
memoria completion zsh
memoria completion zsh --short
memoria completion fish
```

## doctor サブコマンド

確認項目:

- DB path
- schema version
- ingest worker 状態
- embedding worker 状態
- queue depth
- UDS 接続性
- hook 設定例
- skill / plugin 配置状況

## plugin サブコマンド

```text
memoria plugin list
memoria plugin doctor
```

これは将来の CLI 内部拡張向けであり、Claude Code plugin 自体とは責務を分ける。
