package ui

import (
	"path/filepath"
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
	"image/color"

	"frpcx/internal/config"
	"frpcx/internal/frpc"
)

type App struct {
	app fyne.App
	win fyne.Window
	cfg *config.AppConfig
	mgr *frpc.Manager

	statusDot    *canvas.Text
	profileLabel *widget.Label
	errorLabel   *widget.Label
	pathEntry    *widget.Entry
	logEntry     *widget.Entry

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

	win.Resize(fyne.NewSize(700, 380))
	win.ShowAndRun()
}

func (u *App) build() {
	u.statusDot = canvas.NewText("●", statusColor("stopped"))
	u.statusDot.TextSize = 16

	u.profileLabel = widget.NewLabelWithStyle("未配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.errorLabel = widget.NewLabel("")

	u.pathEntry = widget.NewEntry()
	u.pathEntry.SetPlaceHolder("输入 frpc 配置文件路径（自动保存）")
	if p := u.currentProfile(); p != nil {
		u.pathEntry.SetText(p.ConfigPath)
	}
	u.pathEntry.OnChanged = func(v string) {
		u.scheduleAutoSave(v)
	}

	u.logEntry = widget.NewMultiLineEntry()
	u.logEntry.SetMinRowsVisible(4)
	u.logEntry.Wrapping = fyne.TextWrapOff
	u.logEntry.Disable()

	startBtn := widget.NewButtonWithIcon("启动", theme.MediaPlayIcon(), func() {
		u.mgr.StartAuto()
	})
	stopBtn := widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), func() {
		u.mgr.Stop()
	})

	statusRow := container.NewHBox(u.statusDot, widget.NewLabel(" "), u.profileLabel)
	actionsRow := container.NewGridWithColumns(2, startBtn, stopBtn)
	logsCard := widget.NewCard("日志", "", u.logEntry)

	u.refreshConfiguredProfileLabel()
	u.win.SetContent(container.NewVBox(statusRow, u.pathEntry, actionsRow, u.errorLabel, logsCard))
}

func (u *App) scheduleAutoSave(path string) {
	trimmed := strings.TrimSpace(path)

	u.autoSaveMu.Lock()
	defer u.autoSaveMu.Unlock()

	if u.autoSaveTimer != nil {
		u.autoSaveTimer.Stop()
	}

	u.autoSaveTimer = time.AfterFunc(450*time.Millisecond, func() {
		u.saveSingleProfile(trimmed)
	})
}

func (u *App) saveSingleProfile(configPath string) {
	u.cfg.AutoSwitch = false
	u.cfg.WebDAV = config.WebDAVConfig{}

	if configPath == "" {
		u.cfg.ActiveProfile = ""
		u.cfg.Profiles = nil
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		fyne.Do(func() {
			u.refreshConfiguredProfileLabel()
		})
		return
	}

	name := autoProfileName(configPath)
	if name == "" {
		name = "默认配置"
	}

	p := config.Profile{
		Name:              name,
		Enabled:           true,
		FrpcPath:          "",
		ConfigPath:        configPath,
		RequireStatus:     false,
		StartTimeoutSec:   8,
		HealthTimeoutSec:  3,
		StatusTimeoutSec:  0,
		StatusIntervalSec: 0,
	}

	u.cfg.ActiveProfile = p.Name
	u.cfg.Profiles = []config.Profile{p}
	_ = config.Save(u.cfg)
	u.mgr.SetConfig(u.cfg)

	fyne.Do(func() {
		u.refreshConfiguredProfileLabel()
	})
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
		name = autoProfileName(p.ConfigPath)
	}
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
		name := autoProfileName(p.ConfigPath)
		if name == "" {
			name = "默认配置"
		}
		p.Name = name
	}

	cfg.Profiles = []config.Profile{p}
	cfg.ActiveProfile = p.Name
	_ = config.Save(cfg)
}

func autoProfileName(configPath string) string {
	base := filepath.Base(strings.TrimSpace(configPath))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	name := strings.TrimSuffix(base, filepath.Ext(base))
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return name
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
