// Package configgen: sing-box 版本 schema 适配器。
//
// 不同 sing-box 版本支持的配置字段有差异，生成配置时按目标版本剔除
// 不兼容的字段，确保生成的 JSON 一定能被指定版本的 sing-box 接受。
package configgen

import "strings"

// CompareVersions: a<b return -1, a==b return 0, a>b return 1.
// 支持 "1.14.0-alpha.1"、"1.10.7"、"1.9.0-rc1" 等格式。
func CompareVersions(a, b string) int {
	pa := parseVersion(a)
	pb := parseVersion(b)
	for i := 0; i < 4; i++ {
		va, _ := atOrZero(pa, i)
		vb, _ := atOrZero(pb, i)
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseVersion(v string) []int {
	// 去掉 v 前缀和后缀 -xxx
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, 4)
	for _, p := range parts {
		if p == "" {
			continue
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}

func atOrZero(s []int, i int) (int, bool) {
	if i < len(s) {
		return s[i], true
	}
	return 0, false
}

// Adapters returns the set of field-removal rules for a given sing-box version.
// Newer sing-box: fewer removals. Older: more removals.
type Adapter struct {
	version string
}

func NewAdapter(version string) *Adapter {
	if version == "" {
		// 未知版本 - 不做裁剪
		return &Adapter{version: ""}
	}
	return &Adapter{version: version}
}

// Apply modifies cfg in-place to remove fields incompatible with this sing-box version.
func (a *Adapter) Apply(cfg map[string]any) {
	if a.version == "" {
		return
	}
	// experimental.cache_file 在 1.11+ 才稳定；旧版本可能报错
	if CompareVersions(a.version, "1.11.0") < 0 {
		if exp, ok := cfg["experimental"].(map[string]any); ok {
			delete(exp, "cache_file")
		}
	}

	// rule.action 等 1.11+ 才有；旧版只支持 outbound 字段
	// 我们生成的格式已稳定，无需特殊处理

	// outbounds[*].tls.reality.spider_x 1.14+ 已移除，无需剔除（生成时本就不带）

	// outbounds[*].packet_encoding 1.9+ 才有
	if CompareVersions(a.version, "1.9.0") < 0 {
		for _, ob := range getOutbounds(cfg) {
			delete(ob, "packet_encoding")
		}
	}

	// Transport type naming: "xhttp" → "splithttp" in sing-box 1.11+
	// sing-box < 1.11.0 uses "xhttp"; 1.11.0+ uses "splithttp"
	for _, ob := range getOutbounds(cfg) {
		normalizeTransport(ob, a.version)
	}

	// log.timestamp 1.8+
	if CompareVersions(a.version, "1.8.0") < 0 {
		if log, ok := cfg["log"].(map[string]any); ok {
			delete(log, "timestamp")
		}
	}

	// route.final 默认值差异 - 不需要适配

	// DNS final string OK in all versions
}

// normalizeTransport adjusts the transport type field based on the target sing-box version.
// - sing-box >= 1.11.0: "xhttp" → "splithttp" (the canonical name)
// - sing-box < 1.11.0: "splithttp" → "xhttp" (the old name)
// - version unknown (""): keep original value (safe default — user's link may use either)
func normalizeTransport(ob map[string]any, version string) {
	transport, ok := ob["transport"].(map[string]any)
	if !ok {
		return
	}
	ttype, _ := transport["type"].(string)
	if ttype == "" {
		return
	}
	if version == "" {
		// Unknown version — try "splithttp" first (modern default), let config check fail
		// if the version is too old, which gives a clear error.
		// But actually, safer to keep original value from the link.
		return
	}
	if ttype == "xhttp" && CompareVersions(version, "1.11.0") >= 0 {
		transport["type"] = "splithttp"
	} else if ttype == "splithttp" && CompareVersions(version, "1.11.0") < 0 {
		transport["type"] = "xhttp"
	}
}

func getOutbounds(cfg map[string]any) []map[string]any {
	obs, _ := cfg["outbounds"].([]map[string]any)
	if len(obs) == 0 {
		// type assert for any-typed map slice
		anyObs, _ := cfg["outbounds"].([]any)
		out := make([]map[string]any, 0, len(anyObs))
		for _, o := range anyObs {
			if m, ok := o.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return obs
}