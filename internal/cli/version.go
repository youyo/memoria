package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// VersionInfo はバージョン情報を保持する。ビルド時に -ldflags で埋め込む値を受け取る。
type VersionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// VersionCmd は version コマンドを定義する。
type VersionCmd struct{}

// Run は version コマンドを実行する。
func (v *VersionCmd) Run(globals *Globals, info *VersionInfo, w *io.Writer) error {
	if globals.Format == "json" {
		enc := json.NewEncoder(*w)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	_, err := fmt.Fprintf(*w, "memoria v%s (commit: %s, built: %s)\n", info.Version, info.Commit, info.Date)
	return err
}
