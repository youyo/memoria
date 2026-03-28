package config

import (
	"os"
	"path/filepath"
	"testing"
)

func home(t *testing.T) string {
	t.Helper()
	h, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	return h
}

func TestConfigDir(t *testing.T) {
	got := ConfigDir()
	want := filepath.Join(home(t), ".config", "memoria")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestDataDir(t *testing.T) {
	got := DataDir()
	want := filepath.Join(home(t), ".local", "share", "memoria")
	if got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestStateDir(t *testing.T) {
	got := StateDir()
	want := filepath.Join(home(t), ".local", "state", "memoria")
	if got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
}

func TestConfigFile(t *testing.T) {
	got := ConfigFile()
	want := filepath.Join(home(t), ".config", "memoria", "config.toml")
	if got != want {
		t.Errorf("ConfigFile() = %q, want %q", got, want)
	}
}

func TestDBFile(t *testing.T) {
	got := DBFile()
	want := filepath.Join(home(t), ".local", "share", "memoria", "memoria.db")
	if got != want {
		t.Errorf("DBFile() = %q, want %q", got, want)
	}
}

func TestRunDir(t *testing.T) {
	got := RunDir()
	want := filepath.Join(home(t), ".local", "state", "memoria", "run")
	if got != want {
		t.Errorf("RunDir() = %q, want %q", got, want)
	}
}

func TestLogDir(t *testing.T) {
	got := LogDir()
	want := filepath.Join(home(t), ".local", "state", "memoria", "logs")
	if got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}

func TestSocketPath(t *testing.T) {
	got := SocketPath()
	want := filepath.Join(home(t), ".local", "state", "memoria", "run", "embedding.sock")
	if got != want {
		t.Errorf("SocketPath() = %q, want %q", got, want)
	}
}
