package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frpcx/internal/config"
	"frpcx/internal/frpc"
)

const singleProfileName = "default"

type frpcForm struct {
	ServerAddr string
	ServerPort string
	Token      string
	ProxyType  string
	LocalPort  string
	Domain     string
	RemotePort string
}

type App struct {
	app fyne.App
	win fyne.Window
	cfg *config.AppConfig
	mgr *frpc.Manager

	statusDot    *canvas.Text
	profileLabel *widget.Label
	hintLabel    *widget.Label
	errorLabel   *widget.Label
	logEntry     *widget.Entry

	serverAddrEntry *widget.Entry
	serverPortEntry *widget.Entry
	tokenEntry      *widget.Entry
	proxyTypeSelect *widget.Select
	localPortEntry  *widget.Entry
	domainEntry     *widget.Entry
	remotePortEntry *widget.Entry

	domainRow     fyne.CanvasObject
	remotePortRow fyne.CanvasObject

	autoSaveMu    sync.Mutex
	autoSaveTimer *time.Timer
}

func Run(cfg *config.AppConfig) {
	normalizeSimpleConfig(cfg)

	a := app.NewWithID("suidaohe")
	a.Settings().SetTheme(newSuidaoTheme())
	win := a.NewWindow("穿透助手")
	mgr := frpc.NewManager(cfg)

	u := &App{app: a, win: win, cfg: cfg, mgr: mgr}
	u.build()
	u.setupTray()
	u.startStatusTicker()

	win.Resize(fyne.NewSize(700, 470))
	win.ShowAndRun()
}

func (u *App) build() {
	u.statusDot = canvas.NewText("●", statusColor("stopped"))
	u.statusDot.TextSize = 16
	u.profileLabel = widget.NewLabelWithStyle(singleProfileName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.hintLabel = widget.NewLabel("修改参数后自动保存")
	u.errorLabel = widget.NewLabel("")

	formData := u.loadFormFromCurrentProfile()

	u.serverAddrEntry = widget.NewEntry()
	u.serverPortEntry = widget.NewEntry()
	u.tokenEntry = widget.NewPasswordEntry()
	u.localPortEntry = widget.NewEntry()
	u.domainEntry = widget.NewEntry()
	u.remotePortEntry = widget.NewEntry()

	u.proxyTypeSelect = widget.NewSelect([]string{"http", "tcp"}, func(string) {
		u.updateProxyTypeUI()
		u.scheduleAutoSave(u.readForm())
	})

	u.serverAddrEntry.SetPlaceHolder("例如 121.40.193.43")
	u.serverPortEntry.SetPlaceHolder("例如 7000")
	u.tokenEntry.SetPlaceHolder("可选")
	u.localPortEntry.SetPlaceHolder("例如 8000")
	u.domainEntry.SetPlaceHolder("例如 frp.iqei.cn")
	u.remotePortEntry.SetPlaceHolder("TCP 时必填，例如 6000")

	u.serverAddrEntry.SetText(formData.ServerAddr)
	u.serverPortEntry.SetText(formData.ServerPort)
	u.tokenEntry.SetText(formData.Token)
	u.localPortEntry.SetText(formData.LocalPort)
	u.domainEntry.SetText(formData.Domain)
	u.remotePortEntry.SetText(formData.RemotePort)
	u.proxyTypeSelect.SetSelected(formData.ProxyType)

	tokenShown := false
	var tokenToggle *widget.Button
	tokenToggle = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		tokenShown = !tokenShown
		u.tokenEntry.Password = !tokenShown
		if tokenShown {
			tokenToggle.SetIcon(theme.VisibilityOffIcon())
		} else {
			tokenToggle.SetIcon(theme.VisibilityIcon())
		}
		u.tokenEntry.Refresh()
	})

	onEntryChanged := func(string) {
		u.scheduleAutoSave(u.readForm())
	}
	u.serverAddrEntry.OnChanged = onEntryChanged
	u.serverPortEntry.OnChanged = onEntryChanged
	u.tokenEntry.OnChanged = onEntryChanged
	u.localPortEntry.OnChanged = onEntryChanged
	u.domainEntry.OnChanged = onEntryChanged
	u.remotePortEntry.OnChanged = onEntryChanged

	rowServer := container.NewGridWithColumns(4,
		widget.NewLabel("服务器"), u.serverAddrEntry,
		widget.NewLabel("端口"), u.serverPortEntry,
	)
	tokenInput := container.NewBorder(nil, nil, nil, tokenToggle, u.tokenEntry)
	rowToken := container.NewGridWithColumns(2, widget.NewLabel("Token"), tokenInput)
	rowType := container.NewGridWithColumns(4,
		widget.NewLabel("类型"), u.proxyTypeSelect,
		widget.NewLabel("本地端口"), u.localPortEntry,
	)
	u.domainRow = container.NewGridWithColumns(2, widget.NewLabel("域名"), u.domainEntry)
	u.remotePortRow = container.NewGridWithColumns(2, widget.NewLabel("远程端口"), u.remotePortEntry)

	u.updateProxyTypeUI()

	saveBtn := widget.NewButtonWithIcon("保存", theme.DocumentSaveIcon(), func() {
		if err := u.saveFormToGeneratedToml(u.readForm()); err != nil {
			u.errorLabel.SetText(err.Error())
			return
		}
		u.errorLabel.SetText("")
	})
	startBtn := widget.NewButtonWithIcon("启动", theme.MediaPlayIcon(), func() {
		if err := u.saveFormToGeneratedToml(u.readForm()); err != nil {
			u.errorLabel.SetText(err.Error())
			return
		}
		u.errorLabel.SetText("")
		u.mgr.StartAuto()
		go u.watchStartResult()
	})
	stopBtn := widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), func() {
		u.mgr.Stop()
	})
	actionsRow := container.NewGridWithColumns(3, saveBtn, startBtn, stopBtn)

	u.logEntry = widget.NewMultiLineEntry()
	u.logEntry.SetMinRowsVisible(3)
	u.logEntry.Wrapping = fyne.TextWrapOff
	u.logEntry.Disable()
	logsCard := widget.NewCard("日志", "", u.logEntry)

	statusRow := container.NewHBox(u.statusDot, widget.NewLabel(" "), u.profileLabel, layout.NewSpacer())
	configCard := widget.NewCard("", "", container.NewVBox(rowServer, rowToken, rowType, u.domainRow, u.remotePortRow))

	u.win.SetContent(container.NewVBox(statusRow, configCard, u.hintLabel, u.errorLabel, actionsRow, logsCard))
}

func (u *App) updateProxyTypeUI() {
	if u.proxyTypeSelect.Selected == "tcp" {
		u.domainRow.Hide()
		u.remotePortRow.Show()
		return
	}
	u.domainRow.Show()
	u.remotePortRow.Hide()
}

func (u *App) readForm() frpcForm {
	return frpcForm{
		ServerAddr: strings.TrimSpace(u.serverAddrEntry.Text),
		ServerPort: strings.TrimSpace(u.serverPortEntry.Text),
		Token:      strings.TrimSpace(u.tokenEntry.Text),
		ProxyType:  strings.TrimSpace(u.proxyTypeSelect.Selected),
		LocalPort:  strings.TrimSpace(u.localPortEntry.Text),
		Domain:     strings.TrimSpace(u.domainEntry.Text),
		RemotePort: strings.TrimSpace(u.remotePortEntry.Text),
	}
}

func defaultForm() frpcForm {
	return frpcForm{
		ServerAddr: "121.40.193.43",
		ServerPort: "7000",
		Token:      "",
		ProxyType:  "http",
		LocalPort:  "8000",
		Domain:     "frp.iqei.cn",
		RemotePort: "6000",
	}
}

func (u *App) scheduleAutoSave(form frpcForm) {
	u.autoSaveMu.Lock()
	defer u.autoSaveMu.Unlock()

	if u.autoSaveTimer != nil {
		u.autoSaveTimer.Stop()
	}
	u.autoSaveTimer = time.AfterFunc(500*time.Millisecond, func() {
		u.saveFormToGeneratedToml(form)
	})
}

func (u *App) saveFormToGeneratedToml(form frpcForm) error {
	u.autoSaveMu.Lock()
	defer u.autoSaveMu.Unlock()

	u.cfg.AutoSwitch = false
	u.cfg.WebDAV = config.WebDAVConfig{}

	if isEmptyForm(form) {
		u.cfg.ActiveProfile = ""
		u.cfg.Profiles = nil
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		u.setHint("配置已清空")
		return fmt.Errorf("请先填写配置参数")
	}

	if form.ProxyType == "" {
		form.ProxyType = "http"
	}
	if form.ServerAddr == "" {
		u.setHint("未保存：请填写服务器地址")
		return fmt.Errorf("请填写服务器地址")
	}

	serverPort, err := parsePort(form.ServerPort)
	if err != nil {
		u.setHint("未保存：服务器端口无效")
		return fmt.Errorf("服务器端口无效")
	}
	localPort, err := parsePort(form.LocalPort)
	if err != nil {
		u.setHint("未保存：本地端口无效")
		return fmt.Errorf("本地端口无效")
	}

	domain := form.Domain
	remotePort := 0
	if form.ProxyType == "tcp" {
		remotePort, err = parsePort(form.RemotePort)
		if err != nil {
			u.setHint("未保存：远程端口无效")
			return fmt.Errorf("远程端口无效")
		}
	} else {
		if domain == "" {
			u.setHint("未保存：HTTP 模式请填写域名")
			return fmt.Errorf("HTTP 模式请填写域名")
		}
	}

	cfgPath, err := generatedConfigPath()
	if err != nil {
		u.setHint("未保存：无法创建配置目录")
		return fmt.Errorf("无法创建配置目录")
	}

	content := buildFrpcToml(form.ServerAddr, serverPort, form.Token, singleProfileName, form.ProxyType, localPort, domain, remotePort)
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		u.setHint("未保存：写入 TOML 失败")
		return fmt.Errorf("写入 TOML 失败")
	}

	u.cfg.ActiveProfile = singleProfileName
	u.cfg.Profiles = []config.Profile{{
		Name:              singleProfileName,
		Enabled:           true,
		FrpcPath:          "",
		ConfigPath:        cfgPath,
		RequireStatus:     false,
		StartTimeoutSec:   8,
		HealthTimeoutSec:  3,
		StatusTimeoutSec:  0,
		StatusIntervalSec: 0,
	}}
	if err := config.Save(u.cfg); err != nil {
		u.setHint("未保存：写入应用配置失败")
		return fmt.Errorf("写入应用配置失败")
	}
	u.mgr.SetConfig(u.cfg)
	u.setHint("已自动保存")
	return nil
}

func (u *App) watchStartResult() {
	timeout := time.NewTimer(12 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			fyne.Do(func() {
				if strings.TrimSpace(u.errorLabel.Text) == "" {
					u.errorLabel.SetText("启动超时，请检查参数和日志")
				}
			})
			return
		case <-ticker.C:
			snap := u.mgr.Status()
			if snap.Status == "running" {
				return
			}
			if snap.Status == "error" {
				fyne.Do(func() {
					u.errorLabel.SetText(snap.LastError)
				})
				return
			}
		}
	}
}

func isEmptyForm(form frpcForm) bool {
	return form.ServerAddr == "" &&
		form.ServerPort == "" &&
		form.Token == "" &&
		form.ProxyType == "" &&
		form.LocalPort == "" &&
		form.Domain == "" &&
		form.RemotePort == ""
}

func parsePort(s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	if v <= 0 || v > 65535 {
		return 0, strconv.ErrRange
	}
	return v, nil
}

func generatedConfigPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "generated")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(out, "single.toml"), nil
}

func buildFrpcToml(serverAddr string, serverPort int, token, proxyName, proxyType string, localPort int, domain string, remotePort int) string {
	var b strings.Builder
	b.WriteString("serverAddr = \"")
	b.WriteString(escapeTomlString(serverAddr))
	b.WriteString("\"\n")
	b.WriteString("serverPort = ")
	b.WriteString(strconv.Itoa(serverPort))
	b.WriteString("\n")

	if token != "" {
		b.WriteString("auth.method = \"token\"\n")
		b.WriteString("auth.token = \"")
		b.WriteString(escapeTomlString(token))
		b.WriteString("\"\n")
	}

	b.WriteString("\n[[proxies]]\n")
	b.WriteString("name = \"")
	b.WriteString(escapeTomlString(proxyName))
	b.WriteString("\"\n")
	b.WriteString("type = \"")
	b.WriteString(escapeTomlString(proxyType))
	b.WriteString("\"\n")
	b.WriteString("localIP = \"127.0.0.1\"\n")
	b.WriteString("localPort = ")
	b.WriteString(strconv.Itoa(localPort))
	b.WriteString("\n")

	if proxyType == "tcp" {
		b.WriteString("remotePort = ")
		b.WriteString(strconv.Itoa(remotePort))
		b.WriteString("\n")
	} else {
		b.WriteString("customDomains = [\"")
		b.WriteString(escapeTomlString(domain))
		b.WriteString("\"]\n")
	}

	return b.String()
}

func escapeTomlString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func (u *App) loadFormFromCurrentProfile() frpcForm {
	out := defaultForm()

	p := u.currentProfile()
	if p == nil || strings.TrimSpace(p.ConfigPath) == "" {
		return out
	}

	b, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		return out
	}

	for _, raw := range strings.Split(string(b), "\n") {
		key, val, ok := splitTomlKV(raw)
		if !ok {
			continue
		}
		switch key {
		case "serverAddr":
			out.ServerAddr = unquoteTomlValue(val)
		case "serverPort":
			out.ServerPort = normalizeIntText(val)
		case "auth.token":
			out.Token = unquoteTomlValue(val)
		case "type":
			out.ProxyType = unquoteTomlValue(val)
		case "localPort":
			out.LocalPort = normalizeIntText(val)
		case "remotePort":
			out.RemotePort = normalizeIntText(val)
		case "customDomains":
			if d := parseFirstArrayString(val); d != "" {
				out.Domain = d
			}
		}
	}
	if out.ProxyType == "" {
		out.ProxyType = "http"
	}
	return out
}

func splitTomlKV(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[[") {
		return "", "", false
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if idx := strings.Index(val, "#"); idx >= 0 {
		val = strings.TrimSpace(val[:idx])
	}
	return key, val, true
}

func unquoteTomlValue(v string) string {
	v = strings.TrimSpace(v)
	if s, err := strconv.Unquote(v); err == nil {
		return s
	}
	return strings.Trim(v, "\"")
}

func normalizeIntText(v string) string {
	n, err := strconv.Atoi(unquoteTomlValue(v))
	if err != nil {
		return ""
	}
	return strconv.Itoa(n)
}

func parseFirstArrayString(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return ""
	}
	inner := strings.TrimSpace(v[1 : len(v)-1])
	if inner == "" {
		return ""
	}
	first := strings.SplitN(inner, ",", 2)[0]
	return unquoteTomlValue(strings.TrimSpace(first))
}

func (u *App) currentProfile() *config.Profile {
	if len(u.cfg.Profiles) == 0 {
		return nil
	}
	if u.cfg.ActiveProfile != "" {
		for i := range u.cfg.Profiles {
			if u.cfg.Profiles[i].Name == u.cfg.ActiveProfile {
				return &u.cfg.Profiles[i]
			}
		}
	}
	return &u.cfg.Profiles[0]
}

func (u *App) startStatusTicker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for range ticker.C {
			snap := u.mgr.Status()
			fyne.Do(func() {
				u.statusDot.Color = statusColor(snap.Status)
				u.statusDot.Refresh()
				if strings.TrimSpace(snap.LastError) != "" {
					u.errorLabel.SetText(snap.LastError)
				}
				if len(snap.LogLines) > 0 {
					u.logEntry.SetText(strings.Join(snap.LogLines, "\n"))
				}
			})
		}
	}()
}

func (u *App) setupTray() {
	if desk, ok := u.app.(desktop.App); ok {
		showItem := fyne.NewMenuItem("显示", func() { u.win.Show() })
		hideItem := fyne.NewMenuItem("隐藏", func() { u.win.Hide() })
		startItem := fyne.NewMenuItem("启动", func() { u.mgr.StartAuto() })
		stopItem := fyne.NewMenuItem("停止", func() { u.mgr.Stop() })
		quitItem := fyne.NewMenuItem("退出", func() { u.app.Quit() })

		menu := fyne.NewMenu("穿透助手", showItem, hideItem, startItem, stopItem, quitItem)
		desk.SetSystemTrayMenu(menu)
	}
}

func (u *App) setHint(msg string) {
	fyne.Do(func() {
		u.hintLabel.SetText(msg)
	})
}

func normalizeSimpleConfig(cfg *config.AppConfig) {
	if cfg == nil {
		return
	}

	cfg.AutoSwitch = false
	cfg.WebDAV = config.WebDAVConfig{}

	if len(cfg.Profiles) == 0 {
		cfg.ActiveProfile = ""
		_ = config.Save(cfg)
		return
	}

	idx := 0
	if cfg.ActiveProfile != "" {
		for i := range cfg.Profiles {
			if cfg.Profiles[i].Name == cfg.ActiveProfile {
				idx = i
				break
			}
		}
	}

	p := cfg.Profiles[idx]
	p.Name = singleProfileName
	p.Enabled = true
	p.FrpcPath = ""
	p.RequireStatus = false
	p.RemoteConfigPath = ""
	p.ServerAddr = ""
	p.ServerPort = 0
	p.LocalCheckPorts = nil
	p.StatusTimeoutSec = 0
	p.StatusIntervalSec = 0

	cfg.Profiles = []config.Profile{p}
	cfg.ActiveProfile = singleProfileName
	_ = config.Save(cfg)
}

func statusColor(status string) color.Color {
	switch status {
	case "starting":
		return color.NRGBA{R: 0xF2, G: 0xC0, B: 0x38, A: 0xFF}
	case "running":
		return color.NRGBA{R: 0x2E, G: 0xD5, B: 0x73, A: 0xFF}
	case "error":
		return color.NRGBA{R: 0xFF, G: 0x5D, B: 0x6C, A: 0xFF}
	default:
		return color.NRGBA{R: 0x6E, G: 0x7A, B: 0x88, A: 0xFF}
	}
}
