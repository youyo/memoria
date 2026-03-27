package cli

import (
	"fmt"
	"io"
)

// CompletionCmd はシェル補完スクリプト生成サブコマンドグループを定義する。
type CompletionCmd struct {
	Bash CompletionBashCmd `cmd:"" help:"Bash 補完スクリプトを生成する"`
	Zsh  CompletionZshCmd  `cmd:"" help:"Zsh 補完スクリプトを生成する"`
	Fish CompletionFishCmd `cmd:"" help:"Fish 補完スクリプトを生成する"`
}

// CompletionBashCmd は completion bash コマンド。
type CompletionBashCmd struct{}

// Run は completion bash を実行する。
func (c *CompletionBashCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// CompletionZshCmd は completion zsh コマンド。
type CompletionZshCmd struct {
	Short bool `help:"短縮形式で出力する"`
}

// Run は completion zsh を実行する。
func (c *CompletionZshCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// CompletionFishCmd は completion fish コマンド。
type CompletionFishCmd struct{}

// Run は completion fish を実行する。
func (c *CompletionFishCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
