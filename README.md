---
AIGC:
  ContentProducer: '001191110102MAD55U9H0F10002'
  ContentPropagator: '001191110102MAD55U9H0F10002'
  Label: '1'
  ProduceID: '23b39c66-221a-4392-bcb3-61687e271ca6'
  PropagateID: '23b39c66-221a-4392-bcb3-61687e271ca6'
  ReservedCode1: '48499d70-1156-4d48-8b0a-b3446f61e7e4'
  ReservedCode2: '48499d70-1156-4d48-8b0a-b3446f61e7e4'
---

# BoxPanel

多内核代理管理面板 —— Go 后端 + Vue 3 前端，单二进制交付。借鉴 v2rayN 核心架构：多协议节点、多内核引擎自动切换、代理分组、运行时切换、路由规则、实时流量、订阅管理。

> **支持 4 种内核引擎**：sing-box（全协议）/ Xray（vless/vmess/trojan/ss + xhttp）/ mihomo（ss/vmess/trojan/hysteria2/tuic）/ Hysteria2（hy2 专用加速）。启动时根据节点协议/传输类型自动选择最兼容的内核，无需手动切换。

---

## 一、快速开始（用户）

### 方式 A：直接运行（推荐）

双击 **`启动 BoxPanel.bat`**，浏览器自动打开 `http://127.0.0.1:7820`。

如果端口被占用，会自动换到 7821/7822...（控制台窗口会打印实际地址）。

### 方式 B：命令行

```bat
boxpanel.exe                     :: 默认 7820 端口，自动开浏览器
boxpanel.exe -port 9000          :: 指定端口
boxpanel.exe -no-browser         :: 不自动开浏览器
```

关闭：关掉控制台窗口，或按 `Ctrl+C`。

---

## 二、第一次使用：5 步上网

### 1. 导入节点
左侧 **节点** → 点「导入节点」→ 粘贴分享链接，每行一个：

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
左侧 **分组** →「新建分组」→ 选类型：

| 类型 | 行为 |
|---|---|
| **手动选择 (selector)** | 你点哪个用哪个 |
| **自动选最优 (url_test)** | 持续测速，自动用延迟最低的 |
| **故障转移 (fallback)** | 按顺序，第一个可用就用 |

把节点加进分组。分组是 v2rayN 的核心能力——**核心运行时可在组内秒级切换，无需重启**。

### 3.（可选）配路由规则
左侧 **路由** →「新建规则」。例如：

| 类型 | 值 | 出站 | 效果 |
|---|---|---|---|
| 域名后缀 | `github.com` | 走代理 | GitHub 走代理 |
| IP/CIDR | `192.168.0.0/16` | 直连 | 内网直连 |
| 进程名 | `xxx.exe` | 阻断 | 拦截该进程 |

规则从上到下匹配，命中即生效。下方「规则集」可开关内置的 geosite-cn / !cn。

### 4. 启动
左侧 **概览** → 点「启动」。状态点变绿 = 内核在跑。

> AI/TUN 模式（接管全局流量）需要管理员权限：右键 `启动 BoxPanel.bat` → 以管理员身份运行。

### 5. 开系统代理（让浏览器走代理）
概览页「系统代理」→ 点「开启」。这会设置 Windows 系统代理为 `127.0.0.1:20808`，浏览器/支持系统代理的应用即走代理。

不用了点「关闭」。

---

## 三、各页面说明

| 页面 | 干什么 |
|---|---|
| **概览** | 启停核心、系统代理开关、实时流量图、当前生效节点/分组 |
| **节点** | 导入/编辑/删除/测速节点，点选当前节点，兼容性徽章提示 |
| **分组** | 建 selector/url_test/fallback 组，运行时切换成员 |
| **路由** | 自定义路由规则 + 规则集开关 |
| **订阅** | 添加订阅 URL，定时自动刷新，合并去重 |
| **内核** | 多内核管理：下载/添加/删除/切换，自动匹配最佳内核 |
| **日志** | 内核实时日志（SSE 流），按级别着色 |
| **设置** | 主题（深/浅）、语言（中/英）、端口、延迟测试 URL 等 |

---

## 四、多内核引擎（v2rayN 架构）

BoxPanel 借鉴 v2rayN 的多内核架构，核心设计：

| 概念 | 说明 |
|---|---|
| **CoreInfo 元数据注册表** | 每种内核的协议/传输兼容性、启动参数、下载地址等均为数据驱动，新增内核 = 追加一条记录 |
| **NodeValidator 前置校验** | 启动前检查节点协议+传输与内核的兼容性，不兼容时自动切换到兼容内核 |
| **candidateCores 排序** | 按 NodeValidator 评分排序内核：兼容=0 > 警告=1 > 不兼容=2，优先用最好的 |
| **自动下载** | 启动时发现缺少兼容内核，自动从 GitHub 下载并注册 |
| **版本适配** | configgen 按目标内核版本生成对应格式，自动处理字段差异 |

### 兼容性矩阵

| 传输类型 | sing-box | Xray | mihomo | Hysteria2 |
|---|---|---|---|---|
| TCP / raw | ✓ | ✓ | ✓ | - |
| WebSocket | ✓ | ✓ | ✓ | - |
| gRPC | ✓ | ✓ | - | - |
| HTTP/2 | ✓ | ✓ | - | - |
| HTTPUpgrade | ✓ | ✓ | - | - |
| xhttp / splithttp | ✗ | ✓ | ✗ | ✗ |
| kcp | ✗ | ✓ | ✗ | ✗ |

---

## 五、常见问题

**Q：启动后状态点不变绿？**
看「日志」页的报错。常见：节点配置无效、端口被占用、TUN 模式没管理员权限。

**Q：改了节点/分组/路由不生效？**
分组切换是实时的（不用重启）。但**改节点配置或路由规则后需要重启核心**（概览页点「重启」）。

**Q：xhttp 节点启动失败？**
xhttp/splithttp 是 Xray 独有传输，sing-box 不支持。BoxPanel 会自动下载 Xray 内核并切换，无需手动操作。

**Q：数据存在哪？**
`data/boxpanel.db`（SQLite）。删掉这个文件等于重置。

**Q：怎么完全卸载？**
删 `boxpanel.exe` + `data/` 目录。内核和规则集是独立的，可单独保留。

---

## 六、开发者

### 技术栈
- **后端**：Go 1.22+ · chi 路由 · modernc.org/sqlite（纯 Go 无 CGO）· gorilla/websocket
- **前端**：Vite 5 · Vue 3.4 · TypeScript · Element Plus · Pinia · Vue Router · vue-i18n · ECharts
- **内核**：sing-box / Xray / mihomo / Hysteria2（独立二进制，由 BoxPanel 作为子进程管理）

### 项目结构
```
BoxPanel/
├── cmd/panel/              Go 入口
├── internal/
│   ├── api/                HTTP/SSE 层（chi 路由，40+ 接口）
│   ├── core/               多内核进程管理 + Clash API 客户端 + 配置生成
│   │   ├── configgen/      sing-box 配置生成 + 版本 schema 适配器
│   │   ├── xray/           Xray 内核适配
│   │   ├── mihomo/         mihomo 内核适配
│   │   └── hysteria2/      Hysteria2 内核适配
│   ├── coreinfo/           CoreInfo 元数据注册表（数据驱动，零代码扩展）
│   ├── nodevalidator/      NodeValidator 前置校验
│   ├── protocol/           6 协议插件（vless/vmess/trojan/ss/hy2/tuic），注册式扩展
│   ├── routing/            路由规则编译引擎（含单测）
│   ├── coredl/             多内核下载器（GitHub + 镜像回退 + 断点续传）
│   ├── import_/            多格式导入
│   ├── subscription/       订阅抓取/合并
│   ├── latency/            穿代理测速
│   ├── readyprobe/         SOCKS5 握手探测
│   ├── sysproxy/           跨平台系统代理（Win/Mac/Linux）
│   ├── store/sqlite/       持久化（文档模式）
│   ├── models/             领域模型
│   └── web/                go:embed 前端
├── frontend/               Vite 工程（构建产物输出到 internal/web/dist）
└── data/                   运行时数据（数据库 + 内核二进制 + 规则集）
```

### 从源码构建
```bat
:: 1. 构建前端
cd frontend
npm install --legacy-peer-deps
npm run build            :: 产物输出到 internal/web/dist

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
go test .\internal\routing\...    :: 路由编译引擎单测
```

> AI生成