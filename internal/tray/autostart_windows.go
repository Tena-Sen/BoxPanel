//go:build windows

package tray

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const regRunPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// SetAutostartWindows 写/删注册表 Run key。
func setAutostart(appName, exePath string, enable bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, regRunPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("CreateKey: %w", err)
	}
	defer k.Close()

	if enable {
		// 加引号包裹路径以防空格
		value := fmt.Sprintf(`"%s"`, exePath)
		return k.SetStringValue(appName, value)
	}
	_ = k.DeleteValue(appName)
	return nil
}

// AutostartEnabledWindows 检查注册表。
func autostartEnabled(appName, _ string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regRunPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(appName)
	return err == nil
}