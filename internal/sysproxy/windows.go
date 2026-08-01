//go:build windows

package sysproxy

import (
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const regPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// WindowsController implements Controller via the Windows registry.
type WindowsController struct{}

// New returns the Windows controller.
func New() Controller { return &WindowsController{} }

func (WindowsController) Get() State {
	st := State{Supported: true}
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		return st
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		st.Enabled = v != 0
	}
	if v, _, err := k.GetStringValue("ProxyServer"); err == nil {
		st.Server = v
	}
	if v, _, err := k.GetStringValue("ProxyOverride"); err == nil {
		st.Bypass = v
	}
	return st
}

func (WindowsController) Enable(server, bypass string) (State, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return State{}, err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return State{}, err
	}
	if err := k.SetStringValue("ProxyServer", server); err != nil {
		return State{}, err
	}
	if bypass != "" {
		_ = k.SetStringValue("ProxyOverride", bypass)
	}
	notifySettingsChanged()
	return WindowsController{}.Get(), nil
}

func (WindowsController) Disable() (State, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return State{}, err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return State{}, err
	}
	notifySettingsChanged()
	return WindowsController{}.Get(), nil
}

// notifySettingsChanged tells applications to refresh their proxy settings.
func notifySettingsChanged() {
	wininet := syscall.NewLazyDLL("wininet.dll")
	proc := wininet.NewProc("InternetSetOptionW")
	const (
		INTERNET_OPTION_SETTINGS_CHANGED = 39
		INTERNET_OPTION_REFRESH          = 37
	)
	_, _, _ = proc.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	_, _, _ = proc.Call(0, INTERNET_OPTION_REFRESH, 0, 0)
}

