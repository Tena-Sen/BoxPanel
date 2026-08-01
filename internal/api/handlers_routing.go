package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"boxpanel/internal/models"
	"boxpanel/internal/rulesets"
)

// ----- routing rules -----

func (s *APIServer) handleListRoutingRules(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profile_id")
	rules, _ := s.store.ListRoutingRules(r.Context(), profileID)
	if rules == nil {
		rules = []models.RoutingRule{}
	}
	writeJSON(w, 200, rules)
}

func (s *APIServer) handleCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	var rule models.RoutingRule
	if err := readJSON(r, &rule); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if rule.ID == "" {
		rule.ID = models.NewID("rule")
	}
	if rule.ProfileID == "" {
		rule.ProfileID = "default" // 默认 profile，与 defaultProfile 一致
	}
	if rule.Outbound == "" {
		rule.Outbound = models.OutProxy
	}
	_ = s.store.SaveRoutingRule(r.Context(), rule)
	writeJSON(w, 200, rule)
}

func (s *APIServer) handleUpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	rule := mustBody(models.RoutingRule{}, r, w)
	if rule == nil {
		return
	}
	rule.ID = chi.URLParam(r, "id")
	_ = s.store.SaveRoutingRule(r.Context(), *rule)
	writeJSON(w, 200, rule)
}

func (s *APIServer) handleDeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	_ = s.store.DeleteRoutingRule(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"deleted": chi.URLParam(r, "id")})
}

func (s *APIServer) handleReorderRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID string   `json:"profile_id"`
		IDs       []string `json:"ids"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := s.store.ReorderRoutingRules(r.Context(), body.ProfileID, body.IDs); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ----- rule sets -----

func (s *APIServer) handleListRuleSets(w http.ResponseWriter, r *http.Request) {
	sets, _ := s.store.ListRuleSets(r.Context())
	if sets == nil {
		sets = []models.RuleSet{}
	}
	writeJSON(w, 200, sets)
}

func (s *APIServer) handleSaveRuleSet(w http.ResponseWriter, r *http.Request) {
	var rs models.RuleSet
	if err := readJSON(r, &rs); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if rs.ID == "" {
		rs.ID = models.NewID("rs")
	}
	if rs.Type == "remote" && rs.UpdateInterval == 0 {
		rs.UpdateInterval = 168 // 默认 7 天
	}
	_ = s.store.SaveRuleSet(r.Context(), rs)
	writeJSON(w, 200, rs)
}

func (s *APIServer) handleDeleteRuleSet(w http.ResponseWriter, r *http.Request) {
	_ = s.store.DeleteRuleSet(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"deleted": chi.URLParam(r, "id")})
}

// GET /api/rule-sets/builtin — 内置推荐源（开源热门）
func (s *APIServer) handleBuiltinRuleSets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"items": rulesets.BuiltinPresets()})
}

// GET /api/rule-sets/status — 列出所有规则集 + 缓存状态
// 加 5 秒超时保护，防止文件扫描阻塞 API
func (s *APIServer) handleRuleSetsStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sets, _ := s.store.ListRuleSets(ctx)
	statuses := make([]rulesets.Status, 0, len(sets))
	for _, rs := range sets {
		select {
		case <-ctx.Done():
			// 超时则返回已收集的部分结果
			writeJSON(w, 200, statuses)
			return
		default:
		}
		statuses = append(statuses, s.rs.StatusOf(rs))
	}
	writeJSON(w, 200, statuses)
}

// POST /api/rule-sets/{id}/refresh — 立即下载
func (s *APIServer) handleRefreshRuleSet(w http.ResponseWriter, r *http.Request) {
	rs, err := s.store.GetRuleSet(r.Context(), chi.URLParam(r, "id"))
	if err != nil || rs == nil {
		writeError(w, 404, "not found")
		return
	}
	res := s.rs.Refresh(r.Context(), *rs, true)
	if !res.OK {
		writeError(w, 500, res.Error)
		return
	}
	writeJSON(w, 200, res)
}

// POST /api/rule-sets/refresh-all — 刷新所有过期的远程集
func (s *APIServer) handleRefreshAllRuleSets(w http.ResponseWriter, r *http.Request) {
	sets, _ := s.store.ListRuleSets(r.Context())
	results := s.rs.RefreshAll(r.Context(), sets)
	writeJSON(w, 200, map[string]any{"results": results})
}
