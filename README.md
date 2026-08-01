# sbpanel

sing-box 的本地可视化管理面板 —— Go 后端 + Vue 3 前端，单二进制交付。功能对齐 v2rayN 核心体验：多协议节点、代理分组、运行时切换、路由规则、实时流量、订阅管理。

> **内核是 sing-box**（SagerNet 出品，独立项目）。sbpanel 只是个管理壳：生成配置、启停进程、通过 Clash API 读取实时数据。把 sbpanel 关了，sing-box 还能独立跑。

---

## 一、快速开始（用户）

### 方式 A：直接运行（推荐）

双击 **`启动 sbpanel.bat`**，浏览器自动打开 `http://127.0.0.1:7820`。

如果端口被占用，会自动换到 7821/7822...（控制台窗口会打印实际地址）。

### 方式 B：命令行

```bat
bin\sbpanel.exe                  :: 默认 7820 端口，自动开浏览器
bin\sbpanel.exe -port 9000       :: 指定端口
bin\sbpanel.exe -no-browser      :: 不自动开浏览器
```

关闭：关掉控制台窗口，或按 `Ctrl+C`。

---

## 二、第一次使用：5 步上网

### 1. 导入节点
左侧 **🌐 节点** → 点「导入节点」→ 粘贴分享链接，每行一个：

```
vless://670a39ac-ffb4-46ed-b2e2-7087dce3bebb@example.com:443?type=tcp&security=reality&sni=www.apple.com&fp=chrome&pbk=xxx&sid=xxx&flow=xtls-rprx-vision#我的节点
vmess://eyJ2IjoiMiIs...
trojan://password@server:443#名字
ss://...
hysteria2://...
tuic://...
```

也支持粘贴 **sing-box JSON 配置**、**Clash YAML**、**整段 base64 订阅**。导入后第一个节点会自动选中。

### 2.（可选）建代理分组
左侧 **📂 分组** →「新建分组」→ 选类型：

| 类型 | 行为 |
|---|---|
| **手动选择 (selector)** | 你点哪个用哪个 |
| **自动选最优 (url_test)** | 持续测速，自动用延迟最低的 |
| **故障转移 (fallback)** | 按顺序，第一个可用就用 |

把节点加进分组。分组是 v2rayN 的核心能力——**核心运行时可在组内秒级切换，无需重启**。

### 3.（可选）配路由规则
左侧 **🛣 路由** →「新建规则」。例如：

| 类型 | 值 | 出站 | 效果 |
|---|---|---|---|
| 域名后缀 | `github.com` | 走代理 | GitHub 走代理 |
| IP/CIDR | `192.168.0.0/16` | 直连 | 内网直连 |
| 进程名 | `xxx.exe` | 阻断 | 拦截该进程 |

规则从上到下匹配，命中即生效。下方「规则集」可开关内置的 geosite-cn / !cn。

### 4. 启动
左侧 **🏠 概览** → 点「▶ 启动」。状态点变绿 = sing-box 在跑。

> AI/TUN 模式（接管全局流量）需要管理员权限：右键 `启动 sbpanel.bat` → 以管理员身份运行。

### 5. 开系统代理（让浏览器走代理）
概览页「系统代理」→ 点「开启」。这会设置 Windows 系统代理为 `127.0.0.1:20808`，浏览器/支持系统代理的应用即走代理。

不用了点「关闭」。

---

## 三、各页面说明

| 页面 | 干什么 |
|---|---|
| 🏠 **概览** | 启停核心、系统代理开关、实时流量图、当前生效节点/分组 |
| 🌐 **节点** | 导入/编辑/删除/测速节点，点选当前节点 |
| 📂 **分组** | 建 selector/url_test/fallback 组，运行时切换成员 |
| 🛣 **路由** | 自定义路由规则 + 规则集开关 |
| 📡 **订阅** | 添加订阅 URL，定时自动刷新，合并去重 |
| 📜 **日志** | sing-box 实时日志（SSE 流），按级别着色 |
| ⚙ **设置** | 主题（深/浅）、语言（中/英）、端口、延迟测试 URL 等 |

---

## 四、常见问题

**Q：启动后状态点不变绿？**
看「📜 日志」页的报错。常见：节点配置无效、端口被占用、TUN 模式没管理员权限。

**Q：改了节点/分组/路由不生效？**
分组切换是实时的（不用重启）。但**改节点配置或路由规则后需要重启核心**（概览页点「↻ 重启」）。

**Q：数据存在哪？**
`%APPDATA%\sbpanel\data\sbpanel.db`（SQLite）。删掉这个文件等于重置。

**Q：怎么完全卸载？**
删 `bin\sbpanel.exe` + `%APPDATA%\sbpanel\` 目录。sing-box.exe 和 .srs 规则集是独立的，可单独保留。

**Q：和旧版 Python 面板什么关系？**
旧版（`panel/` 目录）已删除，功能全部移植到 Go 版。数据不兼容（旧 JSON → 新 SQLite），需重新导入节点。

---

## 五、开发者

### 技术栈
- **后端**：Go 1.22+ · chi 路由 · modernc.org/sqlite（纯 Go 无 CGO）· gorilla/websocket
- **前端**：Vite 5 · Vue 3.4 · TypeScript · Element Plus · Pinia · Vue Router · vue-i18n · ECharts
- **内核**：sing-box（独立二进制，由 sbpanel 作为子进程管理）

### 项目结构
```
sbpanel/
├── cmd/panel/              Go 入口
├── internal/
│   ├── api/                HTTP/SSE 层（chi 路由，40+ 接口）
│   ├── core/               sing-box 进程管理 + Clash API 客户端 + 配置生成
│   ├── protocol/           6 协议插件（vless/vmess/trojan/ss/hy2/tuic），注册式扩展
│   ├── routing/            路由规则编译引擎（含单测）
│   ├── import_/            多格式导入
│   ├── subscription/       订阅抓取/合并
│   ├── latency/            穿代理测速
│   ├── sysproxy/           跨平台系统代理（Win/Mac/Linux）
│   ├── store/sqlite/       持久化（文档模式）
│   ├── models/             领域模型
│   └── web/                go:embed 前端
├── frontend/               Vite 工程（构建产物输出到 internal/web/dist）
├── sing-box.exe            sing-box 内核（独立）
└── geosite-*.srs           规则集
```

### 从源码构建
```bat
:: 1. 构建前端
cd frontend
npm install --legacy-peer-deps
npm run build            :: 产物输出到 internal/web/dist

:: 2. 构建后端（含前端）
cd ..
go build -o bin\sbpanel.exe .\cmd\panel

:: 3. 前端开发模式（热更新，代理 /api 到 7820）
cd frontend && npm run dev   :: 另开终端跑 bin\sbpanel.exe
```

### 扩展点
- **新协议**：`internal/protocol/<name>/` 实现 `Protocol` 接口 + `init()` 注册
- **新路由规则类型**：`internal/routing/routing.go` 加分支
- **新 API**：`internal/api/handlers_*.go` 加 handler + `server.go` 注册路由

### 测试
```bat
go test .\internal\routing\...    :: 路由编译引擎单测
```

---

## 六、已知限制（诚实说明）

- 这是 **MVP 骨架**，不是商用成品：零集成测试、无自动更新、无崩溃恢复
- 仅 Windows 实测；macOS/Linux 系统代理代码写了但未真机验证
- 无托盘图标、无开机自启（计划中）
- 节点协议字段编辑有限（改名/地址/端口可改，协议参数需重新导入）
- 多 profile（多套配置档案）后端已就绪，前端 UI 未做
