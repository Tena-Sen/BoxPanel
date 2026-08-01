// Package coredl: 本地内核缓存管理。
//
// `data/bin/<version>/sing-box.exe` 是实际文件；
// `data/bin/.cache.json` 是元数据清单（版本、路径、安装时间、上次检查）。
//
// 设计目标：
//   - 启动时静默 check GitHub 最新版 → 自动下载缺失或过期版本
//   - 用户/auto-match 优先用本地缓存，不重复下载
//   - 设置页能查看本地有哪些版本、上次更新时间
package coredl

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const cacheFileName = ".cache.json"

// CachedCore describes one locally cached sing-box binary.
type CachedCore struct {
	Version    string    `json:"version"`     // "1.13.14"
	TagName    string    `json:"tag_name"`    // "v1.13.14"
	Path       string    `json:"path"`        // data/bin/1.13.14/sing-box.exe
	InstalledAt time.Time `json:"installed_at"`
	SizeBytes  int64     `json:"size_bytes"`
	Prerelease bool      `json:"prerelease"`
}

// Cache manages the local cache index.
type Cache struct {
	mu      sync.Mutex
	path    string
	binDir  string
	entries map[string]*CachedCore // key = version
}

// NewCache opens or creates the cache index. binDir is the directory
// where sing-box binaries are extracted (e.g. data/bin/).
func NewCache(binDir string) (*Cache, error) {
	_ = os.MkdirAll(binDir, 0o755)
	c := &Cache{
		path:    filepath.Join(binDir, cacheFileName),
		binDir:  binDir,
		entries: map[string]*CachedCore{},
	}
	if err := c.load(); err != nil {
		// 损坏不致命，重新开始
		c.entries = map[string]*CachedCore{}
	}
	return c, nil
}

// load reads the index file from disk.
func (c *Cache) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []*CachedCore
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, e := range list {
		c.entries[e.Version] = e
	}
	return nil
}

// save writes the index atomically.
func (c *Cache) save() error {
	list := make([]*CachedCore, 0, len(c.entries))
	for _, e := range c.entries {
		list = append(list, e)
	}
	// 稳定顺序
	sort.Slice(list, func(i, j int) bool { return list[i].Version > list[j].Version })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// List returns all cached cores sorted by version desc.
func (c *Cache) List() []*CachedCore {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*CachedCore, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out
}

// Has reports whether the given version is cached locally.
func (c *Cache) Has(version string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[version]
	return ok
}

// Add inserts/updates a cached core (called by Downloader after a successful download).
func (c *Cache) Add(core *CachedCore) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	core.InstalledAt = time.Now()
	c.entries[core.Version] = core
	return c.save()
}

// Remove deletes a cached core by version (file + index entry).
func (c *Cache) Remove(version string) error {
	c.mu.Lock()
	entry, ok := c.entries[version]
	if ok {
		delete(c.entries, version)
	}
	c.mu.Unlock()
	if ok && entry != nil {
		_ = os.RemoveAll(filepath.Dir(entry.Path))
	}
	return c.save()
}

// Path returns the cached sing-box binary path for version (assumes Has).
func (c *Cache) Path(version string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[version]; ok {
		return e.Path
	}
	return ""
}

// LastChecked returns the most recent InstalledAt across all entries (0 if none).
func (c *Cache) LastChecked() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	var latest time.Time
	for _, e := range c.entries {
		if e.InstalledAt.After(latest) {
			latest = e.InstalledAt
		}
	}
	return latest
}

// Sweep removes cache entries whose underlying file no longer exists on disk.
func (c *Cache) Sweep() error {
	c.mu.Lock()
	removed := []string{}
	for v, e := range c.entries {
		if _, err := os.Stat(e.Path); err != nil {
			delete(c.entries, v)
			removed = append(removed, v)
		}
	}
	c.mu.Unlock()
	if len(removed) > 0 {
		return c.save()
	}
	return nil
}

// AutoUpdate checks GitHub for missing/older versions and downloads them in background.
//
// 策略（每次启动自动触发一次）：
//   1. 拉 GitHub 最新 stable（5 个）
//   2. 对每个：若本地缓存无 → 下载（后台）
//   3. 不主动更新本地已有的（用户显式点"升级"才替换）
//
// 这样冷启动后用户能立刻用到本地缓存的版本，无需每次下载。
func AutoUpdate(ctx context.Context, dl *Downloader, c *Cache, customMirrors []string) {
	releases, err := dl.ListReleases(ctx, false)
	if err != nil {
		slog.Warn("auto-update: list releases failed", "err", err)
		return
	}
	for _, rel := range releases {
		ver := trimV(rel.TagName)
		if c.Has(ver) {
			continue // 已有，不重复下载
		}
		// 后台下载
		go func(version, tag string, prerelease bool) {
			cctx, cancel := context.WithTimeout(context.Background(), 5*60)
			defer cancel()
			core, err := dl.Download(cctx, version, customMirrors, nil)
			if err != nil {
				slog.Warn("auto-update download failed", "version", version, "err", err)
				return
			}
			_ = c.Add(&CachedCore{
				Version:     version,
				TagName:     tag,
				Path:        core.Path,
				SizeBytes:   0, // 可选
				Prerelease:  prerelease,
			})
			slog.Info("auto-update: added", "version", version)
		}(ver, rel.TagName, isPrerelease(rel.TagName))
	}
}

func trimV(s string) string {
	if len(s) > 0 && s[0] == 'v' {
		return s[1:]
	}
	return s
}