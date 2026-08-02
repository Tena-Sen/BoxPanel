---
AIGC:
  ContentProducer: '001191110102MAD55U9H0F10002'
  ContentPropagator: '001191110102MAD55U9H0F10002'
  Label: '1'
  ProduceID: 'ac2dfd71-0fb0-4241-824e-a702739e0b33'
  PropagateID: 'ac2dfd71-0fb0-4241-824e-a702739e0b33'
  ReservedCode1: '5bfbae09-53e8-4c62-99c9-d78527a3756a'
  ReservedCode2: '5bfbae09-53e8-4c62-99c9-d78527a3756a'
---

# BoxPanel — 多内核代理管理面板

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![Vue 3](https://img.shields.io/badge/Vue-3-4FC08D?logo=vue.js)](https://vuejs.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**BoxPanel** 是一个开箱即用的多内核代理管理面板，Go 后端 + Vue 3 前端，单二进制交付。借鉴 v2rayN 核心架构，支持 **sing-box / Xray / mihomo / Hysteria2** 四大内核引擎，根据节点协议自动选择最兼容的内核，无需手动切换。

> English description below ↓

---

## 为什么选 BoxPanel？

| 痛点 | BoxPanel 方案 |
|---|---|
| 不同节点需要不同内核 | **多内核引擎**：4 种内核自动切换，xhttp 节点自动下载 Xray |
| 切节点要重启 | **运行时切换**：分组内秒级切节点，核心不重启 |
| 不知道哪个内核兼容 | **NodeValidator 前置校验**：启动前自动检测，不兼容降级为 block 而非阻断 |
| 管理工具太重 | **单 exe 交付**：无运行时依赖，双击即用，前端 go:embed 打包 |

---

## 核心特性

- **多内核引擎** — sing-box（全协议）/ Xray（vless/vmess/trojan/ss + xhttp）/ mihomo（hy2/tuic）/ Hysteria2，自动匹配最佳内核
- **6 种协议** — VLESS / VMess / Trojan / Shadowsocks / Hysteria2 / TUIC，支持 Reality / XTLS
- **运行时分组** — selector 手动选择 / url_test 自动测速 / fallback 故障转移，切换无需重启
- **路由规则** — 域名 / IP / 进程名规则 + geosite/geoip 规则集开关
- **订阅管理** — URL 导入、定时刷新、合并去重
- **系统代理** — 一键开关，内核退出自动关闭防断网
- **代理模式** — 规则模式 / 全局模式 / AI(TUN) 模式
- **实时监控** — 流量图表、节点延迟、内核日志
- **多格式导入** — 分享链接 / sing-box JSON / Clash YAML / base64 订阅
- **节点导出** — 批量导出 + 二维码分享

---

## 兼容性矩阵

| 协议 / 传输 | sing-box | Xray | mihomo | Hysteria2 |
|---|:---:|:---:|:---:|:---:|
| VLESS + Reality | ✓ | ✓ | - | - |
| VMess | ✓ | ✓ | ✓ | - |
| Trojan | ✓ | ✓ | ✓ | - |
| Shadowsocks | ✓ | ✓ | ✓ | - |
| Hysteria2 | ✓ | - | ✓ | ✓ |
| TUIC | ✓ | - | ✓ | - |
| WebSocket | ✓ | ✓ | ✓ | - |
| gRPC | ✓ | ✓ | - | - |
| HTTP/2 / h2 | ✓ | ✓ | - | - |
| HTTPUpgrade | ✓ | ✓ | - | - |
| XHTTP / SplitHTTP | ✗ | ✓ | ✗ | ✗ |

---

## 快速开始

### 方式 A：直接运行（推荐）

双击 **`启动 BoxPanel.bat`**，浏览器自动打开 `http://127.0.0.1:7820`。

端口被占用时自动换到 7821/7822...（控制台窗口会打印实际地址）。

### 方式 B：命令行

```bat
boxpanel.exe                     :: 默认 7820 端口，自动开浏览器
boxpanel.exe -port 9000          :: 指定端口
boxpanel.exe -no-browser         :: 不自动开浏览器
```

关闭：关掉控制台窗口，或 `Ctrl+C`。

---

## 5 步上网

### 1. 导入节点
左侧 **节点** →「导入节点」→ 粘贴分享链接，每行一个：

```
vless://uuid@example.com:443?type=tcp&security=reality&sni=www.apple.com&fp=chrome&pbk=xxx&sid=xxx&flow=xtls-rprx-vision#我的节点
vmess://eyJ2IjoiMiIs...
trojan://password@server:443#名字
ss://...
hysteria2://...
tuic://...
```

也支持粘贴 **sing-box JSON 配置**、**Clash YAML**、**整段 base64 订阅**。

### 2.（可选）建代理分组
左侧 **分组** →「新建分组」→ 选类型：

| 类型 | 行为 |
|---|---|
| **手动选择 (selector)** | 你点哪个用哪个 |
| **自动选最优 (url_test)** | 持续测速，自动用延迟最低的 |
| **故障转移 (fallback)** | 按顺序，第一个可用就用 |

### 3.（可选）配路由规则
左侧 **路由** →「新建规则」。例如：

| 类型 | 值 | 出站 | 效果 |
|---|---|---|---|
| 域名后缀 | `github.com` | 走代理 | GitHub 走代理 |
| IP/CIDR | `192.168.0.0/16` | 直连 | 内网直连 |
| 进程名 | `xxx.exe` | 阻断 | 拦截该进程 |

### 4. 启动
左侧 **概览** → 点「启动」。状态点变绿 = 内核在跑。

> AI/TUN 模式（接管全局流量）需管理员权限：右键 bat → 以管理员身份运行。

### 5. 开系统代理
概览页「系统代理」→ 点「开启」，浏览器即走代理。不用了点「关闭」。

---

## 各页面说明

| 页面 | 功能 |
|---|---|
| **概览** | 启停核心、系统代理开关、实时流量图、当前节点/分组 |
| **节点** | 导入/编辑/删除/测速节点，点选当前节点，兼容性徽章 |
| **分组** | selector / url_test / fallback 组，运行时切换 |
| **路由** | 自定义路由规则 + 规则集开关 |
| **订阅** | 添加订阅 URL，定时刷新，合并去重 |
| **内核** | 多内核管理：下载/添加/删除/切换/探测 |
| **日志** | 内核实时日志（SSE 流），按级别着色 |
| **设置** | 主题（深/浅）、语言（中/英）、端口、延迟测试 URL 等 |

---

## 多内核架构（v2rayN 路线）

BoxPanel 借鉴 v2rayN 多内核架构，核心设计：

| 概念 | 说明 |
|---|---|
| **CoreInfo 元数据注册表** | 每种内核的协议/传输兼容性、下载地址等均为数据驱动，新增内核 = 追加一条记录 |
| **NodeValidator 前置校验** | 启动前检查节点兼容性，不兼容时自动切换到兼容内核，而非硬拦截 |
| **candidateCores 排序** | 按 NodeValidator 评分排序：兼容=0 > 警告=1 > 不兼容=2 |
| **自动下载** | 缺少兼容内核时自动从 GitHub 下载并注册 |
| **版本适配** | configgen 按目标内核版本生成对应格式 |

---

## 常见问题

**Q：启动后状态点不变绿？**
看「日志」页报错。常见：节点配置无效、端口被占用、TUN 模式没管理员权限。

**Q：改了节点/分组/路由不生效？**
分组切换是实时的。改节点配置或路由规则后需重启核心（概览页「重启」）。

**Q：xhttp 节点启动失败？**
xhttp/splithttp 是 Xray 独有传输，sing-box 不支持。BoxPanel 会自动下载 Xray 并切换。

**Q：数据存在哪？**
`data/boxpanel.db`（SQLite）。删掉 = 重置。

**Q：怎么完全卸载？**
删 `boxpanel.exe` + `data/` 目录。

---

## 开发者

### 技术栈
- **后端**：Go 1.22+ · chi · modernc.org/sqlite（纯 Go 无 CGO）· gorilla/websocket
- **前端**：Vite 5 · Vue 3 · TypeScript · Element Plus · Pinia · vue-i18n · ECharts
- **内核**：sing-box / Xray / mihomo / Hysteria2（独立二进制，子进程管理）

### 项目结构
```
BoxPanel/
├── cmd/panel/              Go 入口
├── internal/
│   ├── api/                HTTP/SSE 层（chi，40+ 接口）
│   ├── core/               多内核进程管理 + Clash API + 配置生成
│   │   ├── configgen/      sing-box 配置生成 + 版本适配器
│   │   ├── xray/           Xray 内核适配
│   │   ├── mihomo/         mihomo 内核适配
│   │   └── hysteria2/      Hysteria2 内核适配
│   ├── coreinfo/           CoreInfo 元数据注册表
│   ├── nodevalidator/      NodeValidator 前置校验
│   ├── protocol/           6 协议插件（vless/vmess/trojan/ss/hy2/tuic）
│   ├── routing/            路由规则编译引擎
│   ├── coredl/             多内核下载器（GitHub + 镜像回退 + 断点续传）
│   ├── import_/            多格式导入
│   ├── subscription/       订阅抓取/合并
│   ├── latency/            穿代理测速
│   ├── readyprobe/         SOCKS5 握手探测
│   ├── sysproxy/           跨平台系统代理
│   ├── store/sqlite/       持久化
│   ├── models/             领域模型
│   └── web/                go:embed 前端
├── frontend/               Vite 工程
└── data/                   运行时数据（db + 内核 + 规则集）
```

### 从源码构建
```bat
:: 1. 构建前端
cd frontend
npm install --legacy-peer-deps
npm run build            :: 产物 → internal/web/dist

:: 2. 构建后端（含前端）
cd ..
go build -o boxpanel.exe .\cmd\panel

:: 3. 前端开发模式（热更新，代理 /api 到 7820）
cd frontend && npm run dev   :: 另开终端跑 boxpanel.exe
```

### 扩展点
- **新协议**：`internal/protocol/<name>/` 实现 `Protocol` 接口 + `init()` 注册
- **新内核**：`internal/core/<name>/` 实现 `Core` 接口 + `coreinfo.Registry` 追加元数据
- **新路由规则类型**：`internal/routing/routing.go` 加分支
- **新 API**：`internal/api/handlers_*.go` 加 handler + `server.go` 注册路由

### 测试
```bat
go test .\internal\routing\...
```

---

## English

**BoxPanel** is a multi-kernel proxy management panel (Go + Vue 3, single binary). Inspired by v2rayN architecture, it supports **sing-box / Xray / mihomo / Hysteria2** kernel engines with automatic kernel switching based on node protocol.

### Key Features
- **Multi-kernel engine** — Auto-switch between sing-box, Xray, mihomo, Hysteria2 based on node compatibility
- **6 protocols** — VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC with Reality/XTLS support
- **Runtime group switching** — selector / url_test / fallback groups, switch without kernel restart
- **Routing rules** — Domain / IP / process rules + geosite/geoip rule-set toggles
- **Subscription management** — URL import, scheduled refresh, deduplication
- **System proxy** — One-click toggle with auto-cleanup on kernel exit
- **Proxy modes** — Rule / Global / AI(TUN)
- **Real-time monitoring** — Traffic charts, latency badges, kernel logs
- **Multi-format import** — Share links / sing-box JSON / Clash YAML / base64 subscription
- **Node export** — Batch export + QR code sharing

### Quick Start
```bat
:: Download and run
boxpanel.exe                    :: Default port 7820, auto-open browser
boxpanel.exe -port 9000         :: Custom port
boxpanel.exe -no-browser        :: Don't open browser
```

### Build from Source
```bat
cd frontend && npm install --legacy-peer-deps && npm run build
cd .. && go build -o boxpanel.exe ./cmd/panel
```

### Keywords (for search discovery)
`proxy manager` `proxy panel` `multi-kernel proxy` `sing-box gui` `xray gui` `mihomo gui` `v2rayN alternative` `clash alternative` `VLESS` `VMess` `Trojan` `Shadowsocks` `Hysteria2` `TUIC` `Reality` `XHTTP` `subscription manager` `system proxy` `代理管理` `代理面板` `多内核代理` `网络工具` `节点管理` `订阅管理` `流量管理`

> AI生成