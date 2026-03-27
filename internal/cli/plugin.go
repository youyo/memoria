package cli

import (
	"fmt"
	"io"
)

// PluginCmd はプラグイン管理サブコマンドグループを定義する。
type PluginCmd struct {
	List   PluginListCmd   `cmd:"" help:"プラグイン一覧を表示する"`
	Doctor PluginDoctorCmd `cmd:"" help:"プラグインの診断を実行する"`
}

// PluginListCmd は plugin list コマンド。
type PluginListCmd struct{}

// Run は plugin list を実行する。
func (c *PluginListCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// PluginDoctorCmd は plugin doctor コマンド。
type PluginDoctorCmd struct{}

// Run は plugin doctor を実行する。
func (c *PluginDoctorCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
