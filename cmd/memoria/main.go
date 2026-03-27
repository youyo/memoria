package main

import (
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/youyo/memoria/internal/cli"
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

	ctx := kong.Parse(&c,
		kong.Name("memoria"),
		kong.Description("Claude Code 向けプロジェクト認識型ローカル RAG メモリシステム"),
		kong.UsageOnError(),
		kong.Bind(info),
		kong.Bind(&w),
	)

	err := ctx.Run(&c.Globals)
	ctx.FatalIfErrorf(err)
}
