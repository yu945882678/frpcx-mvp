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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

    "frpcx/internal/config"
    "frpcx/internal/frpc"
    "frpcx/internal/webdav"
)

type App struct {
	app    fyne.App
	win    fyne.Window
	cfg    *config.AppConfig
	mgr    *frpc.Manager

	statusLabel  *widget.Label
	profileLabel *widget.Label
	errorLabel   *widget.Label
	healthLabel  *widget.Label
	healthErr    *widget.Label
	selectedInfo *widget.Label
	logEntry     *widget.Entry
	list         *widget.List
	selectedIdx  int
}

func Run(cfg *config.AppConfig) {
	a := app.NewWithID("suidaohe")
	a.Settings().SetTheme(newSuidaoTheme())
	win := a.NewWindow("穿透助手")
	mgr := frpc.NewManager(cfg)

    ui := &App{
        app:         a,
        win:         win,
        cfg:         cfg,
        mgr:         mgr,
        selectedIdx: -1,
    }

    ui.build()
    ui.setupTray()
    ui.startStatusTicker()

	win.Resize(fyne.NewSize(760, 540))
	win.ShowAndRun()
}

func (u *App) build() {
	u.statusLabel = widget.NewLabelWithStyle("已停止", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.profileLabel = widget.NewLabelWithStyle("-", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.errorLabel = widget.NewLabel("")
	u.healthLabel = widget.NewLabelWithStyle("未知", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.healthErr = widget.NewLabel("")
	u.logEntry = widget.NewMultiLineEntry()
	u.logEntry.SetMinRowsVisible(7)
	u.logEntry.Wrapping = fyne.TextWrapOff
	u.logEntry.Disable()

	startBtn := widget.NewButtonWithIcon("启动代理", theme.MediaPlayIcon(), func() {
		u.mgr.StartAuto()
	})
	stopBtn := widget.NewButtonWithIcon("停止代理", theme.MediaStopIcon(), func() {
		u.mgr.Stop()
	})
	syncBtn := widget.NewButtonWithIcon("同步配置", theme.ViewRefreshIcon(), func() {
		u.syncWebDAV()
	})
	checkBtn := widget.NewButtonWithIcon("状态检查", theme.SearchIcon(), func() {
		if err := u.mgr.CheckStatusNow(); err != nil {
			dialog.ShowError(err, u.win)
			return
        }
        dialog.ShowInformation("状态", "正常", u.win)
    })
	autoSwitch := widget.NewCheck("自动切换", func(v bool) {
		u.cfg.AutoSwitch = v
		u.mgr.SetConfig(u.cfg)
		_ = config.Save(u.cfg)
	})
	autoSwitch.SetChecked(u.cfg.AutoSwitch)

	title := widget.NewLabelWithStyle("穿透助手", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel("三步使用：添加配置 -> 设为默认 -> 启动代理")
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
	healthCard := widget.NewCard("健康检查", "", container.NewVBox(
		u.healthLabel,
		u.healthErr,
	))
	metrics := container.NewGridWithColumns(2, statusCard, healthCard)

	actions := widget.NewCard("快捷操作", "", container.NewVBox(
		container.NewGridWithColumns(2, startBtn, stopBtn),
		container.NewGridWithColumns(2, syncBtn, checkBtn),
		autoSwitch,
	))

	logsCard := widget.NewCard("日志", "", u.logEntry)
	statusTab := container.NewVBox(titleCard, metrics, actions, logsCard)

	profilesTab := u.buildProfilesTab()
	webdavTab := u.buildWebDAVTab()

	tabs := container.NewAppTabs(
		container.NewTabItem("状态", statusTab),
		container.NewTabItem("配置", profilesTab),
		container.NewTabItem("云同步", webdavTab),
	)

	u.win.SetContent(tabs)
}

func (u *App) buildProfilesTab() fyne.CanvasObject {
	u.selectedInfo = widget.NewLabel("请先点击“引导添加”，只需填写配置文件即可。")
	u.selectedInfo.Wrapping = fyne.TextWrapWord

	u.list = widget.NewList(
		func() int { return len(u.cfg.Profiles) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(u.cfg.Profiles) {
				return
			}
			p := u.cfg.Profiles[i]
			status := ""
			if !p.Enabled {
				status = "（已禁用）"
			}
			if u.cfg.ActiveProfile == p.Name {
				status += " [默认]"
			}
			o.(*widget.Label).SetText(p.Name + status)
		},
	)

	u.list.OnSelected = func(id widget.ListItemID) {
		u.selectedIdx = id
		u.refreshSelectedProfileInfo()
	}

	addBtn := widget.NewButton("引导添加", func() {
		u.showProfileDialog("引导添加配置", nil, func(p config.Profile, setDefault bool) {
			u.cfg.Profiles = append(u.cfg.Profiles, p)
			if setDefault {
				u.cfg.ActiveProfile = p.Name
			}
			_ = config.Save(u.cfg)
			u.mgr.SetConfig(u.cfg)
			u.list.Refresh()
			u.refreshSelectedProfileInfo()
		})
	})

	editBtn := widget.NewButton("编辑", func() {
		if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
			return
		}
		p := u.cfg.Profiles[u.selectedIdx]
		u.showProfileDialog("编辑配置", &p, func(updated config.Profile, setDefault bool) {
			u.cfg.Profiles[u.selectedIdx] = updated
			if setDefault {
				u.cfg.ActiveProfile = updated.Name
			} else if u.cfg.ActiveProfile == p.Name {
				// 默认配置重命名时保持默认指向新名称。
				u.cfg.ActiveProfile = updated.Name
			}
			_ = config.Save(u.cfg)
			u.mgr.SetConfig(u.cfg)
			u.list.Refresh()
			u.refreshSelectedProfileInfo()
		})
	})

	removeBtn := widget.NewButton("删除", func() {
		if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
			return
		}
		removed := u.cfg.Profiles[u.selectedIdx]
		u.cfg.Profiles = append(u.cfg.Profiles[:u.selectedIdx], u.cfg.Profiles[u.selectedIdx+1:]...)
		if u.cfg.ActiveProfile == removed.Name {
			u.cfg.ActiveProfile = ""
		}
		u.selectedIdx = -1
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		u.list.Refresh()
		u.refreshSelectedProfileInfo()
	})

	upBtn := widget.NewButton("上移", func() {
		if u.selectedIdx <= 0 || u.selectedIdx >= len(u.cfg.Profiles) {
			return
		}
		u.cfg.Profiles[u.selectedIdx-1], u.cfg.Profiles[u.selectedIdx] = u.cfg.Profiles[u.selectedIdx], u.cfg.Profiles[u.selectedIdx-1]
		u.selectedIdx = u.selectedIdx - 1
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		u.list.Refresh()
		u.refreshSelectedProfileInfo()
	})

	downBtn := widget.NewButton("下移", func() {
		if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles)-1 {
			return
		}
		u.cfg.Profiles[u.selectedIdx+1], u.cfg.Profiles[u.selectedIdx] = u.cfg.Profiles[u.selectedIdx], u.cfg.Profiles[u.selectedIdx+1]
		u.selectedIdx = u.selectedIdx + 1
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		u.list.Refresh()
		u.refreshSelectedProfileInfo()
	})

	setActiveBtn := widget.NewButton("设为默认", func() {
		if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
			return
		}
		u.cfg.ActiveProfile = u.cfg.Profiles[u.selectedIdx].Name
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
		u.list.Refresh()
		u.refreshSelectedProfileInfo()
	})

	guide := widget.NewLabel("引导模式：\n1. 点击“引导添加”并选择本地 frpc 配置文件\n2. 点击“设为默认”\n3. 回到“状态”页点击“启动代理”")
	guide.Wrapping = fyne.TextWrapWord
	buttonsMain := container.NewGridWithColumns(2, addBtn, editBtn)
	buttonsExtra := container.NewGridWithColumns(2, removeBtn, setActiveBtn)
	buttonsOrder := container.NewGridWithColumns(2, upBtn, downBtn)
	listCard := widget.NewCard("配置列表", "", u.list)
	detailCard := widget.NewCard("配置摘要", "", container.NewVBox(u.selectedInfo, widget.NewSeparator(), guide))

	left := container.NewVBox(listCard, buttonsMain, buttonsExtra, buttonsOrder)
	right := container.NewVBox(detailCard)
	split := container.NewHSplit(left, right)
	split.SetOffset(0.56)
	return split
}

func (u *App) buildWebDAVTab() fyne.CanvasObject {
	urlEntry := widget.NewEntry()
	urlEntry.SetText(u.cfg.WebDAV.URL)
	urlEntry.SetPlaceHolder("可选，例如 https://dav.jianguoyun.com/dav/")
    userEntry := widget.NewEntry()
    userEntry.SetText(u.cfg.WebDAV.Username)
    userEntry.SetPlaceHolder("可选")
    passEntry := widget.NewPasswordEntry()
    passEntry.SetText(u.cfg.WebDAV.Password)
    passEntry.SetPlaceHolder("可选")
    baseEntry := widget.NewEntry()
    baseEntry.SetText(u.cfg.WebDAV.RemoteBase)
    baseEntry.SetPlaceHolder("可选，例如 /frpc")

	saveBtn := widget.NewButtonWithIcon("保存设置", theme.DocumentSaveIcon(), func() {
		u.cfg.WebDAV.URL = strings.TrimSpace(urlEntry.Text)
		u.cfg.WebDAV.Username = strings.TrimSpace(userEntry.Text)
		u.cfg.WebDAV.Password = passEntry.Text
		u.cfg.WebDAV.RemoteBase = strings.TrimSpace(baseEntry.Text)
		_ = config.Save(u.cfg)
		u.mgr.SetConfig(u.cfg)
	})

	syncBtn := widget.NewButtonWithIcon("立即同步", theme.ViewRefreshIcon(), func() {
		u.syncWebDAV()
	})

    form := &widget.Form{
        Items: []*widget.FormItem{
            {Text: "URL", Widget: urlEntry},
            {Text: "用户名", Widget: userEntry},
            {Text: "密码", Widget: passEntry},
            {Text: "远程根目录", Widget: baseEntry},
        },
        SubmitText: "",
        OnSubmit:   nil,
    }

	tips := widget.NewLabel("未配置 WebDAV 也可正常使用；配置后可从云端自动拉取 frpc 配置。")
	tips.Wrapping = fyne.TextWrapWord

	card := widget.NewCard("WebDAV 设置", "", form)
	actions := container.NewGridWithColumns(2, saveBtn, syncBtn)
	return container.NewVBox(card, tips, actions, layout.NewSpacer())
}

func (u *App) refreshSelectedProfileInfo() {
	if u.selectedInfo == nil {
		return
	}
	if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
		u.selectedInfo.SetText("请先点击“引导添加”，只需填写配置文件即可。")
		return
	}
	p := u.cfg.Profiles[u.selectedIdx]
	lines := []string{
		fmt.Sprintf("名称: %s", p.Name),
		fmt.Sprintf("启用: %s", boolToText(p.Enabled)),
	}
	if u.cfg.ActiveProfile == p.Name {
		lines = append(lines, "默认启动: 是")
	} else {
		lines = append(lines, "默认启动: 否")
	}
	if p.ConfigPath != "" {
		lines = append(lines, fmt.Sprintf("配置文件: %s", p.ConfigPath))
	} else {
		lines = append(lines, "配置文件: 未设置")
	}
	if p.RequireStatus {
		lines = append(lines, "状态检查: 已启用")
	} else {
		lines = append(lines, "状态检查: 未启用")
	}
	u.selectedInfo.SetText(strings.Join(lines, "\n"))
}

func (u *App) showProfileDialog(title string, existing *config.Profile, onSave func(config.Profile, bool)) {
	name := widget.NewEntry()
	enabled := widget.NewCheck("启用此配置", nil)
	setDefault := widget.NewCheck("设为默认启动", nil)
	configPath := widget.NewEntry()
	frpcPath := widget.NewEntry()
	remoteConfig := widget.NewEntry()
	requireStatus := widget.NewCheck("启用健康检查（推荐）", nil)

	name.SetPlaceHolder("可选，留空将自动按配置文件名生成")
	configPath.SetPlaceHolder("必填，例如 /path/to/frpc.toml")
	frpcPath.SetPlaceHolder("可选，留空使用内置或系统 frpc")
	remoteConfig.SetPlaceHolder("可选，云端配置路径")

	if existing != nil {
		name.SetText(existing.Name)
		enabled.SetChecked(existing.Enabled)
		setDefault.SetChecked(u.cfg.ActiveProfile == existing.Name)
		configPath.SetText(existing.ConfigPath)
		frpcPath.SetText(existing.FrpcPath)
		remoteConfig.SetText(existing.RemoteConfigPath)
		requireStatus.SetChecked(existing.RequireStatus)
	} else {
		enabled.SetChecked(true)
		setDefault.SetChecked(len(u.cfg.Profiles) == 0)
		requireStatus.SetChecked(true)
	}

	configRow := u.filePickerRow(configPath, storage.NewExtensionFileFilter([]string{".toml", ".ini", ".yaml", ".yml", ".json"}))
	frpcRow := u.filePickerRow(frpcPath, nil)
	tip := widget.NewLabel("只需要选择配置文件即可运行。其余设置保持默认即可。")
	tip.Wrapping = fyne.TextWrapWord

	basicForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "配置文件", Widget: configRow},
			{Text: "名称", Widget: name},
			{Text: "状态", Widget: enabled},
			{Text: "默认", Widget: setDefault},
		},
	}

	advancedForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "健康检查", Widget: requireStatus},
			{Text: "frpc 路径", Widget: frpcRow},
			{Text: "云端路径", Widget: remoteConfig},
		},
	}
	advanced := widget.NewAccordion(widget.NewAccordionItem("高级（可选）", advancedForm))
	advanced.CloseAll()

	content := container.NewVBox(tip, basicForm, advanced)
	form := dialog.NewCustomConfirm(title, "保存", "取消", content, func(ok bool) {
		if !ok {
			return
		}
		cfgPath := strings.TrimSpace(configPath.Text)
		if cfgPath == "" {
			dialog.ShowError(fmt.Errorf("请选择配置文件"), u.win)
			return
		}

		var p config.Profile
		if existing != nil {
			p = *existing
		} else {
			p = config.Profile{
				StartTimeoutSec:   8,
				HealthTimeoutSec:  3,
				StatusTimeoutSec:  10,
				StatusIntervalSec: 5,
			}
		}

		p.ConfigPath = cfgPath
		p.Enabled = enabled.Checked
		p.FrpcPath = strings.TrimSpace(frpcPath.Text)
		p.RemoteConfigPath = strings.TrimSpace(remoteConfig.Text)
		p.RequireStatus = requireStatus.Checked

		n := strings.TrimSpace(name.Text)
		if n == "" {
			n = autoProfileName(cfgPath)
		}
		if n == "" {
			dialog.ShowError(fmt.Errorf("名称不能为空"), u.win)
			return
		}
		p.Name = n

		onSave(p, setDefault.Checked)
	}, u.win)
	form.Resize(fyne.NewSize(520, 420))
	form.Show()
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
                    u.profileLabel.SetText("-")
                } else {
                    u.profileLabel.SetText(snap.ProfileName)
                }
                u.errorLabel.SetText(snap.LastError)
                u.healthLabel.SetText(localizeHealth(snap.Health))
                u.healthErr.SetText(snap.HealthError)
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
        syncItem := fyne.NewMenuItem("同步", func() { u.syncWebDAV() })
        quitItem := fyne.NewMenuItem("退出", func() { u.app.Quit() })

        menu := fyne.NewMenu("穿透助手", showItem, hideItem, startItem, stopItem, syncItem, quitItem)
        desk.SetSystemTrayMenu(menu)
    }
}

func (u *App) syncWebDAV() {
    updated, err := webdav.SyncProfiles(u.cfg)
    if err != nil {
        dialog.ShowError(err, u.win)
        return
    }
    if len(updated) == 0 {
        dialog.ShowInformation("WebDAV", "没有需要更新的配置", u.win)
        return
    }

    for i := range u.cfg.Profiles {
        if p, ok := updated[u.cfg.Profiles[i].Name]; ok {
            if u.cfg.Profiles[i].ConfigPath == "" {
                u.cfg.Profiles[i].ConfigPath = p
            }
        }
    }
    _ = config.Save(u.cfg)
    u.mgr.SetConfig(u.cfg)
	dialog.ShowInformation("WebDAV", "同步完成", u.win)
}

func boolToText(v bool) string {
	if v {
		return "是"
	}
	return "否"
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

func localizeHealth(health string) string {
    switch health {
    case "unknown":
        return "未知"
    case "checking":
        return "检查中"
    case "ok":
        return "正常"
    case "fail":
        return "失败"
    case "disabled":
        return "未启用"
    case "stopped":
        return "已停止"
    default:
        return health
    }
}
