package config

import (
	"os"
	"path/filepath"
)

// homeDir はユーザーのホームディレクトリを返す。
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic("cannot determine home directory: " + err.Error())
	}
	return h
}

// ConfigDir は設定ディレクトリ (~/.config/memoria/) を返す。
func ConfigDir() string {
	return filepath.Join(homeDir(), ".config", "memoria")
}

// DataDir はデータディレクトリ (~/.local/share/memoria/) を返す。
func DataDir() string {
	return filepath.Join(homeDir(), ".local", "share", "memoria")
}

// StateDir は状態ディレクトリ (~/.local/state/memoria/) を返す。
func StateDir() string {
	return filepath.Join(homeDir(), ".local", "state", "memoria")
}

// ConfigFile は設定ファイルパス (~/.config/memoria/config.toml) を返す。
func ConfigFile() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

// DBFile は SQLite データベースファイルパス (~/.local/share/memoria/memoria.db) を返す。
func DBFile() string {
	return filepath.Join(DataDir(), "memoria.db")
}

// RunDir は実行時ファイルディレクトリ (~/.local/state/memoria/run/) を返す。
func RunDir() string {
	return filepath.Join(StateDir(), "run")
}

// LogDir はログディレクトリ (~/.local/state/memoria/logs/) を返す。
func LogDir() string {
	return filepath.Join(StateDir(), "logs")
}

// SocketPath は embedding worker の Unix Domain Socket パスを返す。
func SocketPath() string {
	return filepath.Join(RunDir(), "embedding.sock")
}
