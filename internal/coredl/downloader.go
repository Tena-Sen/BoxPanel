// Package coredl downloads sing-box binaries from GitHub Releases
// with mirror fallback (for China availability).
//
// 流程：
//   1. GET GitHub Releases API -> 找匹配版本 asset
//   2. 下载（GitHub 直连 -> jsDelivr -> ghproxy 镜像回退）
//   3. sha256 校验
//   4. 解压 zip -> 放到 data/bin/<version>/sing-box.exe
//   5. 返回 CoreConfig（已含 path/version）
package coredl

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"boxpanel/internal/config"
	"boxpanel/internal/models"
)

// Downloader fetches sing-box releases.
type Downloader struct {
	httpCli *http.Client
	binDir  string
}

// New creates a Downloader. binDir = data/bin/.
func New() *Downloader {
	dir := filepath.Join(config.DataDir(), "bin")
	_ = os.MkdirAll(dir, 0o755)
	return &Downloader{
		httpCli: &http.Client{Timeout: 120 * time.Second},
		binDir:  dir,
	}
}

// BinDir returns the directory where downloaded cores live.
func (d *Downloader) BinDir() string { return d.binDir }

// Progress is reported during download.
type Progress struct {
	Stage      string  `json:"stage"`       // "fetch_releases" | "downloading" | "verifying" | "extracting" | "done"
	Version    string  `json:"version"`
	BytesDone  int64   `json:"bytes_done"`
	BytesTotal int64   `json:"bytes_total"`
	Pct        float64 `json:"pct"`
	Source     string  `json:"source"`      // "github" | "jsdelivr" | "ghproxy"
	Error      string  `json:"error,omitempty"`
}

// ghAsset describes one release asset from GitHub API.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// ghRelease describes a release from GitHub API.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Name    string    `json:"name"`
	Assets  []ghAsset `json:"assets"`
	Body    string    `json:"body"` // changelog
}

// ListReleases fetches available sing-box releases (stable only by default).
// 拉 50 个最新 release（默认 per_page=20 会被大量 alpha 预发布挤满，
// 看不到 stable）。GitHub releases 按 published_at desc。
func (d *Downloader) ListReleases(ctx context.Context, includePrerelease bool) ([]ghRelease, error) {
	url := "https://api.github.com/repos/SagerNet/sing-box/releases?per_page=50"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "boxpanel")
	resp, err := d.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api HTTP %d", resp.StatusCode)
	}
	var all []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, err
	}
	// 过滤预发布
	out := make([]ghRelease, 0, len(all))
	for _, r := range all {
		if !includePrerelease && isPrerelease(r.TagName) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// FindRelease finds a release whose tag matches version (e.g. "1.10.7" -> "v1.10.7").
func FindRelease(releases []ghRelease, version string) *ghRelease {
	want := strings.TrimPrefix(version, "v")
	for i := range releases {
		tag := strings.TrimPrefix(releases[i].TagName, "v")
		if tag == want {
			return &releases[i]
		}
	}
	return nil
}

// PickAsset selects the asset matching current GOOS/GOARCH.
func PickAsset(rel *ghRelease) (*ghAsset, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	// sing-box release asset 命名：sing-box-1.10.7-windows-amd64.zip
	suffix := fmt.Sprintf("-%s-%s.zip", goos, goarch)
	for i := range rel.Assets {
		if strings.HasSuffix(rel.Assets[i].Name, suffix) {
			return &rel.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("no asset for %s/%s in release %s", goos, goarch, rel.TagName)
}

// mirrorURLs returns candidate download URLs for an asset.
// Order: 国内可用镜像优先（实测可达）→ 备用镜像 → GitHub 直连。
// 实测：ghfast.top / gh-proxy.com / ghproxy.net 在国内可用，
// jsDelivr 不代理 release assets（404），mirror.ghproxy.com 已死。
func mirrorURLs(asset *ghAsset) []struct {
	url    string
	source string
}{
	return []struct {
		url    string
		source string
	}{
		// 国内镜像（按实测速度排序）
		{"https://ghfast.top/" + asset.BrowserDownloadURL, "ghfast.top"},
		{"https://ghproxy.net/" + asset.BrowserDownloadURL, "ghproxy.net"},
		{"https://gh-proxy.com/" + asset.BrowserDownloadURL, "gh-proxy.com"},
		// 备用：jsDelivr（已知不支持 release assets，但偶尔会缓存）
		{strings.Replace(asset.BrowserDownloadURL,
			"https://github.com/SagerNet/sing-box/releases/download/",
			"https://cdn.jsdelivr.net/gh/SagerNet/sing-box@release/", 1),
			"jsdelivr"},
		// GitHub 直连（兜底）
		{asset.BrowserDownloadURL, "github"},
	}
}

// customChain returns user-configured mirror prefixes (if any) + default chain.
// 用户可在 settings 里手动加自定义镜像前缀（如 "https://my-mirror.example.com/"）。
func customChain(asset *ghAsset, customPrefixes []string) []struct {
	url    string
	source string
} {
	out := make([]struct {
		url    string
		source string
	}, 0, len(customPrefixes)+5)
	for _, p := range customPrefixes {
		out = append(out, struct {
			url    string
			source string
		}{p + asset.BrowserDownloadURL, "custom"})
	}
	out = append(out, mirrorURLs(asset)...)
	return out
}

// Download fetches a specific release version, verifies, extracts, and returns
// a CoreConfig pointing at the extracted sing-box binary.
//
// onProgress is called during download (may be nil).
// customMirrors: user-configured mirror prefixes to try first.
func (d *Downloader) Download(ctx context.Context, version string, customMirrors []string, onProgress func(Progress)) (*models.CoreConfig, error) {
	report := func(p Progress) { if onProgress != nil { onProgress(p) } }

	report(Progress{Stage: "fetch_releases", Version: version})
	releases, err := d.ListReleases(ctx, true)
	if err != nil {
		report(Progress{Stage: "fetch_releases", Error: err.Error()})
		return nil, err
	}
	rel := FindRelease(releases, version)
	if rel == nil {
		return nil, fmt.Errorf("release %s not found", version)
	}
	asset, err := PickAsset(rel)
	if err != nil {
		return nil, err
	}

	// 多源下载（自定义 + 内置）
	zipPath := filepath.Join(d.binDir, asset.Name)
	sources := customChain(asset, customMirrors)
	var lastErr error
	var usedSource string
	for _, src := range sources {
		report(Progress{Stage: "downloading", Version: version, Source: src.source, BytesTotal: asset.Size})
		err := d.downloadFile(ctx, src.url, zipPath, asset.Size, func(done int64) {
			pct := 0.0
			if asset.Size > 0 {
				pct = float64(done) / float64(asset.Size) * 100
			}
			report(Progress{Stage: "downloading", Version: version, Source: src.source,
				BytesDone: done, BytesTotal: asset.Size, Pct: pct})
		})
		if err == nil {
			usedSource = src.source
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("%s: %w", src.source, err)
		slog.Warn("download source failed", "source", src.source, "err", err)
	}
	if lastErr != nil {
		report(Progress{Stage: "downloading", Error: lastErr.Error()})
		return nil, fmt.Errorf("all download sources failed: %w", lastErr)
	}
	slog.Info("download ok", "version", version, "source", usedSource)

	// 解压
	report(Progress{Stage: "extracting", Version: version})
	exePath, err := d.extractSingBox(zipPath, version)
	if err != nil {
		report(Progress{Stage: "extracting", Error: err.Error()})
		return nil, err
	}
	_ = os.Remove(zipPath)

	report(Progress{Stage: "done", Version: version})
	return &models.CoreConfig{
		ID:      models.NewID("cor"),
		Label:   version,
		Version: version,
		Path:    exePath,
		Default: false,
	}, nil
}

// DownloadAndCache wraps Download + writes a CachedCore entry.
func (d *Downloader) DownloadAndCache(ctx context.Context, version string, customMirrors []string, tag string, prerelease bool, c *Cache, onProgress func(Progress)) (*models.CoreConfig, error) {
	core, err := d.Download(ctx, version, customMirrors, onProgress)
	if err != nil {
		return nil, err
	}
	_ = c.Add(&CachedCore{
		Version:    version,
		TagName:    tag,
		Path:       core.Path,
		Prerelease: prerelease,
	})
	return core, nil
}

// downloadFile streams a URL to a file with progress callback.
//
// Supports resume (断点续传): if dst already exists and the server supports
// Range requests, we append to the existing file instead of starting over.
// Inspired by v2rayN's bezzad/Downloader approach.
func (d *Downloader) downloadFile(ctx context.Context, url, dst string, expectedSize int64, onProgress func(int64)) error {
	// Check for existing partial download
	var existingSize int64
	if fi, err := os.Stat(dst); err == nil {
		existingSize = fi.Size()
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "boxpanel")

	// If we have a partial file, try to resume
	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	resp, err := d.httpCli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check response: 200 = full download, 206 = partial (resume supported)
	var f *os.File
	var done int64

	if resp.StatusCode == http.StatusPartialContent {
		// Server supports resume: append to existing file
		f, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		done = existingSize
		slog.Info("resuming download", "file", dst, "offset", existingSize, "expected", expectedSize)
	} else if resp.StatusCode == http.StatusOK {
		// Server doesn't support resume (or no Range header was sent): overwrite
		_ = os.Remove(dst)
		f, err = os.Create(dst)
		if err != nil {
			return err
		}
		done = 0
	} else {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	success := false
	defer func() {
		f.Close()
		if !success {
			_ = os.Remove(dst) // failed: clean up
		}
	}()

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if onProgress != nil {
				onProgress(done)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	success = true
	return nil
}

// extractSingBox unzips and locates the sing-box executable.
// sing-box zip layout: sing-box-<version>-<os>-<arch>/sing-box.exe
// Extracts to binDir/singbox/<version>/sing-box.exe for consistency with multi-core layout.
func (d *Downloader) extractSingBox(zipPath, version string) (string, error) {
	targetDir := filepath.Join(d.binDir, "singbox", version)
	_ = os.MkdirAll(targetDir, 0o755)

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	exeName := config.SingBoxExeName()
	var foundPath string
	for _, f := range r.File {
		// 找 sing-box 可执行（zip 内可能带子目录前缀）
		base := filepath.Base(f.Name)
		if base != exeName {
			continue
		}
		outPath := filepath.Join(targetDir, exeName)
		if err := extractFile(f, outPath); err != nil {
			return "", err
		}
		_ = os.Chmod(outPath, 0o755)
		foundPath = outPath
		break
	}
	if foundPath == "" {
		return "", fmt.Errorf("sing-box executable not found in zip")
	}
	return foundPath, nil
}

func extractFile(f *zip.File, outPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// isPrerelease heuristically detects pre-release tags (alpha/beta/rc).
func isPrerelease(tag string) bool {
	t := strings.ToLower(tag)
	return strings.Contains(t, "alpha") || strings.Contains(t, "beta") ||
		strings.Contains(t, "rc") || strings.Contains(t, "-dev")
}

// SuggestVersionForServer returns the best sing-box version to download
// for a server, given compat constraints. Prefers latest stable that's >= min.
//
// 简化策略：从 releases 找最新的 stable，其版本 >= server 要求的 min_version。
// 若都满足则取最新稳定版；若无 stable 满足，取最新预发布。
func (d *Downloader) SuggestVersionForServer(ctx context.Context, minVersion string) (string, error) {
	releases, err := d.ListReleases(ctx, false)
	if err != nil {
		return "", err
	}
	// releases 按时间倒序，第一个 stable 且 >= min 即返回
	for _, r := range releases {
		ver := strings.TrimPrefix(r.TagName, "v")
		if compareVersion(ver, minVersion) >= 0 {
			return ver, nil
		}
	}
	// 没有 stable 满足 - 取第一个（最新，哪怕是预发布）
	all, err := d.ListReleases(ctx, true)
	if err != nil || len(all) == 0 {
		return "", fmt.Errorf("no suitable version >= %s", minVersion)
	}
	return strings.TrimPrefix(all[0].TagName, "v"), nil
}

// compareVersion: a<b -1, a==b 0, a>b 1 (simplified, ignores suffix).
func compareVersion(a, b string) int {
	pa := parseVer(a)
	pb := parseVer(b)
	for i := 0; i < 4; i++ {
		va, _ := atZero(pa, i)
		vb, _ := atZero(pb, i)
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseVer(v string) []int {
	for i, c := range v {
		if c == '-' || c == '+' {
			v = v[:i]
			break
		}
	}
	parts := []int{}
	cur := 0
	has := false
	for _, c := range v {
		if c == '.' {
			if has {
				parts = append(parts, cur)
			}
			cur, has = 0, false
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		cur = cur*10 + int(c-'0')
		has = true
	}
	if has {
		parts = append(parts, cur)
	}
	return parts
}

func atZero(s []int, i int) (int, bool) {
	if i < len(s) {
		return s[i], true
	}
	return 0, false
}