package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/youyo/memoria/internal/cli"
	"github.com/youyo/memoria/internal/config"
)

// ビルド時に -ldflags で埋め込む変数。
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var c cli.CLI

	info := &cli.VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	w := io.Writer(os.Stdout)

	// 一旦 --config フラグを事前パースしてパスを取得するため、
	// まずデフォルト設定でバインドし、パース後に実際の設定をロードする。
	// config パッケージの DI: *config.Config を Kong Bind で全コマンドに注入。
	cfg := config.DefaultConfig()

	// LazyDB: DB を必要とするコマンドの Run() が初回呼び出し時のみ db.Open() を実行する。
	// version / config init/show/path/print-hook 等は lazyDB.Get() を呼ばないため
	// DB ファイルが存在しない初回起動時でも正常動作する。
	lazyDB := cli.NewLazyDB(config.DBFile())

	parser, err := kong.New(&c,
		kong.Name("memoria"),
		kong.Description("Claude Code 向けプロジェクト認識型ローカル RAG メモリシステム"),
		kong.UsageOnError(),
		kong.Bind(info),
		kong.Bind(&w),
		kong.Bind(cfg),
		kong.Bind(lazyDB),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// --completion-bash フラグを Parse 前にインターセプト
	if handleCompletionBash(parser, os.Args[1:]) {
		return
	}

	ctx, parseErr := parser.Parse(os.Args[1:])
	if parseErr != nil {
		parser.FatalIfErrorf(parseErr)
	}

	// --config フラグが指定された場合は実際の設定ファイルをロードして cfg に反映する。
	if c.Globals.ConfigPath != "" {
		loaded, loadErr := config.Load(c.Globals.ConfigPath)
		if loadErr != nil {
			ctx.FatalIfErrorf(loadErr)
		}
		*cfg = *loaded
	} else {
		// デフォルトパスから読み込む（ファイルが存在しない場合はデフォルト値を使用）
		loaded, loadErr := config.Load(config.ConfigFile())
		if loadErr != nil {
			ctx.FatalIfErrorf(loadErr)
		}
		*cfg = *loaded
	}

	runErr := ctx.Run(&c.Globals)
	ctx.FatalIfErrorf(runErr)
}

// collectCompletions は --completion-bash 以降の部分入力を解析し、
// 補完候補のスライスを返す。--completion-bash がない場合は nil, false を返す。
func collectCompletions(k *kong.Kong, args []string) ([]string, bool) {
	idx := -1
	for i, a := range args {
		if a == "--completion-bash" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, false
	}

	partial := args[idx+1:]
	node := k.Model.Node
	usedFlags := make(map[string]bool)
	prefix := ""
	endsWithFlag := false

	loopPartial := partial
	if len(partial) > 0 {
		last := partial[len(partial)-1]
		if last != "" && strings.HasPrefix(last, "--") {
			prefix = last
			loopPartial = partial[:len(partial)-1]
		}
	}

	for _, word := range loopPartial {
		if word == "" {
			continue
		}
		if strings.HasPrefix(word, "--") {
			usedFlags[word] = true
			endsWithFlag = true
		} else {
			endsWithFlag = false
			found := false
			for _, child := range node.Children {
				if child.Name == word {
					node = child
					found = true
					break
				}
			}
			if !found {
				prefix = word
			}
		}
	}

	var completions []string

	// フラグ候補を収集（AllFlags(true) は hidden フラグを除外する）
	flagGroups := node.AllFlags(true)
	for _, group := range flagGroups {
		for _, flag := range group {
			if flag.Hidden {
				continue
			}
			candidate := "--" + flag.Name
			if usedFlags[candidate] {
				continue
			}
			if prefix != "" && !strings.HasPrefix(candidate, prefix) {
				continue
			}
			completions = append(completions, candidate)
		}
	}

	// サブコマンド候補を収集（フラグのプレフィクス入力中でなければ）
	if !endsWithFlag && !strings.HasPrefix(prefix, "--") {
		for _, child := range node.Children {
			if child.Hidden {
				continue
			}
			if prefix != "" && !strings.HasPrefix(child.Name, prefix) {
				continue
			}
			completions = append(completions, child.Name)
		}
	}

	return completions, true
}

// handleCompletionBash は --completion-bash フラグを処理する。
func handleCompletionBash(k *kong.Kong, args []string) bool {
	completions, ok := collectCompletions(k, args)
	if !ok {
		return false
	}
	for _, c := range completions {
		fmt.Println(c)
	}
	return true
}
