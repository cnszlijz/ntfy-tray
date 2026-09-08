# ntfy-tray 技术方案

## 目标

Windows 常驻托盘程序：订阅 ntfy.sh topic，收到消息时弹出 Windows toast 通知。

## 技术栈

| 职责 | 选型 | 理由 |
|---|---|---|
| 语言 | Go 1.27 | 单文件 `.exe`，无运行时依赖；`-ldflags -H=windowsgui` 无控制台窗口 |
| 托盘图标 | `github.com/getlantern/systray` | Windows 托盘事实标准，支持菜单/点击事件 |
| Toast 通知 | `gopkg.in/toast.v1` | 未打包 Win32 程序弹 toast 需 AppUserModelID，该库自动创建带 AUMID 的开始菜单快捷方式，免去注册表操作 |
| ntfy 订阅 | 原生 HTTP 长连接 | 无第三方依赖 |

## 核心设计

### ntfy 流式订阅

- `GET {server}/{topics}/json?since=all` 返回 NDJSON 长流，每行一个 JSON 事件。
- 多 topic 逗号合并到**一条连接**。
- `since=all` 跳过服务端缓存的历史消息，只收新消息。
- 事件类型：`open` / `keepalive` / `message` / `poll_request`，仅 `message` 触发通知。
- 服务端每 ~45s 发 `keepalive` → **watchdog 90s 无帧即判定死连接**，主动重连。
- 断线重连：指数退避 1s → 2s → … → 上限 30s。
- 受保护 topic 用 `Authorization: Bearer tk_...` 头。

### Toast 行为

- 标题：消息 `title`，缺省回退为 topic 名。
- 点击 toast / "Open" 按钮：打开消息 `click` URL，缺省回退为 `https://ntfy.sh/{topic}`。
- `priority >= 4`（high/urgent）使用循环警报音。

### 托盘

- 图标：`icon.ico`（32×32，程序内 `go:embed`，生成脚本见下文）。
- 菜单：`Open ntfy`（浏览器打开 topic 页面）、`Quit`。

## 运行参数

```
ntfy-tray.exe -topics=mytopic,alerts [-server=https://ntfy.sh] [-token=tk_xxx]
```

## 构建

```
go build -ldflags "-H=windowsgui" -o ntfy-tray.exe .
```

注意：本机网络无法访问 sum.golang.org，已 `go env -w GOSUMDB=off`。

## 状态

- [x] main.go（流订阅 / 重连 / watchdog / toast / 托盘）
- [x] icon.ico 生成
- [x] go.mod 依赖（systray v1.2.2, toast.v1）
- [x] 修复 `openBrowser` 未定义，编译通过（`ntfy-tray.exe`，windowsgui 无窗口）
- [x] 端到端验证：启动后订阅 `ntfy-tray-smoke-9f3k2`，发布消息，toast 实时弹出，进程 37s 无重启（流稳定）
- [ ] 开机自启（注册表 `HKCU\...\Run` 或启动文件夹快捷方式，待决定是否内置）
