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
    "fyne.io/fyne/v2/storage"
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
    logEntry     *widget.Entry
    list         *widget.List
    selectedIdx  int
}

func Run(cfg *config.AppConfig) {
    a := app.NewWithID("suidaohe")
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

    win.Resize(fyne.NewSize(520, 520))
    win.ShowAndRun()
}

func (u *App) build() {
    u.statusLabel = widget.NewLabel("已停止")
    u.profileLabel = widget.NewLabel("-")
    u.errorLabel = widget.NewLabel("")
    u.healthLabel = widget.NewLabel("未知")
    u.healthErr = widget.NewLabel("")
    u.logEntry = widget.NewMultiLineEntry()
    u.logEntry.SetMinRowsVisible(8)
    u.logEntry.Wrapping = fyne.TextWrapOff
    u.logEntry.Disable()

    startBtn := widget.NewButton("启动", func() {
        u.mgr.StartAuto()
    })
    stopBtn := widget.NewButton("停止", func() {
        u.mgr.Stop()
    })
    syncBtn := widget.NewButton("同步 WebDAV", func() {
        u.syncWebDAV()
    })
    checkBtn := widget.NewButton("检查状态", func() {
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

    statusGrid := container.NewGridWithColumns(2,
        widget.NewLabel("状态"), u.statusLabel,
        widget.NewLabel("配置"), u.profileLabel,
        widget.NewLabel("最近错误"), u.errorLabel,
        widget.NewLabel("健康"), u.healthLabel,
        widget.NewLabel("健康错误"), u.healthErr,
    )

    statusTab := container.NewVBox(
        statusGrid,
        container.NewHBox(startBtn, stopBtn, syncBtn, checkBtn, autoSwitch),
        widget.NewLabel("日志"),
        u.logEntry,
    )

    profilesTab := u.buildProfilesTab()
    webdavTab := u.buildWebDAVTab()

    tabs := container.NewAppTabs(
        container.NewTabItem("状态", statusTab),
        container.NewTabItem("配置", profilesTab),
        container.NewTabItem("WebDAV", webdavTab),
    )

    u.win.SetContent(tabs)
}

func (u *App) buildProfilesTab() fyne.CanvasObject {
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
    }

    addBtn := widget.NewButton("添加", func() {
        u.showProfileDialog("添加配置", nil, func(p config.Profile) {
            u.cfg.Profiles = append(u.cfg.Profiles, p)
            _ = config.Save(u.cfg)
            u.mgr.SetConfig(u.cfg)
            u.list.Refresh()
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
    })

    setActiveBtn := widget.NewButton("设为默认", func() {
        if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
            return
        }
        u.cfg.ActiveProfile = u.cfg.Profiles[u.selectedIdx].Name
        _ = config.Save(u.cfg)
        u.mgr.SetConfig(u.cfg)
    })

    buttons := container.NewHBox(addBtn, editBtn, removeBtn, upBtn, downBtn, setActiveBtn)

    return container.NewBorder(buttons, nil, nil, nil, u.list)
}

func (u *App) buildWebDAVTab() fyne.CanvasObject {
    urlEntry := widget.NewEntry()
    urlEntry.SetText(u.cfg.WebDAV.URL)
    userEntry := widget.NewEntry()
    userEntry.SetText(u.cfg.WebDAV.Username)
    passEntry := widget.NewPasswordEntry()
    passEntry.SetText(u.cfg.WebDAV.Password)
    baseEntry := widget.NewEntry()
    baseEntry.SetText(u.cfg.WebDAV.RemoteBase)

    saveBtn := widget.NewButton("保存", func() {
        u.cfg.WebDAV.URL = strings.TrimSpace(urlEntry.Text)
        u.cfg.WebDAV.Username = strings.TrimSpace(userEntry.Text)
        u.cfg.WebDAV.Password = passEntry.Text
        u.cfg.WebDAV.RemoteBase = strings.TrimSpace(baseEntry.Text)
        _ = config.Save(u.cfg)
        u.mgr.SetConfig(u.cfg)
    })

    syncBtn := widget.NewButton("立即同步", func() {
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

    return container.NewVBox(form, container.NewHBox(saveBtn, syncBtn))
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

    form := dialog.NewForm(title, "保存", "取消", []*widget.FormItem{
        {Text: "名称", Widget: name},
        {Text: "启用", Widget: enabled},
        {Text: "frpc 路径", Widget: frpcRow},
        {Text: "配置文件", Widget: configRow},
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
    }, func(ok bool) {
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
        onSave(p)
    }, u.win)

    form.Resize(fyne.NewSize(460, 520))
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
