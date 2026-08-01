// Package subscription fetches, parses and merges subscription sources.
package subscription

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"boxpanel/internal/import_"
	"boxpanel/internal/models"
	"boxpanel/internal/store"
)

// Manager handles subscription refresh and merge.
type Manager struct {
	store   store.Store
	httpCli *http.Client
}

// New creates a subscription Manager.
func New(s store.Store) *Manager {
	return &Manager{
		store:   s,
		httpCli: &http.Client{Timeout: 20 * time.Second},
	}
}

// Fetch downloads the subscription body.
func (m *Manager) Fetch(ctx context.Context, url, ua string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	} else if strings.Contains(strings.ToLower(url), "github") {
		req.Header.Set("User-Agent", "curl/7.88")
	} else {
		req.Header.Set("User-Agent", "clash-meta")
	}
	resp, err := m.httpCli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("subscription http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// RefreshResult summarizes a refresh operation.
type RefreshResult struct {
	ID      string `json:"id"`
	Added   int    `json:"added"`
	Updated int    `json:"updated"`
	Removed int    `json:"removed"`
	Fetched int    `json:"fetched"`
	Total   int    `json:"total"`
}

// Refresh fetches and merges one subscription. If replace is true, servers no
// longer present in the subscription are removed.
func (m *Manager) Refresh(ctx context.Context, subID string, replace bool) (*RefreshResult, error) {
	sub, err := m.store.GetSubscription(ctx, subID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, fmt.Errorf("subscription not found: %s", subID)
	}
	if sub.URL == "" {
		return nil, fmt.Errorf("subscription url empty")
	}

	st, _ := m.store.GetSettings(ctx)
	ua := sub.UserAgent
	if ua == "" {
		ua = st.SubscriptionUA
	}
	content, err := m.Fetch(ctx, sub.URL, ua)
	if err != nil {
		sub.LastStatus = "error: " + err.Error()
		sub.LastRefresh = models.Now()
		_ = m.store.SaveSubscription(ctx, *sub)
		return nil, fmt.Errorf("fetch: %w", err)
	}

	incoming, err := import_.FromBytes(content, "subscription")
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	existing, err := m.store.ListServers(ctx)
	if err != nil {
		return nil, err
	}

	merged, stats := merge(existing, incoming, replace)
	if err := m.store.BatchSaveServers(ctx, merged); err != nil {
		return nil, err
	}

	sub.LastRefresh = models.Now()
	sub.LastStatus = "ok"
	sub.LastAdded = stats.added
	sub.LastUpdated = stats.updated
	sub.LastRemoved = stats.removed
	sub.ServerCount = len(merged)
	_ = m.store.SaveSubscription(ctx, *sub)

	return &RefreshResult{
		ID: subID, Added: stats.added, Updated: stats.updated,
		Removed: stats.removed, Fetched: len(incoming), Total: len(merged),
	}, nil
}

type mergeStats struct{ added, updated, removed int }

// merge combines existing and incoming servers by dedup key.
func merge(existing, incoming []models.Server, replace bool) ([]models.Server, mergeStats) {
	existingByKey := map[string]models.Server{}
	for _, s := range existing {
		existingByKey[dedupKey(s)] = s
	}
	var out []models.Server
	seen := map[string]bool{}
	stats := mergeStats{}
	for _, s := range incoming {
		k := dedupKey(s)
		seen[k] = true
		if old, ok := existingByKey[k]; ok {
			// 更新但保留 id/added_at/延迟
			s.ID = old.ID
			s.AddedAt = old.AddedAt
			if old.LastLatency != nil {
				v := *old.LastLatency
				s.LastLatency = &v
			}
			stats.updated++ // 内容可能变化（map 字段不能直接比较，保守计为更新）
		} else {
			stats.added++
		}
		out = append(out, s)
	}
	if replace {
		for _, s := range existing {
			if !seen[dedupKey(s)] {
				stats.removed++
			}
		}
	} else {
		// 保留 incoming 没有的旧节点
		for _, s := range existing {
			if !seen[dedupKey(s)] {
				out = append(out, s)
			}
		}
	}
	return out, stats
}

func dedupKey(s models.Server) string {
	cred := s.UUID
	if s.Protocol == models.ProtoShadowsocks || s.Protocol == models.ProtoHysteria2 {
		cred = s.Password
	}
	if s.Protocol == models.ProtoTUIC {
		cred = s.TUICUUID
	}
	return fmt.Sprintf("%s|%s|%d|%s", s.Protocol, s.Server, s.ServerPort, cred)
}

// AutoRefresh refreshes all subscriptions whose interval has elapsed.
func (m *Manager) AutoRefresh(ctx context.Context) []RefreshResult {
	subs, err := m.store.ListSubscriptions(ctx)
	if err != nil {
		return nil
	}
	var results []RefreshResult
	for _, sub := range subs {
		if sub.URL == "" || sub.IntervalHours <= 0 {
			continue
		}
		if !sub.LastRefresh.IsZero() {
			if time.Since(sub.LastRefresh) < time.Duration(sub.IntervalHours)*time.Hour {
				continue
			}
		}
		r, err := m.Refresh(ctx, sub.ID, false)
		if err != nil {
			continue
		}
		results = append(results, *r)
	}
	return results
}
