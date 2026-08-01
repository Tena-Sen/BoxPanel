//go:build !windows

package sysproxy

import (
	"os/exec"
	"runtime"
	"strings"
)

// UnixController implements Controller for macOS/Linux (best-effort).
type UnixController struct{}

// New returns the platform controller (macOS via networksetup, Linux via gsettings).
func New() Controller { return &UnixController{} }

func (UnixController) Get() State {
	if runtime.GOOS == "darwin" {
		return getMacOS()
	}
	return getLinux()
}

func (UnixController) Enable(server, bypass string) (State, error) {
	if runtime.GOOS == "darwin" {
		return enableMacOS(server, bypass)
	}
	return enableLinux(server, bypass)
}

func (UnixController) Disable() (State, error) {
	if runtime.GOOS == "darwin" {
		return disableMacOS()
	}
	return disableLinux()
}

// ----- macOS via networksetup -----

func getMacOS() State {
	st := State{Supported: true}
	out, err := exec.Command("networksetup", "-getwebproxy", "Wi-Fi").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "Enabled:") {
				st.Enabled = strings.Contains(l, "Yes")
			}
			if strings.HasPrefix(l, "Server:") {
				parts := strings.SplitN(l, ":", 2)
				if len(parts) == 2 {
					st.Server = strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return st
}

func enableMacOS(server, bypass string) (State, error) {
	host, port := splitHostPort(server, "8080")
	for _, svc := range []string{"Wi-Fi"} {
		_ = exec.Command("networksetup", "-setwebproxy", svc, host, port).Run()
		_ = exec.Command("networksetup", "-setsecurewebproxy", svc, host, port).Run()
		if bypass != "" {
			_ = exec.Command("networksetup", "-setproxybypassdomains", svc, strings.Split(bypass, ";")...).Run()
		}
	}
	return getMacOS(), nil
}

func disableMacOS() (State, error) {
	for _, svc := range []string{"Wi-Fi"} {
		_ = exec.Command("networksetup", "-setwebproxystate", svc, "off").Run()
		_ = exec.Command("networksetup", "-setsecurewebproxystate", svc, "off").Run()
	}
	return getMacOS(), nil
}

// ----- Linux via gsettings (GNOME) -----

func getLinux() State {
	st := State{Supported: true}
	out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
	if err == nil {
		st.Enabled = strings.TrimSpace(string(out)) == "'manual'"
	}
	if st.Enabled {
		h, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "host").Output()
		p, _ := exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "port").Output()
		host := strings.Trim(strings.TrimSpace(string(h)), "'")
		port := strings.TrimSpace(string(p))
		if host != "" && port != "" {
			st.Server = host + ":" + port
		}
	}
	return st
}

func enableLinux(server, bypass string) (State, error) {
	host, port := splitHostPort(server, "8080")
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", host).Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", port).Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "host", host).Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "port", port).Run()
	return getLinux(), nil
}

func disableLinux() (State, error) {
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
	return getLinux(), nil
}

func splitHostPort(s, defPort string) (string, string) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return s, defPort
	}
	return s[:idx], s[idx+1:]
}
