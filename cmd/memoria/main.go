package main

import (
	"io"
	"os"

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

	ctx := kong.Parse(&c,
		kong.Name("memoria"),
		kong.Description("Claude Code 向けプロジェクト認識型ローカル RAG メモリシステム"),
		kong.UsageOnError(),
		kong.Bind(info),
		kong.Bind(&w),
		kong.Bind(cfg),
		kong.Bind(lazyDB),
	)

	// --config フラグが指定された場合は実際の設定ファイルをロードして cfg に反映する。
	if c.Globals.ConfigPath != "" {
		loaded, err := config.Load(c.Globals.ConfigPath)
		if err != nil {
			ctx.FatalIfErrorf(err)
		}
		*cfg = *loaded
	} else {
		// デフォルトパスから読み込む（ファイルが存在しない場合はデフォルト値を使用）
		loaded, err := config.Load(config.ConfigFile())
		if err != nil {
			ctx.FatalIfErrorf(err)
		}
		*cfg = *loaded
	}

	runErr := ctx.Run(&c.Globals)
	ctx.FatalIfErrorf(runErr)
}
