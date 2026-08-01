package compat

import (
	"testing"

	"boxpanel/internal/models"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.10.0", "1.10.0", 0},
		{"1.10.0", "1.11.0", -1},
		{"1.14.0", "1.10.0", 1},
		{"1.14.0-alpha.1", "1.14.0", 0},
		{"1.9.0", "1.10.7", -1},
		{"1.8.0", "1.9.0", -1},
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCheckServer_VlessReality_NewCore(t *testing.T) {
	srv := models.Server{
		Protocol: models.ProtoVless, TLSEnabled: true, RealityEnabled: true,
		RealitySpiderX: "/abc", UUID: "00000000-0000-0000-0000-000000000000",
	}
	r := CheckServer(srv, "1.14.0-alpha.1", models.CoreKindSingBox)
	if r.Level == OK {
		t.Error("expected warn/bad for reality+spider_x on 1.14, got OK")
	}
	foundSpider := false
	for _, rs := range r.Reasons {
		if rs.Code == "REALITY_SPIDERX_DROPPED" {
			foundSpider = true
		}
	}
	if !foundSpider {
		t.Error("expected REALITY_SPIDERX_DROPPED reason")
	}
}

func TestCheckServer_VlessReality_OldCore(t *testing.T) {
	srv := models.Server{
		Protocol: models.ProtoVless, TLSEnabled: true, RealityEnabled: true,
		RealitySpiderX: "/abc", UUID: "00000000-0000-0000-0000-000000000000",
	}
	r := CheckServer(srv, "1.10.7", models.CoreKindSingBox)
	if r.Level == Bad {
		t.Errorf("1.10 should accept reality+spider_x (warn at most), got bad: %+v", r)
	}
}

func TestCheckServer_Hysteria2_OldCore(t *testing.T) {
	srv := models.Server{Protocol: models.ProtoHysteria2, TLSEnabled: true, Password: "x"}
	r := CheckServer(srv, "1.5.0", models.CoreKindSingBox)
	if r.Level != Bad {
		t.Errorf("1.5 should not support hy2, got %s", r.Level)
	}
}

func TestCheckServer_XHTTPTransport(t *testing.T) {
	// 注：xhttp 解析时被规范化为 splithttp（sing-box 实际字段名）
	srv := models.Server{
		Protocol: models.ProtoVless, TransportType: "splithttp",
		UUID: "00000000-0000-0000-0000-000000000000",
	}
	r := CheckServer(srv, "1.10.0", models.CoreKindSingBox)
	if r.Level == Bad {
		t.Errorf("splithttp 1.10 should be ok, got %s", r.Level)
	}
	r2 := CheckServer(srv, "1.11.0", models.CoreKindSingBox)
	if r2.Level == Bad {
		t.Errorf("splithttp 1.11+ should be ok, got %s", r2.Level)
	}
}

func TestCheckServer_VMessAidDeprecated(t *testing.T) {
	srv := models.Server{Protocol: models.ProtoVMess, AlterID: 100, UUID: "xxx"}
	r := CheckServer(srv, "1.11.0", models.CoreKindSingBox)
	if r.Level != Bad {
		t.Errorf("VMess AID>0 on 1.11+ should be bad, got %s", r.Level)
	}
}

func TestCheckServer_CrossKind(t *testing.T) {
	// Xray does not support hysteria2
	srv := models.Server{Protocol: models.ProtoHysteria2, TLSEnabled: true, Password: "x"}
	r := CheckServer(srv, "24.1.0", models.CoreKindXray)
	if r.Level != Bad {
		t.Errorf("xray should not support hysteria2, got %s", r.Level)
	}
	// Mihomo supports hysteria2
	r2 := CheckServer(srv, "1.18.0", models.CoreKindMihomo)
	if r2.Level != OK {
		t.Errorf("mihomo should support hysteria2, got %s", r2.Level)
	}
	// Hysteria2 core does not support vless
	srv2 := models.Server{Protocol: models.ProtoVless, UUID: "xxx"}
	r3 := CheckServer(srv2, "2.0.0", models.CoreKindHysteria2)
	if r3.Level != Bad {
		t.Errorf("hysteria2 core should not support vless, got %s", r3.Level)
	}
}

func TestSuggestCore(t *testing.T) {
	srv := models.Server{
		Protocol: models.ProtoVless, TLSEnabled: true, RealityEnabled: true,
		RealitySpiderX: "/abc", UUID: "xxx",
	}
	cores := []models.CoreConfig{
		{ID: "a", Version: "1.10.7", Kind: models.CoreKindSingBox},
		{ID: "b", Version: "1.14.0", Kind: models.CoreKindSingBox},
		{ID: "c", Version: "1.11.0", Kind: models.CoreKindSingBox},
	}
	best := SuggestCore(srv, cores)
	if best == nil || best.ID != "a" {
		t.Errorf("expected core a (1.10.7 best for spider_x), got %+v", best)
	}
}