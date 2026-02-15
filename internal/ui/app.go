package ui

import (
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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frpcx/internal/config"
	"frpcx/internal/frpc"
)

type frpcForm struct {
	ServerAddr string
	ServerPort string
	Token      string
	ProxyName  string
	LocalPort  string
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
	proxyNameEntry  *widget.Entry
	localPortEntry  *widget.Entry
	remotePortEntry *widget.Entry

	autoSaveMu    sync.Mutex
	autoSaveTimer *time.Timer
}

func Run(cfg *config.AppConfig) {
	normalizeSimpleConfig(cfg)

	a := app.NewWithID("suidaohe")
	a.Settings().SetTheme(newSuidaoTheme())
	win := a.NewWindow("穿透助手")
	mgr := frpc.NewManager(cfg)

	u := &App{
		app: a,
		win: win,
		cfg: cfg,
		mgr: mgr,
	}

	u.build()
	u.setupTray()
	u.startStatusTicker()

	win.Resize(fyne.NewSize(700, 460))
	win.ShowAndRun()
}

func (u *App) build() {
	u.statusDot = canvas.NewText("●", statusColor("stopped"))
	u.statusDot.TextSize = 16

	u.profileLabel = widget.NewLabelWithStyle("未配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.hintLabel = widget.NewLabel("修改配置项后将自动保存为 TOML")
	u.errorLabel = widget.NewLabel("")

	initial := u.loadFormFromCurrentProfile()
	u.serverAddrEntry = widget.NewEntry()
	u.serverPortEntry = widget.NewEntry()
	u.tokenEntry = widget.NewPasswordEntry()
	u.proxyNameEntry = widget.NewEntry()
	u.localPortEntry = widget.NewEntry()
	u.remotePortEntry = widget.NewEntry()

	u.serverAddrEntry.SetPlaceHolder("服务器地址，例如 1.2.3.4")
	u.serverPortEntry.SetPlaceHolder("服务器端口，例如 7000")
	u.tokenEntry.SetPlaceHolder("Token（可选）")
	u.proxyNameEntry.SetPlaceHolder("代理名称，例如 ssh")
	u.localPortEntry.SetPlaceHolder("本地端口，例如 22")
	u.remotePortEntry.SetPlaceHolder("远程端口，例如 6000")

	u.serverAddrEntry.SetText(initial.ServerAddr)
	u.serverPortEntry.SetText(initial.ServerPort)
	u.tokenEntry.SetText(initial.Token)
	u.proxyNameEntry.SetText(initial.ProxyName)
	u.localPortEntry.SetText(initial.LocalPort)
	u.remotePortEntry.SetText(initial.RemotePort)

	onChange := func(string) {
		u.scheduleAutoSave(u.readForm())
	}
	u.serverAddrEntry.OnChanged = onChange
	u.serverPortEntry.OnChanged = onChange
	u.tokenEntry.OnChanged = onChange
	u.proxyNameEntry.OnChanged = onChange
	u.localPortEntry.OnChanged = onChange
	u.remotePortEntry.OnChanged = onChange

	u.logEntry = widget.NewMultiLineEntry()
	u.logEntry.SetMinRowsVisible(3)
	u.logEntry.Wrapping = fyne.TextWrapOff
	u.logEntry.Disable()

	startBtn := widget.NewButtonWithIcon("启动", theme.MediaPlayIcon(), func() {
		u.mgr.StartAuto()
	})
	stopBtn := widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), func() {
		u.mgr.Stop()
	})

	statusRow := container.NewHBox(u.statusDot, widget.NewLabel(" "), u.profileLabel)
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "服务器地址", Widget: u.serverAddrEntry},
			{Text: "服务器端口", Widget: u.serverPortEntry},
			{Text: "Token", Widget: u.tokenEntry},
			{Text: "代理名称", Widget: u.proxyNameEntry},
			{Text: "本地端口", Widget: u.localPortEntry},
			{Text: "远程端口", Widget: u.remotePortEntry},
		},
	}
	actionsRow := container.NewGridWithColumns(2, startBtn, stopBtn)
	logsCard := widget.NewCard("日志", "", u.logEntry)

	u.refreshConfiguredProfileLabel()
	u.win.SetContent(container.NewVBox(statusRow, form, u.hintLabel, u.errorLabel, actionsRow, logsCard))
}

func (u *App) readForm() frpcForm {
	return frpcForm{
		ServerAddr: strings.TrimSpace(u.serverAddrEntry.Text),
		ServerPort: strings.TrimSpace(u.serverPortEntry.Text),
		Token:      strings.TrimSpace(u.tokenEntry.Text),
		ProxyName:  strings.TrimSpace(u.proxyNameEntry.Text),
		LocalPort:  strings.TrimSpace(u.localPortEntry.Text),
		RemotePort: strings.TrimSpace(u.remotePortEntry.Text),
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

func (u *App) saveFormToGeneratedToml(form frpcForm) {
	u.cfg.AutoSwitch = false
	u.cfg.WebDAV = config.WebDAVConfig{}

	if isEmptyForm(form) {
		u.cfg.ActiveProfile = ""
		u.cfg.Profiles = nil
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		fyne.Do(func() {
			u.hintLabel.SetText("配置已清空")
			u.refreshConfiguredProfileLabel()
		})
		return
	}

	serverPort, err := parsePort(form.ServerPort)
	if err != nil {
		u.setHint("未保存：服务器端口无效")
		return
	}
	localPort, err := parsePort(form.LocalPort)
	if err != nil {
		u.setHint("未保存：本地端口无效")
		return
	}
	remotePort, err := parsePort(form.RemotePort)
	if err != nil {
		u.setHint("未保存：远程端口无效")
		return
	}
	if form.ServerAddr == "" {
		u.setHint("未保存：请填写服务器地址")
		return
	}
	if form.ProxyName == "" {
		u.setHint("未保存：请填写代理名称")
		return
	}

	cfgPath, err := generatedConfigPath()
	if err != nil {
		u.setHint("未保存：无法创建配置目录")
		return
	}

	content := buildFrpcToml(form.ServerAddr, serverPort, form.Token, form.ProxyName, localPort, remotePort)
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		u.setHint("未保存：写入 TOML 失败")
		return
	}

	p := config.Profile{
		Name:              form.ProxyName,
		Enabled:           true,
		FrpcPath:          "",
		ConfigPath:        cfgPath,
		RequireStatus:     false,
		StartTimeoutSec:   8,
		HealthTimeoutSec:  3,
		StatusTimeoutSec:  0,
		StatusIntervalSec: 0,
	}

	u.cfg.ActiveProfile = p.Name
	u.cfg.Profiles = []config.Profile{p}
	if err := config.Save(u.cfg); err != nil {
		u.setHint("未保存：写入应用配置失败")
		return
	}
	u.mgr.SetConfig(u.cfg)

	fyne.Do(func() {
		u.hintLabel.SetText("已自动保存")
		u.refreshConfiguredProfileLabel()
	})
}

func (u *App) setHint(msg string) {
	fyne.Do(func() {
		u.hintLabel.SetText(msg)
	})
}

func isEmptyForm(form frpcForm) bool {
	return form.ServerAddr == "" &&
		form.ServerPort == "" &&
		form.Token == "" &&
		form.ProxyName == "" &&
		form.LocalPort == "" &&
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

func buildFrpcToml(serverAddr string, serverPort int, token, proxyName string, localPort, remotePort int) string {
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
	b.WriteString("type = \"tcp\"\n")
	b.WriteString("localIP = \"127.0.0.1\"\n")
	b.WriteString("localPort = ")
	b.WriteString(strconv.Itoa(localPort))
	b.WriteString("\n")
	b.WriteString("remotePort = ")
	b.WriteString(strconv.Itoa(remotePort))
	b.WriteString("\n")
	return b.String()
}

func escapeTomlString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func (u *App) loadFormFromCurrentProfile() frpcForm {
	p := u.currentProfile()
	if p == nil || strings.TrimSpace(p.ConfigPath) == "" {
		return frpcForm{}
	}

	b, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		return frpcForm{}
	}

	out := frpcForm{}
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
		case "name":
			if out.ProxyName == "" {
				out.ProxyName = unquoteTomlValue(val)
			}
		case "localPort":
			out.LocalPort = normalizeIntText(val)
		case "remotePort":
			out.RemotePort = normalizeIntText(val)
		}
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

func (u *App) refreshConfiguredProfileLabel() {
	p := u.currentProfile()
	if p == nil {
		u.profileLabel.SetText("未配置")
		return
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "默认配置"
	}
	u.profileLabel.SetText(name)
}

func (u *App) startStatusTicker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for range ticker.C {
			snap := u.mgr.Status()
			fyne.Do(func() {
				u.statusDot.Color = statusColor(snap.Status)
				u.statusDot.Refresh()
				if snap.ProfileName == "" {
					u.refreshConfiguredProfileLabel()
				} else {
					u.profileLabel.SetText(snap.ProfileName)
				}
				u.errorLabel.SetText(snap.LastError)
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
	p.Enabled = true
	p.FrpcPath = ""
	p.RequireStatus = false
	p.RemoteConfigPath = ""
	p.ServerAddr = ""
	p.ServerPort = 0
	p.LocalCheckPorts = nil
	p.StatusTimeoutSec = 0
	p.StatusIntervalSec = 0

	if strings.TrimSpace(p.Name) == "" {
		p.Name = "默认配置"
	}

	cfg.Profiles = []config.Profile{p}
	cfg.ActiveProfile = p.Name
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
