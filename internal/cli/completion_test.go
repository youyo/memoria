package cli_test

import (
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/cli"
)

func TestCompletionScript(t *testing.T) {
	t.Run("zsh completion が memoria を含む", func(t *testing.T) {
		output := cli.GenerateCompletion("memoria")
		if !strings.Contains(output, "memoria") {
			t.Errorf("zsh completion に 'memoria' が含まれていない: %s", output)
		}
	})

	t.Run("zsh completion が --completion-bash を含む", func(t *testing.T) {
		output := cli.GenerateCompletion("memoria")
		if !strings.Contains(output, "--completion-bash") {
			t.Errorf("zsh completion に '--completion-bash' が含まれていない: %s", output)
		}
	})

	t.Run("zsh completion が compdef を含む", func(t *testing.T) {
		output := cli.GenerateCompletion("memoria")
		if !strings.Contains(output, "compdef _memoria memoria") {
			t.Errorf("zsh completion に 'compdef _memoria memoria' が含まれていない: %s", output)
		}
	})

	t.Run("zsh completion の words 展開に引用符が付いていない", func(t *testing.T) {
		output := cli.GenerateCompletion("memoria")
		if strings.Contains(output, `"${words[@]:1}"`) {
			t.Errorf("引用符付き ${words[@]:1} が含まれている: %s", output)
		}
		if !strings.Contains(output, "${words[@]:1}") {
			t.Errorf("${words[@]:1} が含まれていない: %s", output)
		}
	})
}
