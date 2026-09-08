# ntfy-tray

Windows 托盘常驻程序：订阅 [ntfy.sh](https://ntfy.sh)（或自建 ntfy 服务器）的 topic，收到新消息时弹出 Windows 通知（toast），并通过托盘图标闪烁提醒未读。

## 功能

- 订阅一个或多个 ntfy topic，消息实时弹 Windows 通知
- 断线自动重连（指数退避），重启后只补推错过的消息，不重复提醒
- 有未读通知时托盘图标闪烁，左键单击图标确认并停止闪烁
- 高优先级消息（priority ≥ 4）使用警报音
- 单文件 exe，无窗口、无运行时依赖，资源占用极低

## 命令行启动

```
ntfy-tray.exe -topics=mytopic,alerts
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-topics` | （必填） | 订阅的 topic，多个用英文逗号分隔 |
| `-server` | `https://ntfy.sh` | ntfy 服务器地址，自建服务器改这里 |
| `-token` | 空 | 访问受保护 topic 的 token（`tk_...`，在 ntfy 网页端生成） |

示例 —— 订阅自建服务器上的受保护 topic：

```
ntfy-tray.exe -server=https://ntfy.example.com -topics=alerts -token=tk_AbCdEf123
```

程序无控制台窗口，启动后驻留任务栏右下角通知区域。开机自启可将带参数的快捷方式放入 `shell:startup` 文件夹。

向 topic 发消息测试：

```
curl -d "hello" https://ntfy.sh/mytopic
```

## 交互

| 操作 | 行为 |
|---|---|
| 收到消息 | 弹出 Windows 通知；托盘图标开始闪烁（500ms 间隔） |
| **左键单击图标** | 确认全部未读，停止闪烁（不弹菜单） |
| **右键单击图标** | 停止闪烁，并弹出菜单 |
| 菜单 → Open ntfy | 浏览器打开第一个 topic 的网页 |
| 菜单 → Quit | 退出程序 |
| 点击通知 / 通知的 Open 按钮 | 打开消息自带的 `click` 链接，缺省为 topic 网页 |

## 状态文件

exe 同目录下的 `ntfy-tray-state.json` 记录每个 topic 最近一条已展示消息的时间戳：

```json
{
  "topics": {
    "mytopic": 1788891727
  }
}
```

程序重启后据此从服务器补拉错过的消息并去重。删除该文件即重置：下次启动只收新消息。文件由程序自动维护，无需手动编辑。

## 构建

需要 Go 1.27+：

```
go build -ldflags "-H=windowsgui" -o ntfy-tray.exe .
```

- `-H=windowsgui` 使 exe 无控制台窗口；调试用可省略该参数，日志会输出到控制台。
- 若构建时无法访问 sum.golang.org：`go env -w GOSUMDB=off`。

## 核心逻辑

- **订阅**：对 `{server}/{topics}/json` 发起 HTTP 长连接，服务端以 NDJSON 持续推送事件；多 topic 复用一条连接。仅 `message` 事件触发通知。
- **保活与重连**：服务端每 ~45s 发 `keepalive`；90s 无任何帧判定连接死亡，按 1s→2s→…→30s 指数退避重连。
- **断线续传**：启动时以状态文件中各 topic 水印的最小值作为 `since` 参数补拉历史，客户端再按 `msg.Time <= 水印` 逐 topic 去重，保证重启后只展示上次更新之后的通知、不漏不重。水印精度为秒。
- **闪烁**：每次通知使未读计数 +1，独立 goroutine 按 500ms 在正常/空白图标间切换；点击图标清零计数即恢复常显。
- **图标点击检测**：上游 systray 库不暴露图标点击事件，`internal/systray` 是打了补丁的 vendor 副本（WndProc 区分左右键分支），通过 `go.mod` 的 `replace` 接入。

更详细的技术取舍见 [plan.md](plan.md)。

## 文件结构

```
main.go            主程序：参数、订阅循环、toast、托盘与闪烁
state.go           状态文件读写（断线续传）
icon.ico           托盘常态图标（内嵌）
icon_blank.ico     闪烁用空白图标（内嵌）
internal/systray/  打了点击回调补丁的 systray vendor 副本
plan.md            技术方案文档
```
