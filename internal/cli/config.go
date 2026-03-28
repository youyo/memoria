package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
	cfg_pkg "github.com/youyo/memoria/internal/config"
)

// ConfigCmd は設定管理サブコマンドグループを定義する。
type ConfigCmd struct {
	Init      ConfigInitCmd      `cmd:"" help:"設定ファイルを初期化する"`
	Show      ConfigShowCmd      `cmd:"" help:"現在の設定を表示する"`
	Path      ConfigPathCmd      `cmd:"" help:"設定ファイルのパスを表示する"`
	PrintHook ConfigPrintHookCmd `cmd:"" name:"print-hook" help:"Claude Code hook 設定を出力する"`
}

// resolveConfigPath は globals の ConfigPath を参照し、
// 未指定の場合はデフォルトの XDG パスを返す。
func resolveConfigPath(globals *Globals) string {
	if globals.ConfigPath != "" {
		return globals.ConfigPath
	}
	return cfg_pkg.ConfigFile()
}

// ConfigInitCmd は config init コマンド。
type ConfigInitCmd struct {
	Force bool `help:"既存の設定ファイルを上書きする" short:"f"`
}

// Run は config init を実行する。
func (c *ConfigInitCmd) Run(globals *Globals, w *io.Writer) error {
	path := resolveConfigPath(globals)

	if !c.Force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("設定ファイルが既に存在します: %s (上書きするには --force を使用してください)", path)
		}
	}

	def := cfg_pkg.DefaultConfig()
	if err := cfg_pkg.Save(path, def); err != nil {
		return fmt.Errorf("設定ファイルの作成に失敗しました: %w", err)
	}

	fmt.Fprintf(*w, "設定ファイルを作成しました: %s\n", path)
	return nil
}

// ConfigShowCmd は config show コマンド。
type ConfigShowCmd struct{}

// Run は config show を実行する。
func (c *ConfigShowCmd) Run(globals *Globals, w *io.Writer, cfg *cfg_pkg.Config) error {
	path := resolveConfigPath(globals)

	// パスが指定されている場合はそのファイルをロードし直す
	if globals.ConfigPath != "" {
		loaded, err := cfg_pkg.Load(path)
		if err != nil {
			return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
		}
		cfg = loaded
	}

	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	default:
		return toml.NewEncoder(*w).Encode(cfg)
	}
}

// ConfigPathCmd は config path コマンド。
type ConfigPathCmd struct{}

// Run は config path を実行する。
func (c *ConfigPathCmd) Run(globals *Globals, w *io.Writer) error {
	path := resolveConfigPath(globals)

	switch globals.Format {
	case "json":
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{"path": path})
	default:
		fmt.Fprintln(*w, path)
	}
	return nil
}

// ConfigPrintHookCmd は config print-hook コマンド。
type ConfigPrintHookCmd struct{}

// Run は config print-hook を実行する。
func (c *ConfigPrintHookCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
