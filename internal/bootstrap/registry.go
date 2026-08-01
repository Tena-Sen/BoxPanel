// Package bootstrap wires up the application: registers all protocol plugins,
// opens the store, builds the API server, and provides Run().
//
// 协议插件在此统一 blank-import，避免在 protocol 包内形成导入环。
package bootstrap

import (
	// blank-import protocols so their init() registers them
	_ "boxpanel/internal/protocol/hysteria2"
	_ "boxpanel/internal/protocol/shadowsocks"
	_ "boxpanel/internal/protocol/trojan"
	_ "boxpanel/internal/protocol/tuic"
	_ "boxpanel/internal/protocol/vless"
	_ "boxpanel/internal/protocol/vmess"
)

// BootstrapMarker is a sentinel so cmd/panel can force-import this package
// (which blank-imports all protocol plugins, registering them via init()).
var BootstrapMarker = struct{}{}
