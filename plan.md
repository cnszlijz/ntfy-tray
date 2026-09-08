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

- `GET {server}/{topics}/json?since=...` 返回 NDJSON 长流，每行一个 JSON 事件。
- 多 topic 逗号合并到**一条连接**。
- `since` 取值见「断线续传」：无状态时为 `all`，只收新消息。
- 事件类型：`open` / `keepalive` / `message` / `poll_request`，仅 `message` 触发通知。
- 服务端每 ~45s 发 `keepalive` → **watchdog 90s 无帧即判定死连接**，主动重连。
- 断线重连：指数退避 1s → 2s → … → 上限 30s。
- 受保护 topic 用 `Authorization: Bearer tk_...` 头。

### 断线续传（state.go）

- `ntfy-tray-state.json` 存于 exe 同目录：`{"topics": {"<topic>": <unix 秒>}}`，记录每个 topic 最近一条已展示消息的时间戳；写盘用 tmp+rename 原子替换。
- 启动时 `since` 取所有 topic 水印的**最小值**（服务端补发历史），客户端按 `msg.Time <= 水印` 逐 topic 去重，仅展示上次更新后的通知。
- 无状态的新 topic 以程序启动时间为水印，只收新消息。
- 已知边界：水印精度为秒，与最后一条消息同秒发布的离线消息会被视为已读（ntfy 流不支持多 topic 分别指定 message ID，可接受）。

### Toast 行为

- 标题：消息 `title`，缺省回退为 topic 名。
- 点击 toast / "Open" 按钮：打开消息 `click` URL，缺省回退为 `https://ntfy.sh/{topic}`。
- `priority >= 4`（high/urgent）使用循环警报音。

### 托盘

- 图标：`icon.ico`（32×32，程序内 `go:embed`）。
- 菜单：`Open ntfy`（浏览器打开 topic 页面）、`Quit`。
- **闪烁**：有未确认通知时图标以 500ms 间隔在 `icon.ico` / `icon_blank.ico` 间切换。**左键单击仅消除闪烁（不弹菜单）**；右键单击消除闪烁并弹出菜单。上游 systray 无图标点击回调 → 库已 vendor 到 `internal/systray` 并打补丁新增 `SetOnTrayClick`（WndProc 的 `WM_LBUTTONUP` 分支只触发回调，`WM_RBUTTONUP` 分支触发回调 + `showMenu()`），go.mod 用 `replace` 指向本地副本。

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

- [x] main.go（流订阅 / 重连 / watchdog / toast / 托盘 / 图标闪烁）
- [x] state.go（断线续传）
- [x] icon.ico / icon_blank.ico 生成
- [x] go.mod 依赖（systray → `internal/systray` vendor 补丁版，toast.v1）
- [x] 编译通过（`ntfy-tray.exe`，windowsgui 无窗口）
- [x] 端到端验证：在线收 M1 → 写状态 → 离线收 M2 → 重启仅补推 M2，无重复推送，水印推进正确
- [ ] 开机自启（注册表 `HKCU\...\Run` 或启动文件夹快捷方式，待决定是否内置）
