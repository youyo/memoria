package cli

import (
	"fmt"
	"io"
)

// WorkerCmd は worker 管理サブコマンドグループを定義する。
type WorkerCmd struct {
	Start   WorkerStartCmd   `cmd:"" help:"Worker を起動する"`
	Stop    WorkerStopCmd    `cmd:"" help:"Worker を停止する"`
	Restart WorkerRestartCmd `cmd:"" help:"Worker を再起動する"`
	Status  WorkerStatusCmd  `cmd:"" help:"Worker の状態を表示する"`
}

// WorkerStartCmd は worker start コマンド。
type WorkerStartCmd struct{}

// Run は worker start を実行する。
func (c *WorkerStartCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// WorkerStopCmd は worker stop コマンド。
type WorkerStopCmd struct{}

// Run は worker stop を実行する。
func (c *WorkerStopCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// WorkerRestartCmd は worker restart コマンド。
type WorkerRestartCmd struct{}

// Run は worker restart を実行する。
func (c *WorkerRestartCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// WorkerStatusCmd は worker status コマンド。
type WorkerStatusCmd struct{}

// Run は worker status を実行する。
func (c *WorkerStatusCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
