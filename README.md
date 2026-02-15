# 穿透助手 (MVP)

一个小巧的跨平台桌面 frpc 客户端，极简单配置模式。

## 功能
- 单窗口轻量 UI + 系统托盘菜单
- 单配置引导（只需选择一个本地 frpc 配置文件）
- 一键保存并启动
- 启动/停止与日志查看

## 说明
- 目前依赖外部 `frpc` 二进制，可在界面中选填 `frpc 路径`，或保证系统 PATH 中可找到 `frpc`。
- 配置文件保存在用户配置目录下：`frpcx/config.json`。
- 当前版本仅保留单配置，不包含云同步、状态检查、多配置切换。

## 构建
```bash
# 需要 Go 1.22+ 和 Fyne
go mod tidy
GOOS=darwin GOARCH=amd64 go build -o frpcx
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
