package cli

import (
	"fmt"
	"io"
)

// MemoryCmd はメモリ操作サブコマンドグループを定義する。
type MemoryCmd struct {
	Search  MemorySearchCmd  `cmd:"" help:"メモリを検索する"`
	Get     MemoryGetCmd     `cmd:"" help:"メモリを ID で取得する"`
	List    MemoryListCmd    `cmd:"" help:"メモリ一覧を表示する"`
	Stats   MemoryStatsCmd   `cmd:"" help:"メモリ統計を表示する"`
	Reindex MemoryReindexCmd `cmd:"" help:"メモリのインデックスを再構築する"`
}

// MemorySearchCmd は memory search コマンド。
type MemorySearchCmd struct {
	Query string `arg:"" help:"検索クエリ"`
}

// Run は memory search を実行する。
func (c *MemorySearchCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryGetCmd は memory get コマンド。
type MemoryGetCmd struct {
	ID string `arg:"" help:"メモリ ID"`
}

// Run は memory get を実行する。
func (c *MemoryGetCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryListCmd は memory list コマンド。
type MemoryListCmd struct{}

// Run は memory list を実行する。
func (c *MemoryListCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryStatsCmd は memory stats コマンド。
type MemoryStatsCmd struct{}

// Run は memory stats を実行する。
func (c *MemoryStatsCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}

// MemoryReindexCmd は memory reindex コマンド。
type MemoryReindexCmd struct{}

// Run は memory reindex を実行する。
func (c *MemoryReindexCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
