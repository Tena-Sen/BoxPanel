// Package coredl: multi-core downloader for Xray, mihomo, Hysteria2 and sing-box.
//
// Each core has its own GitHub repo, asset naming pattern, and extraction logic.
// This file provides the MultiCoreDownloader that abstracts over these differences.
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

// CoreRepo describes a core's GitHub repository and asset naming.
type CoreRepo struct {
	Kind           string // "singbox" | "xray" | "mihomo" | "hysteria2"
	Owner          string // GitHub owner
	Repo           string // GitHub repo name
	ExeName        string // executable name (without .exe)
	AssetPattern   string // asset naming: "windows-amd64" | "windows-x64" etc.
	IsZip          bool   // whether assets are zip files
	ExtraMirrorURL string // optional extra mirror base URL
}

// KnownRepos defines the GitHub repos for each core kind.
var KnownRepos = map[string]CoreRepo{
	models.CoreKindSingBox: {
		Kind:    models.CoreKindSingBox,
		Owner:   "SagerNet",
		Repo:    "sing-box",
		ExeName: "sing-box",
		IsZip:   true,
	},
	models.CoreKindXray: {
		Kind:    models.CoreKindXray,
		Owner:   "XTLS",
		Repo:    "Xray-core",
		ExeName: "xray",
		IsZip:   true,
	},
	models.CoreKindMihomo: {
		Kind:    models.CoreKindMihomo,
		Owner:   "MetaCubeXD",
		Repo:    "mihomo",
		ExeName: "mihomo",
		IsZip:   true,
	},
	models.CoreKindHysteria2: {
		Kind:    models.CoreKindHysteria2,
		Owner:   "apernet",
		Repo:    "hysteria",
		ExeName: "hysteria",
		IsZip:   true,
	},
}

// MultiCoreDownloader downloads any supported core binary.
type MultiCoreDownloader struct {
	httpCli *http.Client
	binDir  string
}

// NewMultiCoreDownloader creates a multi-core downloader.
func NewMultiCoreDownloader() *MultiCoreDownloader {
	dir := filepath.Join(config.DataDir(), "bin")
	_ = os.MkdirAll(dir, 0o755)
	return &MultiCoreDownloader{
		httpCli: &http.Client{Timeout: 120 * time.Second},
		binDir:  dir,
	}
}

// ListAvailableVersions fetches releases for a given core kind.
func (m *MultiCoreDownloader) ListAvailableVersions(ctx context.Context, kind string, includePrerelease bool) ([]ghRelease, error) {
	repo, ok := KnownRepos[kind]
	if !ok {
		return nil, fmt.Errorf("unknown core kind: %s", kind)
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=30", repo.Owner, repo.Repo)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "boxpanel")
	resp, err := m.httpCli.Do(req)
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
	out := make([]ghRelease, 0, len(all))
	for _, r := range all {
		if !includePrerelease && isPrerelease(r.TagName) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// DownloadCore downloads a specific core version from GitHub.
func (m *MultiCoreDownloader) DownloadCore(ctx context.Context, kind, version string, customMirrors []string, onProgress func(Progress)) (*models.CoreConfig, error) {
	repo, ok := KnownRepos[kind]
	if !ok {
		return nil, fmt.Errorf("unknown core kind: %s", kind)
	}

	report := func(p Progress) { if onProgress != nil { onProgress(p) } }
	report(Progress{Stage: "fetch_releases", Version: version})

	releases, err := m.ListAvailableVersions(ctx, kind, true)
	if err != nil {
		return nil, err
	}

	// Find the release matching this version
	var rel *ghRelease
	cleanVer := strings.TrimPrefix(version, "v")
	for i := range releases {
		if strings.TrimPrefix(releases[i].TagName, "v") == cleanVer {
			rel = &releases[i]
			break
		}
	}
	if rel == nil {
		return nil, fmt.Errorf("version %s not found for %s", version, kind)
	}

	// Find matching asset
	asset, err := pickCoreAsset(rel, repo)
	if err != nil {
		return nil, err
	}

	// Download with mirror fallback
	sources := customCoreChain(asset, customMirrors, kind)
	zipPath := filepath.Join(m.binDir, asset.Name)
	var lastErr error
	var usedSource string
	for _, src := range sources {
		report(Progress{Stage: "downloading", Version: version, Source: src.source, BytesTotal: asset.Size})
		err := m.downloadFile(ctx, src.url, zipPath, asset.Size, func(done int64) {
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
		return nil, fmt.Errorf("all download sources failed: %w", lastErr)
	}
	slog.Info("download ok", "kind", kind, "version", version, "source", usedSource)

	// Extract
	report(Progress{Stage: "extracting", Version: version})
	exePath, err := m.extractCore(zipPath, repo, version)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(zipPath)

	report(Progress{Stage: "done", Version: version})
	return &models.CoreConfig{
		ID:      models.NewID("cor"),
		Kind:    kind,
		Label:   repo.ExeName + " " + version,
		Version: version,
		Path:    exePath,
		Default: false,
	}, nil
}

// downloadFile streams a URL to a file with progress callback.
// Supports resume (断点续传): if dst already exists and the server supports
// Range requests, we append instead of starting over.
func (m *MultiCoreDownloader) downloadFile(ctx context.Context, url, dst string, expectedSize int64, onProgress func(int64)) error {
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

	resp, err := m.httpCli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var f *os.File
	var done int64

	if resp.StatusCode == http.StatusPartialContent {
		// Server supports resume: append to existing file
		f, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		done = existingSize
		slog.Info("resuming download", "file", dst, "offset", existingSize)
	} else if resp.StatusCode == http.StatusOK {
		// Server doesn't support resume: overwrite
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
			_ = os.Remove(dst)
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

// extractCore unzips and locates the core executable.
// For Xray, also extracts geoip.dat and geosite.dat which are required for routing rules.
func (m *MultiCoreDownloader) extractCore(zipPath string, repo CoreRepo, version string) (string, error) {
	targetDir := filepath.Join(m.binDir, repo.Kind, version)
	_ = os.MkdirAll(targetDir, 0o755)

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	exeName := repo.ExeName
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}

	// Xray needs geoip.dat and geosite.dat in its working directory
	needGeodata := repo.Kind == models.CoreKindXray
	geodataFiles := map[string]bool{"geoip.dat": true, "geosite.dat": true}

	var foundPath string
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base == exeName {
			outPath := filepath.Join(targetDir, exeName)
			if err := extractCoreFile(f, outPath); err != nil {
				return "", err
			}
			_ = os.Chmod(outPath, 0o755)
			foundPath = outPath
		} else if needGeodata && geodataFiles[base] {
			outPath := filepath.Join(targetDir, base)
			if err := extractCoreFile(f, outPath); err != nil {
				slog.Warn("failed to extract geodata file, routing rules using geodata may not work",
					"file", base, "error", err)
			} else {
				slog.Info("extracted geodata for Xray", "file", base, "size", f.UncompressedSize64)
			}
		}
	}
	if foundPath == "" {
		// Try any file matching the exe name (case-insensitive on Windows)
		for _, f := range r.File {
			base := strings.ToLower(filepath.Base(f.Name))
			if base == strings.ToLower(exeName) {
				outPath := filepath.Join(targetDir, exeName)
				if err := extractCoreFile(f, outPath); err != nil {
					return "", err
				}
				_ = os.Chmod(outPath, 0o755)
				foundPath = outPath
				break
			}
		}
	}
	if foundPath == "" {
		return "", fmt.Errorf("%s executable not found in zip", exeName)
	}
	return foundPath, nil
}

func extractCoreFile(f *zip.File, outPath string) error {
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

// pickCoreAsset selects the right asset for the current OS/arch.
func pickCoreAsset(rel *ghRelease, repo CoreRepo) (*ghAsset, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Build candidate suffixes per core kind
	suffixes := []string{}
	switch repo.Kind {
	case models.CoreKindSingBox:
		suffixes = append(suffixes, fmt.Sprintf("-%s-%s.zip", goos, goarch))
	case models.CoreKindXray:
		// Xray: Xray-windows-64.zip, Xray-linux-arm64-v8a.zip
		arch := goarch
		if goos == "windows" {
			if goarch == "amd64" {
				arch = "64"
			} else {
				arch = "32"
			}
		}
		suffixes = append(suffixes, fmt.Sprintf("-%s-%s.zip", goos, arch))
		if goos == "linux" && goarch == "arm64" {
			suffixes = append(suffixes, "-linux-arm64-v8a.zip")
		}
	case models.CoreKindMihomo:
		// mihomo: mihomo-windows-amd64-compatible-v0.15.0.zip, mihomo-linux-arm64-v0.15.0.gz
		suffixes = append(suffixes, fmt.Sprintf("-%s-%s-", goos, goarch))
		suffixes = append(suffixes, fmt.Sprintf("-%s-%s.", goos, goarch))
	case models.CoreKindHysteria2:
		// hysteria: hysteria-windows-amd64.exe, hysteria-linux-arm64
		ext := ".zip"
		if goos == "windows" {
			suffixes = append(suffixes, fmt.Sprintf("-%s-%s", goos, goarch)+ext)
			suffixes = append(suffixes, fmt.Sprintf("-%s-%s", goos, "x86_64")+ext)
		} else {
			suffixes = append(suffixes, fmt.Sprintf("-%s-%s", goos, goarch)+ext)
		}
	default:
		suffixes = append(suffixes, fmt.Sprintf("-%s-%s.zip", goos, goarch))
	}

	for _, asset := range rel.Assets {
		for _, sfx := range suffixes {
			if strings.Contains(asset.Name, sfx) {
				return &asset, nil
			}
		}
	}

	// Fallback: try a generic pattern
	for _, asset := range rel.Assets {
		if strings.Contains(asset.Name, goos) && strings.Contains(asset.Name, goarch) {
			return &asset, nil
		}
	}

	return nil, fmt.Errorf("no asset for %s/%s in release %s (%s)", goos, goarch, rel.TagName, repo.Kind)
}

// customCoreChain builds download URL candidates for a given asset + core kind.
func customCoreChain(asset *ghAsset, customPrefixes []string, kind string) []struct {
	url    string
	source string
} {
	out := make([]struct {
		url    string
		source string
	}, 0, len(customPrefixes)+4)

	// User custom mirrors first
	for _, p := range customPrefixes {
		out = append(out, struct {
			url    string
			source string
		}{p + asset.BrowserDownloadURL, "custom"})
	}

	// Standard mirrors
	out = append(out,
		struct {
			url    string
			source string
		}{"https://ghfast.top/" + asset.BrowserDownloadURL, "ghfast.top"},
		struct {
			url    string
			source string
		}{"https://ghproxy.net/" + asset.BrowserDownloadURL, "ghproxy.net"},
		struct {
			url    string
			source string
		}{"https://gh-proxy.com/" + asset.BrowserDownloadURL, "gh-proxy.com"},
		struct {
			url    string
			source string
		}{asset.BrowserDownloadURL, "github"},
	)
	return out
}
