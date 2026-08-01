// Package core 抽象多"代理核心"接口。
//
// 设计目标：支持多种内核（sing-box / xray / ...）并存，
// 节点根据协议自动选/下载/切换到兼容的内核。
//
// 关键概念：
//   - Core：一个可执行代理核心（sing-box.exe / xray.exe）
//   - ClashAPI：核心暴露的"控制面"（sing-box 有，xray 无）
//   - BuildRequest：BoxPanel 内部的统一"要启动什么节点"的描述
//   - 每个 Core 负责：把 BuildRequest 翻译成自己的 config schema + 启动子进程
package core

import (
	"context"

	"boxpanel/internal/models"
)

// Core 抽象一个"代理核心"实现（sing-box / xray / mihomo / hysteria2）。
//
// 关键方法：
//   - BuildConfig：把统一 BuildRequest 翻译成该核心的 config schema
//   - Start / Stop / IsRunning：子进程管理
//   - Check：验证生成的 config 语法（启动前）
//   - ClashAPI：返回控制面客户端（xray/hysteria2 可能返回 nil）
//   - SupportsProtocol：判断该核心是否支持某协议
type Core interface {
	// 标识
	Name() string                                    // "sing-box" / "Xray" / "mihomo" / "Hysteria2"
	Kind() string                                    // 简短标识 "singbox" / "xray" / "mihomo" / "hysteria2"
	ExePath() string
	SetExePath(p string)

	// 配置生成
	BuildConfig(ctx context.Context, req BuildRequest, outPath string) error

	// 子进程
	Start(ctx context.Context, configPath string) error
	Stop() error
	IsRunning() bool
	PID() int

	// 校验（启动前）
	Check(ctx context.Context, configPath string) error

	// Clash API（xray/hysteria2 返回 nil）
	ClashAPI() ClashAPI

	// 协议支持
	SupportsProtocol(proto string) bool
}

// ClashAPI 是核心暴露的 Clash 风格控制面（用于分组切换、测速、流量等）。
type ClashAPI interface {
	Proxies(ctx context.Context) (map[string]any, error)
	SelectProxy(ctx context.Context, group, name string) error
	Delay(ctx context.Context, name, url string, timeoutMs int) (int, error)
	Connections(ctx context.Context) (any, error)
	Reachable(ctx context.Context) bool
}

// BuildRequest 是 BoxPanel 内部"要启动什么"的统一描述。
// 每个 Core 负责把它翻译成自己的 config schema。
type BuildRequest struct {
	Profile       models.Profile
	CurrentServer models.Server
	AllServers    []models.Server
	Groups        []models.Group
	RoutingRules  []models.RoutingRule
	RuleSets      []models.RuleSet
	Settings      models.Settings
}

// Info 描述已安装的 Core 实例。
type Info struct {
	Name    string `json:"name"`     // "sing-box" / "xray"
	Path    string `json:"path"`     // 完整 exe 路径
	Version string `json:"version"`  // 探测到的版本
	Active  bool   `json:"active"`   // 是否当前激活
}
