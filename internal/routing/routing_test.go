package routing

import (
	"reflect"
	"testing"

	"boxpanel/internal/models"
)

func TestCompile_DomainSuffix(t *testing.T) {
	r := models.RoutingRule{
		Type:     models.RuleDomainSuffix,
		Values:   []string{"google.com", "youtube.com"},
		Outbound: models.OutProxy,
	}
	got := Compile(r)
	want := Compiled{
		"outbound":      "proxy",
		"domain_suffix": []string{"google.com", "youtube.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCompile_IPCIDR_WithInvert(t *testing.T) {
	r := models.RoutingRule{
		Type:     models.RuleIPCIDR,
		Values:   []string{"192.168.0.0/16"},
		Outbound: models.OutDirect,
		Invert:   true,
	}
	got := Compile(r)
	if got["invert"] != true {
		t.Error("expected invert=true")
	}
	if got["outbound"] != "direct" {
		t.Errorf("outbound=%v", got["outbound"])
	}
}

func TestCompile_UnknownType(t *testing.T) {
	got := Compile(models.RoutingRule{Type: "weird", Outbound: "proxy"})
	if got != nil {
		t.Errorf("expected nil for unknown type, got %+v", got)
	}
}

func TestCompileAll_SkipsInvalid(t *testing.T) {
	rules := []models.RoutingRule{
		{Type: models.RuleDomainSuffix, Values: []string{"a.com"}, Outbound: "proxy"},
		{Type: "bogus", Outbound: "proxy"},
		{Type: models.RuleIPCIDR, Values: []string{"1.1.1.1"}, Outbound: "direct"},
	}
	got := CompileAll(rules)
	if len(got) != 2 {
		t.Errorf("expected 2 compiled, got %d: %+v", len(got), got)
	}
}

func TestCompile_GeoSiteAndProcess(t *testing.T) {
	c1 := Compile(models.RoutingRule{Type: models.RuleGeoSite, Values: []string{"cn"}, Outbound: "direct"})
	if c1["geosite"] == nil {
		t.Error("expected geosite field")
	}
	c2 := Compile(models.RoutingRule{Type: models.RuleProcess, Values: []string{"chrome.exe"}, Outbound: "direct"})
	if c2["process_name"] == nil {
		t.Error("expected process_name field")
	}
}