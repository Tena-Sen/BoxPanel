// Package sysproxy controls the OS-level system proxy.
//
// 接口跨平台；分平台实现按 build tag 选择：windows/macos/linux。
package sysproxy

// State describes the current system proxy configuration.
type State struct {
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
	Server    string `json:"server"`
	Bypass    string `json:"bypass"`
}

// Controller is the cross-platform system proxy interface.
type Controller interface {
	Get() State
	Enable(server, bypass string) (State, error)
	Disable() (State, error)
}
