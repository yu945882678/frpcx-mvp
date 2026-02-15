package ui

import (
	"fmt"
	"strconv"
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

	win.Resize(fyne.NewSize(980, 700))
	win.ShowAndRun()
}

func (u *App) build() {
	u.statusLabel = widget.NewLabelWithStyle("已停止", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.profileLabel = widget.NewLabelWithStyle("-", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.errorLabel = widget.NewLabel("")
	u.healthLabel = widget.NewLabelWithStyle("未知", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	u.healthErr = widget.NewLabel("")
	u.logEntry = widget.NewMultiLineEntry()
	u.logEntry.SetMinRowsVisible(10)
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

	headline := container.NewVBox(
		widget.NewLabelWithStyle("穿透助手", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("极简跨端 FRPC 控制台"),
	)
	headlineCard := widget.NewCard("", "", headline)

	metrics := container.NewGridWithColumns(2,
		u.metricCard("运行状态", u.statusLabel, u.errorLabel),
		u.metricCard("健康检查", u.healthLabel, u.healthErr),
	)
	profileCard := widget.NewCard("当前配置", "", u.profileLabel)

	actionsRow := container.NewGridWithColumns(2, startBtn, stopBtn)
	toolsRow := container.NewGridWithColumns(2, syncBtn, checkBtn)
	autoRow := container.NewHBox(autoSwitch)
	actionsCard := widget.NewCard("快捷操作", "", container.NewVBox(actionsRow, toolsRow, autoRow))
	logsCard := widget.NewCard("日志", "", u.logEntry)

	statusTab := container.NewVBox(
		headlineCard,
		metrics,
		profileCard,
		actionsCard,
		logsCard,
		layout.NewSpacer(),
    )

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
	u.selectedInfo = widget.NewLabel("请选择左侧配置查看摘要")
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
            o.(*widget.Label).SetText(p.Name + status)
        },
    )

	u.list.OnSelected = func(id widget.ListItemID) {
		u.selectedIdx = id
		u.refreshSelectedProfileInfo()
	}

    addBtn := widget.NewButton("添加", func() {
        u.showProfileDialog("添加配置", nil, func(p config.Profile) {
			u.cfg.Profiles = append(u.cfg.Profiles, p)
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
        u.showProfileDialog("编辑配置", &p, func(updated config.Profile) {
			u.cfg.Profiles[u.selectedIdx] = updated
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
        u.cfg.Profiles = append(u.cfg.Profiles[:u.selectedIdx], u.cfg.Profiles[u.selectedIdx+1:]...)
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
		u.refreshSelectedProfileInfo()
	})

	buttonsMain := container.NewGridWithColumns(2, addBtn, editBtn)
	buttonsExtra := container.NewGridWithColumns(2, removeBtn, setActiveBtn)
	buttonsOrder := container.NewGridWithColumns(2, upBtn, downBtn)
	listCard := widget.NewCard("配置列表", "", u.list)
	detailCard := widget.NewCard("配置摘要", "", u.selectedInfo)

	left := container.NewVBox(listCard, buttonsMain, buttonsExtra, buttonsOrder)
	right := container.NewVBox(detailCard, layout.NewSpacer())
	split := container.NewHSplit(left, right)
	split.SetOffset(0.58)
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

func (u *App) metricCard(title string, value *widget.Label, detail *widget.Label) fyne.CanvasObject {
	value.Wrapping = fyne.TextWrapWord
	detail.Wrapping = fyne.TextWrapWord
	return widget.NewCard(title, "", container.NewVBox(value, detail))
}

func (u *App) refreshSelectedProfileInfo() {
	if u.selectedInfo == nil {
		return
	}
	if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
		u.selectedInfo.SetText("请选择左侧配置查看摘要")
		return
	}
	p := u.cfg.Profiles[u.selectedIdx]
	lines := []string{
		fmt.Sprintf("名称: %s", p.Name),
		fmt.Sprintf("启用: %t", p.Enabled),
	}
	if p.ConfigPath != "" {
		lines = append(lines, fmt.Sprintf("配置文件: %s", p.ConfigPath))
	} else {
		lines = append(lines, "配置文件: 未设置")
	}
	if p.ServerAddr != "" && p.ServerPort > 0 {
		lines = append(lines, fmt.Sprintf("服务器: %s:%d", p.ServerAddr, p.ServerPort))
	}
	if len(p.LocalCheckPorts) > 0 {
		lines = append(lines, fmt.Sprintf("本地检查端口: %s", intSliceToString(p.LocalCheckPorts)))
	}
	if p.RequireStatus {
		lines = append(lines, "状态检查: 已启用")
	} else {
		lines = append(lines, "状态检查: 未启用")
	}
	u.selectedInfo.SetText(strings.Join(lines, "\n"))
}

func (u *App) showProfileDialog(title string, existing *config.Profile, onSave func(config.Profile)) {
    name := widget.NewEntry()
    enabled := widget.NewCheck("启用", nil)
    frpcPath := widget.NewEntry()
    configPath := widget.NewEntry()
    remoteConfig := widget.NewEntry()
    serverAddr := widget.NewEntry()
    serverPort := widget.NewEntry()
    localPorts := widget.NewEntry()
    startTimeout := widget.NewEntry()
    healthTimeout := widget.NewEntry()
    requireStatus := widget.NewCheck("启用状态检查", nil)
    statusTimeout := widget.NewEntry()
    statusInterval := widget.NewEntry()
    extraArgs := widget.NewEntry()

    name.SetPlaceHolder("必填，例如 家庭线路")
    configPath.SetPlaceHolder("必填，例如 /path/frpc.toml")
    frpcPath.SetPlaceHolder("可选，留空使用内置/系统 frpc")
    remoteConfig.SetPlaceHolder("可选，WebDAV 远程路径")
    serverAddr.SetPlaceHolder("可选，例如 1.2.3.4")
    serverPort.SetPlaceHolder("可选")
    localPorts.SetPlaceHolder("可选，例 8080,8443")
    startTimeout.SetPlaceHolder("可选，默认 8")
    healthTimeout.SetPlaceHolder("可选，默认 5")
    statusTimeout.SetPlaceHolder("可选，默认 10")
    statusInterval.SetPlaceHolder("可选，默认 5")
    extraArgs.SetPlaceHolder("可选，例如 -u token")

    if existing != nil {
        name.SetText(existing.Name)
        enabled.SetChecked(existing.Enabled)
        frpcPath.SetText(existing.FrpcPath)
        configPath.SetText(existing.ConfigPath)
        remoteConfig.SetText(existing.RemoteConfigPath)
        serverAddr.SetText(existing.ServerAddr)
        if existing.ServerPort > 0 {
            serverPort.SetText(fmt.Sprintf("%d", existing.ServerPort))
        }
        if len(existing.LocalCheckPorts) > 0 {
            localPorts.SetText(intSliceToString(existing.LocalCheckPorts))
        }
        if existing.StartTimeoutSec > 0 {
            startTimeout.SetText(fmt.Sprintf("%d", existing.StartTimeoutSec))
        }
        if existing.HealthTimeoutSec > 0 {
            healthTimeout.SetText(fmt.Sprintf("%d", existing.HealthTimeoutSec))
        }
        requireStatus.SetChecked(existing.RequireStatus)
        if existing.StatusTimeoutSec > 0 {
            statusTimeout.SetText(fmt.Sprintf("%d", existing.StatusTimeoutSec))
        }
        if existing.StatusIntervalSec > 0 {
            statusInterval.SetText(fmt.Sprintf("%d", existing.StatusIntervalSec))
        }
        if len(existing.ExtraArgs) > 0 {
            extraArgs.SetText(strings.Join(existing.ExtraArgs, " "))
        }
    } else {
        enabled.SetChecked(true)
    }

    frpcRow := u.filePickerRow(frpcPath, nil)
    configRow := u.filePickerRow(configPath, storage.NewExtensionFileFilter([]string{".toml", ".ini", ".yaml", ".yml", ".json"}))

    basicForm := &widget.Form{
        Items: []*widget.FormItem{
            {Text: "名称", Widget: name},
            {Text: "启用", Widget: enabled},
            {Text: "配置文件（必填）", Widget: configRow},
        },
        SubmitText: "",
        OnSubmit:   nil,
    }

    advancedForm := &widget.Form{
        Items: []*widget.FormItem{
            {Text: "frpc 路径", Widget: frpcRow},
            {Text: "远程配置", Widget: remoteConfig},
            {Text: "服务器地址", Widget: serverAddr},
            {Text: "服务器端口", Widget: serverPort},
            {Text: "本地端口", Widget: localPorts},
            {Text: "启动超时(秒)", Widget: startTimeout},
            {Text: "健康超时(秒)", Widget: healthTimeout},
            {Text: "状态检查", Widget: requireStatus},
            {Text: "状态超时(秒)", Widget: statusTimeout},
            {Text: "状态间隔(秒)", Widget: statusInterval},
            {Text: "额外参数", Widget: extraArgs},
        },
        SubmitText: "",
        OnSubmit:   nil,
    }

    advanced := widget.NewAccordion(widget.NewAccordionItem("高级设置（可选）", advancedForm))
    advanced.CloseAll()

    content := container.NewVBox(basicForm, advanced)

    form := dialog.NewCustomConfirm(title, "保存", "取消", content, func(ok bool) {
        if !ok {
            return
        }
        p := config.Profile{
            Name:             strings.TrimSpace(name.Text),
            Enabled:          enabled.Checked,
            FrpcPath:         strings.TrimSpace(frpcPath.Text),
            ConfigPath:       strings.TrimSpace(configPath.Text),
            RemoteConfigPath: strings.TrimSpace(remoteConfig.Text),
            ServerAddr:       strings.TrimSpace(serverAddr.Text),
            ServerPort:       parseInt(serverPort.Text),
            LocalCheckPorts:  parsePorts(localPorts.Text),
            StartTimeoutSec:  parseInt(startTimeout.Text),
            HealthTimeoutSec: parseInt(healthTimeout.Text),
            RequireStatus:    requireStatus.Checked,
            StatusTimeoutSec:  parseInt(statusTimeout.Text),
            StatusIntervalSec: parseInt(statusInterval.Text),
            ExtraArgs:        parseArgs(extraArgs.Text),
        }
        if p.Name == "" {
            dialog.ShowError(fmt.Errorf("名称不能为空"), u.win)
            return
        }
        if p.ConfigPath == "" {
            dialog.ShowError(fmt.Errorf("配置文件不能为空"), u.win)
            return
        }
        onSave(p)
    }, u.win)

    form.Resize(fyne.NewSize(480, 520))
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

func parseInt(s string) int {
    s = strings.TrimSpace(s)
    if s == "" {
        return 0
    }
    v, _ := strconv.Atoi(s)
    return v
}

func parsePorts(s string) []int {
    s = strings.TrimSpace(s)
    if s == "" {
        return nil
    }
    parts := strings.Split(s, ",")
    out := make([]int, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        if v, err := strconv.Atoi(p); err == nil {
            out = append(out, v)
        }
    }
    return out
}

func parseArgs(s string) []string {
    s = strings.TrimSpace(s)
    if s == "" {
        return nil
    }
    return strings.Fields(s)
}

func intSliceToString(v []int) string {
    parts := make([]string, 0, len(v))
    for _, n := range v {
        parts = append(parts, strconv.Itoa(n))
    }
    return strings.Join(parts, ",")
}
