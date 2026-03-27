package cli

import (
	"fmt"
	"io"
)

// HookCmd は Claude Code hook サブコマンドグループを定義する。
type HookCmd struct {
	SessionStart HookSessionStartCmd `cmd:"" name:"session-start" help:"セッション開始時の hook"`
	UserPrompt   HookUserPromptCmd   `cmd:"" name:"user-prompt" help:"ユーザープロンプト送信時の hook"`
	Stop         HookStopCmd         `cmd:"" help:"レスポンス完了時の hook"`
	SessionEnd   HookSessionEndCmd   `cmd:"" name:"session-end" help:"セッション終了時の hook"`
}

// HookSessionStartCmd は session-start hook コマンド。
type HookSessionStartCmd struct{}

// Run は session-start hook を実行する。
func (c *HookSessionStartCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// HookUserPromptCmd は user-prompt hook コマンド。
type HookUserPromptCmd struct{}

// Run は user-prompt hook を実行する。
func (c *HookUserPromptCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// HookStopCmd は stop hook コマンド。
type HookStopCmd struct{}

// Run は stop hook を実行する。
func (c *HookStopCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// HookSessionEndCmd は session-end hook コマンド。
type HookSessionEndCmd struct{}

// Run は session-end hook を実行する。
func (c *HookSessionEndCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
