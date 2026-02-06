# frpcx (MVP)

一个小巧的跨平台桌面 frpc 客户端，支持多配置自动切换与 WebDAV 同步。

## 功能
- 单窗口轻量 UI + 系统托盘菜单
- 多 Profile（按列表优先级）
- 启动失败/运行退出自动切换下一个 Profile
- WebDAV 同步（可用于坚果云）

## 说明
- 目前依赖外部 `frpc` 二进制，可在 Profile 中指定 `frpc Path`，或保证系统 PATH 中可找到 `frpc`。
- 配置文件保存在用户配置目录下：`frpcx/config.json`。
- WebDAV 同步会将远程配置下载到本地缓存：`frpcx/cache/`，并自动映射到 Profile。

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
xattr -dr com.apple.quarantine /Applications/FRPCX.app
```
然后双击即可打开。

## 单文件构建（内嵌 frpc）
将对应平台的 `frpc` 二进制放到：
- `internal/frpc/assets/frpc/<goos>_<goarch>/frpc`（Windows 为 `frpc.exe`）

然后编译：
```bash
go build -tags with_embedded_frpc -o frpcx
```

## 自动切换逻辑
- 预检查服务器可达（若设置了 server addr/port）
- 预检查本地服务端口（若设置）
- 启动 frpc 并解析日志错误模式
- 进程退出/失败自动切换下一个 Profile

## 状态健康检查
- 可选：对每个 Profile 使用 `frpc status -c <config>` 验证管理端口可用性。
- 需要在 frpc 配置中启用 `webServer`。
- 启用后，启动阶段会等待 status 成功，运行期定时检查，连续失败会自动切换。
