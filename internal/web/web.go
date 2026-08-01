// Package web embeds the built Vue frontend (Vite dist) into the Go binary.
//
// go:embed 不支持 ".." 路径，故 dist 位于本包目录下。
// Vite 构建输出目录配置为 internal/web/dist（见 frontend/vite.config.ts）。
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler serving the SPA.
// SPA 回退：未匹配静态文件时返回 index.html（交给前端路由）。
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// 静态文件不存在 -> 回退 index.html（SPA 路由）
		if _, err := fs.Stat(sub, path); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
