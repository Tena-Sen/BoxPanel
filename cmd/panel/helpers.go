package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// BootstrapMarker is exported so cmd/panel can force-import the bootstrap
// package, which blank-imports all protocol plugins (registering them).
var BootstrapMarker = bootstrapMarker{}

type bootstrapMarker struct{}

// syscallOS returns the current GOOS.
func syscallOS() string { return runtime.GOOS }

// execCommand runs a command silently (best-effort).
func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

// detectDefaultCore 探测 sing-box 版本并返回 (version, fullPath)。
// 扫描顺序：BASE_DIR/sing-box.exe → bin/sing-box.exe。
func detectDefaultCore() (string, string) {
	candidates := []string{
		filepath.Join(runtimeMainBaseDir(), "sing-box.exe"),
	}
	if exe, err := osExecutable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "sing-box.exe"))
	}
	for _, p := range candidates {
		v, err := probeSingBoxVersion(p)
		if err == nil && v != "" {
			return v, p
		}
	}
	return "", ""
}

func runtimeMainBaseDir() string {
	// main.go 里用的是 config.BaseDir()，但 helpers.go 不便 import config 引起循环
	// 用 cwd 兜底；正常 main 已设了 cwd
	wd, _ := exec.Command("cmd", "/c", "cd").Output()
	s := strings.TrimSpace(string(wd))
	if s != "" {
		return s
	}
	return "."
}

func osExecutable() (string, error) {
	// exec 自带查找自身路径
	exe, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	return exe, nil
}

var singBoxVersionRe = regexp.MustCompile(`sing-box version (\S+)`)

func probeSingBoxVersion(exePath string) (string, error) {
	cmd := exec.Command(exePath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	m := singBoxVersionRe.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return "", fmt.Errorf("no version in output")
	}
	return m[1], nil
}
