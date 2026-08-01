// Package routing compiles routing rules from a domain model into
// sing-box route.rules JSON entries.
//
// 抽出此包的目的：
//   - 独立测试（不依赖 configgen / store）
//   - 新增 rule type 只需在本包加分支
//   - 后期可演进为带 hit-count、trie、geoip cache 等的优化版本
package routing

import (
	"boxpanel/internal/models"
)

// Compiled is a single sing-box route rule.
type Compiled map[string]any

// Compile converts a models.RoutingRule into a sing-box route rule object.
// Returns nil if the rule type is unknown (so callers can skip it).
func Compile(r models.RoutingRule) Compiled {
	if r.Outbound == "" {
		return nil
	}
	rule := Compiled{"outbound": r.Outbound}
	switch r.Type {
	case models.RuleDomain:
		rule["domain"] = r.Values
	case models.RuleDomainSuffix:
		rule["domain_suffix"] = r.Values
	case models.RuleDomainKeyword:
		rule["domain_keyword"] = r.Values
	case models.RuleDomainRegex:
		rule["domain_regex"] = r.Values
	case models.RuleIPCIDR:
		rule["ip_cidr"] = r.Values
	case models.RuleGeoIP:
		rule["geoip"] = r.Values
	case models.RuleGeoSite:
		rule["geosite"] = r.Values
	case models.RuleProcess:
		rule["process_name"] = r.Values
	case models.RuleProtocol:
		rule["protocol"] = r.Values
	case models.RulePort:
		rule["port"] = r.Values
	default:
		return nil
	}
	if r.Invert {
		rule["invert"] = true
	}
	return rule
}

// CompileAll compiles an ordered list of rules, skipping invalid ones.
func CompileAll(rules []models.RoutingRule) []Compiled {
	out := make([]Compiled, 0, len(rules))
	for _, r := range rules {
		if c := Compile(r); c != nil {
			out = append(out, c)
		}
	}
	return out
}

// TypeLabel returns a human-readable label for a rule type.
func TypeLabel(t string) string {
	switch t {
	case models.RuleDomain:
		return "domain"
	case models.RuleDomainSuffix:
		return "domain_suffix"
	case models.RuleDomainKeyword:
		return "domain_keyword"
	case models.RuleDomainRegex:
		return "domain_regex"
	case models.RuleIPCIDR:
		return "ip_cidr"
	case models.RuleGeoIP:
		return "geoip"
	case models.RuleGeoSite:
		return "geosite"
	case models.RuleProcess:
		return "process"
	case models.RuleProtocol:
		return "protocol"
	case models.RulePort:
		return "port"
	}
	return t
}

// AllTypes returns all supported rule types (for UI dropdown).
func AllTypes() []string {
	return []string{
		models.RuleDomain,
		models.RuleDomainSuffix,
		models.RuleDomainKeyword,
		models.RuleDomainRegex,
		models.RuleIPCIDR,
		models.RuleGeoSite,
		models.RuleGeoIP,
		models.RuleProcess,
		models.RuleProtocol,
		models.RulePort,
	}
}