package main

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/youyo/memoria/internal/cli"
	"github.com/youyo/memoria/internal/config"
)

func newTestParser(t *testing.T) *kong.Kong {
	t.Helper()
	var c cli.CLI

	info := &cli.VersionInfo{Version: "test", Commit: "test", Date: "test"}
	var buf strings.Builder
	iow := io.Writer(&buf)
	cfg := config.DefaultConfig()
	lazyDB := cli.NewLazyDB("")

	parser, err := kong.New(&c,
		kong.Name("memoria"),
		kong.Bind(info),
		kong.Bind(&iow),
		kong.Bind(cfg),
		kong.Bind(lazyDB),
		kong.Exit(func(code int) {}),
	)
	if err != nil {
		t.Fatalf("kong.New failed: %v", err)
	}
	return parser
}

func TestCollectCompletions(t *testing.T) {
	t.Run("トップレベルでサブコマンド候補が返る", func(t *testing.T) {
		parser := newTestParser(t)
		got, ok := collectCompletions(parser, []string{"--completion-bash", ""})
		if !ok {
			t.Fatal("true が返されるべき")
		}
		for _, cmd := range []string{"hook", "worker", "memory", "config", "completion", "doctor", "version"} {
			if !slices.Contains(got, cmd) {
				t.Errorf("%s が含まれていない: %v", cmd, got)
			}
		}
	})

	t.Run("memory 配下でサブコマンドが返る", func(t *testing.T) {
		parser := newTestParser(t)
		got, ok := collectCompletions(parser, []string{"--completion-bash", "memory"})
		if !ok {
			t.Fatal("true が返されるべき")
		}
		for _, cmd := range []string{"search", "get", "list", "stats", "reindex"} {
			if !slices.Contains(got, cmd) {
				t.Errorf("%s が含まれていない: %v", cmd, got)
			}
		}
	})

	t.Run("リーフノードでフラグが返る", func(t *testing.T) {
		parser := newTestParser(t)
		got, ok := collectCompletions(parser, []string{"--completion-bash", "memory", "search", "--"})
		if !ok {
			t.Fatal("true が返されるべき")
		}
		if !slices.Contains(got, "--format") {
			t.Errorf("--format が含まれていない: %v", got)
		}
	})

	t.Run("プレフィクスマッチ", func(t *testing.T) {
		parser := newTestParser(t)
		got, ok := collectCompletions(parser, []string{"--completion-bash", "--f"})
		if !ok {
			t.Fatal("true が返されるべき")
		}
		if !slices.Contains(got, "--format") {
			t.Errorf("--format が含まれていない: %v", got)
		}
	})

	t.Run("hidden コマンド（daemon）が含まれない", func(t *testing.T) {
		parser := newTestParser(t)
		got, ok := collectCompletions(parser, []string{"--completion-bash", ""})
		if !ok {
			t.Fatal("true が返されるべき")
		}
		if slices.Contains(got, "daemon") {
			t.Errorf("hidden コマンド daemon が含まれている: %v", got)
		}
	})

	t.Run("--completion-bash なしで false", func(t *testing.T) {
		parser := newTestParser(t)
		got, ok := collectCompletions(parser, []string{"memory", "search"})
		if ok {
			t.Error("false が返されるべき")
		}
		if got != nil {
			t.Errorf("nil が返されるべき: %v", got)
		}
	})
}
