package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frpcx/internal/config"
	"frpcx/internal/frpc"
)

type App struct {
	app fyne.App
	win fyne.Window
	cfg *config.AppConfig
	mgr *frpc.Manager

	statusLabel  *widget.Label
	profileLabel *widget.Label
	errorLabel   *widget.Label
	logEntry     *widget.Entry
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

	win.Resize(fyne.NewSize(700, 420))
	win.ShowAndRun()
}

func (u *App) build() {
	u.statusLabel = widget.NewLabelWithStyle("已停止", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.profileLabel = widget.NewLabelWithStyle("未配置", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.errorLabel = widget.NewLabel("")
	u.logEntry = widget.NewMultiLineEntry()
	u.logEntry.SetMinRowsVisible(4)
	u.logEntry.Wrapping = fyne.TextWrapOff
	u.logEntry.Disable()

	startBtn := widget.NewButtonWithIcon("启动代理", theme.MediaPlayIcon(), func() {
		u.mgr.StartAuto()
	})
	stopBtn := widget.NewButtonWithIcon("停止代理", theme.MediaStopIcon(), func() {
		u.mgr.Stop()
	})

	title := widget.NewLabelWithStyle("穿透助手", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel("极简模式：只需配置一个 frpc 配置文件")
	titleCard := widget.NewCard("", "", container.NewVBox(title, subtitle))

	statusCard := widget.NewCard("运行状态", "", container.NewVBox(
		widget.NewLabel("状态"),
		u.statusLabel,
		widget.NewSeparator(),
		widget.NewLabel("当前配置"),
		u.profileLabel,
		widget.NewSeparator(),
		widget.NewLabel("错误信息"),
		u.errorLabel,
	))

	actionsCard := widget.NewCard("快捷操作", "", container.NewGridWithColumns(2, startBtn, stopBtn))
	logsCard := widget.NewCard("日志", "", u.logEntry)
	statusTab := container.NewVBox(titleCard, statusCard, actionsCard, logsCard)

	configTab := u.buildSingleConfigTab()
	tabs := container.NewAppTabs(
		container.NewTabItem("状态", statusTab),
		container.NewTabItem("配置", configTab),
	)

	u.refreshConfiguredProfileLabel()
	u.win.SetContent(tabs)
}

func (u *App) buildSingleConfigTab() fyne.CanvasObject {
	p := u.currentProfile()

	configPath := widget.NewEntry()
	configPath.SetPlaceHolder("必填：请选择 frpc 配置文件，例如 /path/to/frpc.toml")
	frpcPath := widget.NewEntry()
	frpcPath.SetPlaceHolder("可选：自定义 frpc 可执行文件路径")
	if p != nil {
		configPath.SetText(p.ConfigPath)
		frpcPath.SetText(p.FrpcPath)
	}

	configRow := u.filePickerRow(configPath, storage.NewExtensionFileFilter([]string{".toml", ".ini", ".yaml", ".yml", ".json"}))
	frpcRow := u.filePickerRow(frpcPath, nil)

	tip := widget.NewLabel("只保留一个配置：保存后会覆盖旧配置，直接用于启动。")
	tip.Wrapping = fyne.TextWrapWord

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "配置文件", Widget: configRow},
			{Text: "frpc 路径", Widget: frpcRow},
		},
	}

	save := func(startAfterSave bool) {
		cfgPath := strings.TrimSpace(configPath.Text)
		if cfgPath == "" {
			dialog.ShowError(fmt.Errorf("请选择配置文件"), u.win)
			return
		}

		name := autoProfileName(cfgPath)
		if name == "" {
			name = "默认配置"
		}

		profile := config.Profile{
			Name:              name,
			Enabled:           true,
			FrpcPath:          strings.TrimSpace(frpcPath.Text),
			ConfigPath:        cfgPath,
			RequireStatus:     false,
			StartTimeoutSec:   8,
			HealthTimeoutSec:  3,
			StatusTimeoutSec:  10,
			StatusIntervalSec: 5,
		}

		u.cfg.AutoSwitch = false
		u.cfg.ActiveProfile = profile.Name
		u.cfg.Profiles = []config.Profile{profile}
		u.cfg.WebDAV = config.WebDAVConfig{}

		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		u.refreshConfiguredProfileLabel()

		if startAfterSave {
			u.mgr.StartAuto()
			dialog.ShowInformation("配置", "保存成功，已启动", u.win)
			return
		}
		dialog.ShowInformation("配置", "保存成功", u.win)
	}

	saveBtn := widget.NewButtonWithIcon("保存", theme.DocumentSaveIcon(), func() {
		save(false)
	})
	saveStartBtn := widget.NewButtonWithIcon("保存并启动", theme.MediaPlayIcon(), func() {
		save(true)
	})

	actions := container.NewGridWithColumns(2, saveBtn, saveStartBtn)
	card := widget.NewCard("单配置设置", "", container.NewVBox(tip, form, actions))
	guide := widget.NewLabel("使用流程：1. 选配置文件 2. 保存 3. 回状态页查看运行")
	guide.Wrapping = fyne.TextWrapWord

	return container.NewVBox(card, guide)
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

func (u *App) filePickerRow(entry *widget.Entry, filter storage.FileFilter) fyne.CanvasObject {
	browse := widget.NewButton("浏览", func() {
		dlg := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			if uc == nil {
				return
			}
			entry.SetText(uc.URI().Path())
			_ = uc.Close()
		}, u.win)
		if filter != nil {
			dlg.SetFilter(filter)
		}
		dlg.Show()
	})
	return container.NewBorder(nil, nil, nil, browse, entry)
}

func (u *App) startStatusTicker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for range ticker.C {
			snap := u.mgr.Status()
			fyne.Do(func() {
				u.statusLabel.SetText(localizeStatus(snap.Status))
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
	} else {
		for i := range cfg.Profiles {
			if cfg.Profiles[i].Enabled {
				idx = i
				break
			}
		}
	}

	p := cfg.Profiles[idx]
	p.Enabled = true
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

func localizeStatus(status string) string {
	switch status {
	case "starting":
		return "启动中"
	case "running":
		return "运行中"
	case "stopped":
		return "已停止"
	case "error":
		return "异常"
	default:
		return status
	}
}
