//go:build !windows

package core

import "syscall"

// hideWindowAttr returns a no-op SysProcAttr on non-Windows.
func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
