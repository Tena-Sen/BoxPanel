// Package coreinfo defines metadata descriptors for each supported proxy core.
//
// Inspired by v2rayN's CoreInfo model: each core kind is described by a data-driven
// record (CoreInfo) that captures startup args, download URL templates, version
// detection regexes, candidate executable names, and protocol/transport compatibility
// constraints — replacing scattered hardcoded if/else throughout the codebase.
//
// Benefits:
//   - Adding a new core kind = append one CoreInfo struct (zero code changes)
//   - Startup arg / download URL / exe name changes are data, not logic
//   - NodeValidator can query CoreInfo for per-kind transport/protocol restrictions
//   - CoreExes lookup provides ordered candidate names (handles different packaging)
package coreinfo

import (
	"regexp"

	"boxpanel/internal/models"
)

// CoreInfo describes a proxy core kind with all its metadata.
type CoreInfo struct {
	// Kind is the unique identifier (matches models.CoreKind*).
	Kind string `json:"kind"`

	// Name is the human-readable name ("sing-box", "Xray", "mihomo", "Hysteria2").
	Name string `json:"name"`

	// CandidateExes lists possible executable filenames in priority order.
	// The first existing file wins. Covers different packaging conventions:
	//   sing-box: ["sing-box.exe", "sing-box"] (Windows vs Linux)
	//   Xray: ["xray.exe", "Xray.exe", "xray", "Xray"]
	CandidateExes []string `json:"candidate_exes"`

	// StartArgs is the template for launching the core.
	// Placeholders: {{ConfigPath}}, {{ExePath}}, {{BaseDir}}
	StartArgs []string `json:"start_args"`

	// CheckArgs is the template for verifying config syntax.
	CheckArgs []string `json:"check_args"`

	// VersionArgs is the args to probe the core version string.
	VersionArgs []string `json:"version_args"`

	// VersionRe is the regex to extract version from `VersionArgs` output.
	// Must have at least one capture group for the version string.
	VersionRe *regexp.Regexp `json:"-"` // not JSON-serializable, set in code

	// VersionRePattern is the source pattern (for JSON serialization).
	VersionRePattern string `json:"version_re_pattern"`

	// DownloadURLTemplate is the Go template for download URLs.
	// Placeholders: {{.Version}}, {{.OS}}, {{.Arch}}, {{.Ext}}
	// Example: "https://github.com/SagerNet/sing-box/releases/download/v{{.Version}}/sing-box-{{.Version}}-{{.OS}}-{{.Arch}}.{{.Ext}}"
	DownloadURLTemplate string `json:"download_url_template"`

	// GitHubRepo is the "owner/repo" for the GitHub release API.
	GitHubRepo string `json:"github_repo"`

	// ConfigFormat is the config file format: "json" or "yaml".
	ConfigFormat string `json:"config_format"`

	// HasClashAPI indicates whether this core exposes a Clash-compatible API.
	HasClashAPI bool `json:"has_clash_api"`

	// SupportedProtocols lists protocols this core can handle.
	SupportedProtocols []string `json:"supported_protocols"`

	// UnsupportedTransports lists transport types this core CANNOT handle.
	// For example, sing-box does not support "kcp".
	// NodeValidator uses this to reject incompatible servers before config gen.
	UnsupportedTransports []string `json:"unsupported_transports"`

	// TransportOverrides maps a transport type name to the core-specific name.
	// Example for sing-box >= 1.11: "xhttp" -> "splithttp"
	// Applied by the config adapter, not here.
	TransportOverrides map[string]string `json:"transport_overrides,omitempty"`

	// SSOnlyTransports lists the ONLY transports sing-box supports for
	// Shadowsocks. If a Shadowsocks server uses a transport NOT in this list,
	// sing-box cannot handle it (other cores may).
	SSOnlyTransports []string `json:"ss_only_transports,omitempty"`
}

// Registry holds all registered CoreInfo records.
var Registry = map[string]CoreInfo{}

func init() {
	// --- sing-box ---
	Registry[models.CoreKindSingBox] = CoreInfo{
		Kind:                 models.CoreKindSingBox,
		Name:                 "sing-box",
		CandidateExes:        []string{"sing-box.exe", "sing-box"},
		StartArgs:            []string{"run", "-c", "{{ConfigPath}}"},
		CheckArgs:            []string{"check", "-c", "{{ConfigPath}}"},
		VersionArgs:          []string{"version"},
		VersionRe:            regexp.MustCompile(`sing-box version (\S+)`),
		VersionRePattern:     `sing-box version (\S+)`,
		DownloadURLTemplate:  "https://github.com/SagerNet/sing-box/releases/download/v{{.Version}}/sing-box-{{.Version}}-{{.OS}}-{{.Arch}}.{{.Ext}}",
		GitHubRepo:           "SagerNet/sing-box",
		ConfigFormat:         "json",
		HasClashAPI:          true,
		SupportedProtocols:   models.SupportedProtocolsByKind(models.CoreKindSingBox),
		UnsupportedTransports: []string{"kcp"},
		SSOnlyTransports:     []string{"raw", "ws"}, // sing-box SS only supports raw (no transport) and ws
	}

	// --- Xray ---
	Registry[models.CoreKindXray] = CoreInfo{
		Kind:                 models.CoreKindXray,
		Name:                 "Xray",
		CandidateExes:        []string{"xray.exe", "Xray.exe", "xray", "Xray"},
		StartArgs:            []string{"run", "-c", "{{ConfigPath}}"},
		CheckArgs:            []string{"run", "-c", "{{ConfigPath}}", "-test"},
		VersionArgs:          []string{"version"},
		VersionRe:            regexp.MustCompile(`Xray (\S+)`),
		VersionRePattern:     `Xray (\S+)`,
		DownloadURLTemplate:  "https://github.com/XTLS/Xray-core/releases/download/v{{.Version}}/Xray-{{.OS}}-{{.Arch}}.{{.Ext}}",
		GitHubRepo:           "XTLS/Xray-core",
		ConfigFormat:         "json",
		HasClashAPI:          false,
		SupportedProtocols:   models.SupportedProtocolsByKind(models.CoreKindXray),
		UnsupportedTransports: []string{}, // Xray supports kcp, ws, grpc, h2, splithttp, httpupgrade
	}

	// --- mihomo ---
	Registry[models.CoreKindMihomo] = CoreInfo{
		Kind:                 models.CoreKindMihomo,
		Name:                 "mihomo",
		CandidateExes:        []string{"mihomo.exe", "mihomo-linux-amd64", "mihomo", "Mihomo", "clash-meta"},
		StartArgs:            []string{"-f", "{{ConfigPath}}"},
		CheckArgs:            []string{"-t", "-f", "{{ConfigPath}}"},
		VersionArgs:          []string{"version"},
		VersionRe:            regexp.MustCompile(`Mihomo\s+(\S+)`),
		VersionRePattern:     `Mihomo\s+(\S+)`,
		DownloadURLTemplate:  "https://github.com/MetaCubeX/mihomo/releases/download/v{{.Version}}/mihomo-{{.OS}}-{{.Arch}}-v{{.Version}}.{{.Ext}}",
		GitHubRepo:           "MetaCubeX/mihomo",
		ConfigFormat:         "yaml",
		HasClashAPI:          true,
		SupportedProtocols:   models.SupportedProtocolsByKind(models.CoreKindMihomo),
		UnsupportedTransports: []string{"splithttp", "xhttp", "httpupgrade"}, // mihomo doesn't support these Xray transports
	}

	// --- Hysteria2 ---
	Registry[models.CoreKindHysteria2] = CoreInfo{
		Kind:                 models.CoreKindHysteria2,
		Name:                 "Hysteria2",
		CandidateExes:        []string{"hysteria2.exe", "hysteria.exe", "hysteria2", "hysteria"},
		StartArgs:            []string{"-c", "{{ConfigPath}}"},
		CheckArgs:            []string{}, // no built-in check, just validate JSON
		VersionArgs:          []string{"version"},
		VersionRe:            regexp.MustCompile(`hysteria2? (\S+)`),
		VersionRePattern:     `hysteria2? (\S+)`,
		DownloadURLTemplate:  "https://github.com/apernet/hysteria/releases/download/v{{.Version}}/hysteria-{{.OS}}-{{.Arch}}.{{.Ext}}",
		GitHubRepo:           "apernet/hysteria",
		ConfigFormat:         "json",
		HasClashAPI:          false,
		SupportedProtocols:   models.SupportedProtocolsByKind(models.CoreKindHysteria2),
		UnsupportedTransports: []string{"ws", "grpc", "h2", "splithttp", "xhttp", "httpupgrade", "kcp"}, // Hysteria2 is QUIC-only
	}
}

// GetInfo returns the CoreInfo for a given kind, or a zero-value if not found.
func GetInfo(kind string) CoreInfo {
	info, ok := Registry[kind]
	if !ok {
		return CoreInfo{Kind: kind, Name: kind}
	}
	return info
}

// AllInfo returns all registered CoreInfo records.
func AllInfo() []CoreInfo {
	out := make([]CoreInfo, 0, len(Registry))
	for _, info := range Registry {
		out = append(out, info)
	}
	return out
}

// FindExe resolves the best candidate executable name for a given kind
// by checking which file exists in the given directory.
// Returns the first candidate that exists, or the first candidate if none exist.
func FindExe(kind, dir string) string {
	info, ok := Registry[kind]
	if !ok || len(info.CandidateExes) == 0 {
		return kind // fallback
	}
	return info.CandidateExes[0]
}

// SupportsProtocol checks if a core kind supports the given protocol.
func SupportsProtocol(kind, proto string) bool {
	info, ok := Registry[kind]
	if !ok {
		return false
	}
	for _, p := range info.SupportedProtocols {
		if p == proto {
			return true
		}
	}
	return false
}

// SupportsTransport checks if a core kind supports the given transport type.
// Returns true if the transport is not in the UnsupportedTransports list.
func SupportsTransport(kind, transport string) bool {
	info, ok := Registry[kind]
	if !ok {
		return true // unknown core, assume ok
	}
	for _, t := range info.UnsupportedTransports {
		if t == transport {
			return false
		}
	}
	return true
}

// SSCompatible checks if sing-box can handle a Shadowsocks server with
// the given transport type. sing-box SS only supports "raw" (no transport)
// and "ws" (WebSocket). Other transports require Xray or mihomo.
func SSCompatible(transport string) bool {
	info, ok := Registry[models.CoreKindSingBox]
	if !ok {
		return true
	}
	if len(info.SSOnlyTransports) == 0 {
		return true
	}
	// "raw" means no transport (empty string or "tcp")
	if transport == "" || transport == "tcp" {
		transport = "raw"
	}
	for _, t := range info.SSOnlyTransports {
		if t == transport {
			return true
		}
	}
	return false
}
