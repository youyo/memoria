package cli

import (
	"fmt"
	"io"
)

// ConfigCmd は設定管理サブコマンドグループを定義する。
type ConfigCmd struct {
	Init      ConfigInitCmd      `cmd:"" help:"設定ファイルを初期化する"`
	Show      ConfigShowCmd      `cmd:"" help:"現在の設定を表示する"`
	Path      ConfigPathCmd      `cmd:"" help:"設定ファイルのパスを表示する"`
	PrintHook ConfigPrintHookCmd `cmd:"" name:"print-hook" help:"Claude Code hook 設定を出力する"`
}

// ConfigInitCmd は config init コマンド。
type ConfigInitCmd struct{}

// Run は config init を実行する。
func (c *ConfigInitCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// ConfigShowCmd は config show コマンド。
type ConfigShowCmd struct{}

// Run は config show を実行する。
func (c *ConfigShowCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// ConfigPathCmd は config path コマンド。
type ConfigPathCmd struct{}

// Run は config path を実行する。
func (c *ConfigPathCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// ConfigPrintHookCmd は config print-hook コマンド。
type ConfigPrintHookCmd struct{}

// Run は config print-hook を実行する。
func (c *ConfigPrintHookCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
