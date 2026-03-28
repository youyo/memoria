package cli

// Globals はすべてのコマンドで共有されるグローバルフラグを定義する。
type Globals struct {
	ConfigPath string `help:"設定ファイルパス" type:"path" env:"MEMORIA_CONFIG" name:"config"`
	Verbose    bool   `help:"詳細出力を有効にする" short:"v"`
	NoColor    bool   `help:"カラー出力を無効にする" name:"no-color"`
	Format     string `help:"出力フォーマット (text, json)" default:"text" enum:"text,json"`
}

// CLI はメモリア CLI のルート構造体。Kong が struct tag からコマンドツリーを構築する。
type CLI struct {
	Globals

	Hook       HookCmd       `cmd:"" help:"Claude Code hook コマンド"`
	Worker     WorkerCmd     `cmd:"" help:"Worker 管理コマンド"`
	Memory     MemoryCmd     `cmd:"" help:"メモリ操作コマンド"`
	Config     ConfigCmd     `cmd:"" name:"config" help:"設定管理コマンド"`
	Completion CompletionCmd `cmd:"" help:"シェル補完スクリプト生成"`
	Plugin     PluginCmd     `cmd:"" help:"プラグイン管理コマンド"`
	Doctor     DoctorCmd     `cmd:"" help:"システム診断"`
	Version    VersionCmd    `cmd:"" help:"バージョン情報を表示"`
	Daemon     DaemonCmd     `cmd:"" help:"内部デーモンコマンド" hidden:""`
}
