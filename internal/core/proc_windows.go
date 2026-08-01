//go:build windows

package core

import "syscall"

// hideWindowAttr returns SysProcAttr that hides the child console window.
func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
