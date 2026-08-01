// Package rulesets manages remote rule-set downloads and local caching.
//
// 缓存目录：$DATA_DIR/rulesets/<tag>.srs (binary) 或 .source
// 下载策略：上次更新超过 update_interval（或缺失）则重新下载。
package rulesets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"boxpanel/internal/config"
	"boxpanel/internal/models"
)

// Downloader fetches and caches remote rule-sets.
type Downloader struct {
	mu      sync.Mutex
	httpCli *http.Client
	cacheDir string
}

// New creates a Downloader. The cache directory is created on first use.
func New() *Downloader {
	dir := filepath.Join(config.DataDir(), "rulesets")
	_ = os.MkdirAll(dir, 0o755)
	return &Downloader{
		httpCli: &http.Client{Timeout: 30 * time.Second},
		cacheDir: dir,
	}
}

// CacheDir returns the local cache directory.
func (d *Downloader) CacheDir() string { return d.cacheDir }

// Status describes a rule-set's freshness.
type Status struct {
	ID        string    `json:"id"`
	Tag       string    `json:"tag"`
	URL       string    `json:"url"`
	Cached    bool      `json:"cached"`
	CachedAt  time.Time `json:"cached_at,omitempty"`
	Path      string    `json:"path,omitempty"`
	Size      int64     `json:"size"`
	Sha256    string    `json:"sha256"`
	LastError string    `json:"last_error,omitempty"`
	NextCheck time.Time `json:"next_check"`
}

// RefreshResult summarizes one download attempt.
type RefreshResult struct {
	ID       string `json:"id"`
	OK       bool   `json:"ok"`
	Path     string `json:"path,omitempty"`
	Bytes    int64  `json:"bytes"`
	Sha256   string `json:"sha256,omitempty"`
	Duration string `json:"duration,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Refresh downloads (or re-downloads) a single rule-set if it's remote and stale/missing.
// If force is true, ignore update_interval.
func (d *Downloader) Refresh(ctx context.Context, rs models.RuleSet, force bool) RefreshResult {
	start := time.Now()
	res := RefreshResult{ID: rs.ID}

	if rs.URL == "" {
		res.Error = "rule set has no URL"
		return res
	}

	localPath := d.LocalPath(rs)
	if rs.UpdateInterval <= 0 {
		rs.UpdateInterval = 168 // 默认 7 天
	}

	// 跳过下载条件：存在 + 未过期 + 非强制
	if !force {
		if info, err := os.Stat(localPath); err == nil {
			modTime := info.ModTime()
			if time.Since(modTime) < time.Duration(rs.UpdateInterval)*time.Hour {
				res.OK = true
				res.Path = localPath
				res.Bytes = info.Size()
				return res
			}
		}
	}

	// 下载
	body, sha, err := d.download(ctx, rs.URL)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	// 原子写入：tmp + rename
	if err := d.writeAtomic(localPath, body); err != nil {
		res.Error = "write: " + err.Error()
		return res
	}

	res.OK = true
	res.Path = localPath
	res.Bytes = int64(len(body))
	res.Sha256 = sha
	res.Duration = time.Since(start).String()
	return res
}

// RefreshAll refreshes every remote rule-set that needs it.
func (d *Downloader) RefreshAll(ctx context.Context, sets []models.RuleSet) []RefreshResult {
	out := make([]RefreshResult, 0, len(sets))
	for _, rs := range sets {
		if rs.Type != "remote" || rs.URL == "" {
			continue
		}
		out = append(out, d.Refresh(ctx, rs, false))
	}
	return out
}

// StatusOf returns the cache status for a rule-set (without re-downloading).
// SHA256 is computed lazily only for small files; large files skip it to avoid
// blocking the status endpoint.
func (d *Downloader) StatusOf(rs models.RuleSet) Status {
	st := Status{ID: rs.ID, Tag: rs.Tag, URL: rs.URL}
	localPath := d.LocalPath(rs)
	if info, err := os.Stat(localPath); err == nil {
		st.Cached = true
		st.CachedAt = info.ModTime()
		st.Path = localPath
		st.Size = info.Size()
		// Only compute SHA256 for small files (< 2MB) to avoid blocking
		if info.Size() < 2*1024*1024 {
			if sum, err := fileSha256(localPath); err == nil {
				st.Sha256 = sum
			}
		} else {
			st.Sha256 = "(large file, skipped)"
		}
		interval := time.Duration(rs.UpdateInterval) * time.Hour
		if interval <= 0 {
			interval = 168 * time.Hour
		}
		st.NextCheck = info.ModTime().Add(interval)
	} else {
		// 未缓存：计算下次需要拉取的时间 = now
		interval := time.Duration(rs.UpdateInterval) * time.Hour
		if interval <= 0 {
			interval = 168 * time.Hour
		}
		st.NextCheck = time.Now()
	}
	return st
}

// LocalPath returns the cache file path for a rule-set.
func (d *Downloader) LocalPath(rs models.RuleSet) string {
	if rs.Type == "local" {
		return rs.Path
	}
	ext := ".srs"
	if rs.Format == "source" {
		ext = ".source"
	}
	tag := sanitizeTag(rs.Tag)
	return filepath.Join(d.cacheDir, tag+ext)
}

func sanitizeTag(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return r.Replace(s)
}

func (d *Downloader) download(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "clash-meta")
	resp, err := d.httpCli.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

func (d *Downloader) writeAtomic(path string, body []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fileSha256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BuiltinPresets returns popular open-source remote rule-set sources.
// User can pick one to add; we don't auto-add to avoid surprise.
func BuiltinPresets() []models.RuleSet {
	common := []string{"https://cdn.jsdelivr.net/gh/", "https://raw.githubusercontent.com/"}
	return []models.RuleSet{
		{
			Tag: "geosite-cn", Type: "remote", Format: "binary",
			URL: common[0] + "SagerNet/sing-geosite@rule-set/geosite-cn.srs",
			UpdateInterval: 168, Enabled: true,
		},
		{
			Tag: "geosite-geolocation-cn", Type: "remote", Format: "binary",
			URL: common[0] + "SagerNet/sing-geosite@rule-set/geosite-geolocation-cn.srs",
			UpdateInterval: 168, Enabled: true,
		},
		{
			Tag: "geoip-cn", Type: "remote", Format: "binary",
			URL: common[0] + "SagerNet/sing-geoip@rule-set/geoip-cn.srs",
			UpdateInterval: 168, Enabled: true,
		},
		{
			Tag: "geosite-geolocation-!cn", Type: "remote", Format: "binary",
			URL: common[0] + "SagerNet/sing-geosite@rule-set/geosite-geolocation-!cn.srs",
			UpdateInterval: 168, Enabled: true,
		},
		{
			Tag: "geosite-category-ads-all", Type: "remote", Format: "binary",
			URL: common[0] + "SagerNet/sing-geosite@rule-set/geosite-category-ads-all.srs",
			UpdateInterval: 168, Enabled: true,
		},
	}
}