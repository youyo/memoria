package cli

import (
	"fmt"
	"io"
)

// completionScript は zsh 用の補完スクリプトを生成する。
func completionScript(name string) string {
	return fmt.Sprintf(`# %s completion for zsh
# To enable: eval "$(%s completion zsh)"
_%s() {
  local -a completions
  completions=($(${words[1]} --completion-bash ${words[@]:1}))
  compadd -- $completions
}
compdef _%s %s
`, name, name, name, name, name)
}

// GenerateCompletion はテスト・実装の両方から利用できる公開ヘルパー。
func GenerateCompletion(name string) string {
	return completionScript(name)
}

// CompletionCmd はシェル補完スクリプト生成サブコマンドグループを定義する。
type CompletionCmd struct {
	Zsh CompletionZshCmd `cmd:"" help:"Zsh 補完スクリプトを生成する"`
}

// CompletionZshCmd は completion zsh コマンド。
type CompletionZshCmd struct{}

// Run は completion zsh を実行する。
func (c *CompletionZshCmd) Run(globals *Globals, w *io.Writer) error {
	_, err := fmt.Fprint(*w, completionScript("memoria"))
	return err
}
