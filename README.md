# 穿透助手 (MVP)

一个小巧的跨平台桌面 frpc 客户端，极简单配置模式。

## 功能
- 单窗口轻量 UI + 系统托盘菜单
- 在软件内输入必要 frpc 配置项，自动生成并保存 TOML
- 运行状态圆点展示（颜色区分状态）
- 启动/停止与日志查看

## 说明
- 当前版本仅保留单配置，不包含云同步、状态检查、多配置切换。
- 仅使用内置 `frpc`，请使用 Release 产物（已启用 `with_embedded_frpc`）。
- 配置文件保存在用户配置目录下：`frpcx/config.json`。
- 自动生成的 frpc TOML 文件路径：`frpcx/generated/single.toml`。

## 构建
```bash
# 需要 Go 1.22+ 和 Fyne
go mod tidy
GOOS=darwin GOARCH=amd64 go build -tags with_embedded_frpc -o frpcx
```

## 运行
```bash
./frpcx
```

## macOS 提示“已损坏/无法打开”
这是 macOS Gatekeeper 对未签名应用的拦截。将应用拖到“应用程序”后，执行以下命令解除隔离：
```bash
xattr -dr com.apple.quarantine /Applications/穿透助手.app
```
然后双击即可打开。

## 单文件构建（内嵌 frpc）
将对应平台的 `frpc` 二进制放到：
- `internal/frpc/assets/frpc/<goos>_<goarch>/frpc`（Windows 为 `frpc.exe`）

然后编译：
```bash
go build -tags with_embedded_frpc -o frpcx
```
