// Package config holds application-wide constants, path resolution and defaults.
//
// 路径自动定位 sing-box 所在目录（与旧 Python 版一致），
// 数据目录用 <BaseDir>/data（portable 模式，所有数据随项目走）。
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppName is the human-readable application name.
const AppName = "BoxPanel"

// Version is the application version.
const Version = "1.0.0"

// Default HTTP listen host/port.
const (
	Host         = "127.0.0.1"
	DefaultPort  = 7820
	PortRange    = 20 // 端口被占用时向上尝试的次数
)

// Clash API defaults - injected into every generated sing-box config.
const (
	ClashAPIHost     = "127.0.0.1"
	ClashAPIPort     = 9090
	ClashAPISecret   = "" // 启动时随机生成并写入配置
	MixedInboundPort = 20808
)

// SingBoxExeName per platform.
func SingBoxExeName() string {
	if runtime.GOOS == "windows" {
		return "sing-box.exe"
	}
	return "sing-box"
}

// BaseDir locates the directory containing sing-box executable.
// 查找顺序：可执行文件所在目录 -> cwd -> 向上逐层（最多 5 层）。
func BaseDir() string {
	if exe, err := os.Executable(); err == nil {
		if dir := findUp(exe, 0); dir != "" {
			return dir
		}
	}
	if dir := findUp(filepath.Join(getCWD(), SingBoxExeName()), 0); dir != "" {
		return dir
	}
	return getCWD()
}

func getCWD() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// findUp walks up from the file's directory looking for sing-box exe.
func findUp(path string, depth int) string {
	if depth > 5 {
		return ""
	}
	dir := filepath.Dir(path)
	if dir == path || dir == "." || dir == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, SingBoxExeName())); err == nil {
		return dir
	}
	return findUp(dir, depth+1)
}

// ExePath returns the full path to sing-box executable.
func ExePath() string {
	return filepath.Join(BaseDir(), SingBoxExeName())
}

// DataDir returns the persistent data directory (SQLite db, generated configs, downloaded cores).
// Always uses <BaseDir>/data/ so all data stays with the project (portable).
func DataDir() string {
	d := filepath.Join(BaseDir(), "data")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// DBPath returns the SQLite database file path.
func DBPath() string { return filepath.Join(DataDir(), "boxpanel.db") }

// MigrateLegacyDB renames the old sbpanel.db to boxpanel.db if the new name
// does not exist yet. This is a one-time migration on first run after rename.
func MigrateLegacyDB() {
	newPath := DBPath()
	oldPath := filepath.Join(DataDir(), "sbpanel.db")
	if _, err := os.Stat(newPath); err == nil {
		return // new db already exists
	}
	if _, err := os.Stat(oldPath); err != nil {
		return // old db not found, nothing to migrate
	}
	_ = os.Rename(oldPath, newPath)
	// Also rename WAL/SHM if they exist
	for _, suffix := range []string{"-shm", "-wal"} {
		_ = os.Rename(oldPath+suffix, newPath+suffix)
	}
}

// GeneratedConfigPath returns the path for the active generated sing-box config.
func GeneratedConfigPath() string { return filepath.Join(DataDir(), "config.runtime.json") }

// RuleSetDir returns the directory for .srs rule-set files.
func RuleSetDir() string {
	d := filepath.Join(BaseDir())
	return d
}
