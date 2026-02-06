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
    a := app.NewWithID("frpcx")
    win := a.NewWindow("frpcx")
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
    u.statusLabel = widget.NewLabel("stopped")
    u.profileLabel = widget.NewLabel("-")
    u.errorLabel = widget.NewLabel("")
    u.healthLabel = widget.NewLabel("unknown")
    u.healthErr = widget.NewLabel("")
    u.logEntry = widget.NewMultiLineEntry()
    u.logEntry.SetMinRowsVisible(8)
    u.logEntry.Wrapping = fyne.TextWrapOff
    u.logEntry.Disable()

    startBtn := widget.NewButton("Start", func() {
        u.mgr.StartAuto()
    })
    stopBtn := widget.NewButton("Stop", func() {
        u.mgr.Stop()
    })
    syncBtn := widget.NewButton("Sync WebDAV", func() {
        u.syncWebDAV()
    })
    checkBtn := widget.NewButton("Check Status", func() {
        if err := u.mgr.CheckStatusNow(); err != nil {
            dialog.ShowError(err, u.win)
            return
        }
        dialog.ShowInformation("Status", "OK", u.win)
    })
    autoSwitch := widget.NewCheck("Auto Switch", func(v bool) {
        u.cfg.AutoSwitch = v
        u.mgr.SetConfig(u.cfg)
        _ = config.Save(u.cfg)
    })
    autoSwitch.SetChecked(u.cfg.AutoSwitch)

    statusGrid := container.NewGridWithColumns(2,
        widget.NewLabel("Status"), u.statusLabel,
        widget.NewLabel("Profile"), u.profileLabel,
        widget.NewLabel("Last Error"), u.errorLabel,
        widget.NewLabel("Health"), u.healthLabel,
        widget.NewLabel("Health Error"), u.healthErr,
    )

    statusTab := container.NewVBox(
        statusGrid,
        container.NewHBox(startBtn, stopBtn, syncBtn, checkBtn, autoSwitch),
        widget.NewLabel("Logs"),
        u.logEntry,
    )

    profilesTab := u.buildProfilesTab()
    webdavTab := u.buildWebDAVTab()

    tabs := container.NewAppTabs(
        container.NewTabItem("Status", statusTab),
        container.NewTabItem("Profiles", profilesTab),
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
                status = " (disabled)"
            }
            o.(*widget.Label).SetText(p.Name + status)
        },
    )

    u.list.OnSelected = func(id widget.ListItemID) {
        u.selectedIdx = id
    }

    addBtn := widget.NewButton("Add", func() {
        u.showProfileDialog("Add Profile", nil, func(p config.Profile) {
            u.cfg.Profiles = append(u.cfg.Profiles, p)
            _ = config.Save(u.cfg)
            u.mgr.SetConfig(u.cfg)
            u.list.Refresh()
        })
    })

    editBtn := widget.NewButton("Edit", func() {
        if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
            return
        }
        p := u.cfg.Profiles[u.selectedIdx]
        u.showProfileDialog("Edit Profile", &p, func(updated config.Profile) {
            u.cfg.Profiles[u.selectedIdx] = updated
            _ = config.Save(u.cfg)
            u.mgr.SetConfig(u.cfg)
            u.list.Refresh()
        })
    })

    removeBtn := widget.NewButton("Remove", func() {
        if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles) {
            return
        }
        u.cfg.Profiles = append(u.cfg.Profiles[:u.selectedIdx], u.cfg.Profiles[u.selectedIdx+1:]...)
        u.selectedIdx = -1
        _ = config.Save(u.cfg)
        u.mgr.SetConfig(u.cfg)
        u.list.Refresh()
    })

    upBtn := widget.NewButton("Up", func() {
        if u.selectedIdx <= 0 || u.selectedIdx >= len(u.cfg.Profiles) {
            return
        }
        u.cfg.Profiles[u.selectedIdx-1], u.cfg.Profiles[u.selectedIdx] = u.cfg.Profiles[u.selectedIdx], u.cfg.Profiles[u.selectedIdx-1]
        u.selectedIdx = u.selectedIdx - 1
        _ = config.Save(u.cfg)
        u.mgr.SetConfig(u.cfg)
        u.list.Refresh()
    })

    downBtn := widget.NewButton("Down", func() {
        if u.selectedIdx < 0 || u.selectedIdx >= len(u.cfg.Profiles)-1 {
            return
        }
        u.cfg.Profiles[u.selectedIdx+1], u.cfg.Profiles[u.selectedIdx] = u.cfg.Profiles[u.selectedIdx], u.cfg.Profiles[u.selectedIdx+1]
        u.selectedIdx = u.selectedIdx + 1
        _ = config.Save(u.cfg)
        u.mgr.SetConfig(u.cfg)
        u.list.Refresh()
    })

    setActiveBtn := widget.NewButton("Set Active", func() {
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

    saveBtn := widget.NewButton("Save", func() {
        u.cfg.WebDAV.URL = strings.TrimSpace(urlEntry.Text)
        u.cfg.WebDAV.Username = strings.TrimSpace(userEntry.Text)
        u.cfg.WebDAV.Password = passEntry.Text
        u.cfg.WebDAV.RemoteBase = strings.TrimSpace(baseEntry.Text)
        _ = config.Save(u.cfg)
        u.mgr.SetConfig(u.cfg)
    })

    syncBtn := widget.NewButton("Sync Now", func() {
        u.syncWebDAV()
    })

    form := &widget.Form{
        Items: []*widget.FormItem{
            {Text: "URL", Widget: urlEntry},
            {Text: "Username", Widget: userEntry},
            {Text: "Password", Widget: passEntry},
            {Text: "Remote Base", Widget: baseEntry},
        },
        SubmitText: "",
        OnSubmit:   nil,
    }

    return container.NewVBox(form, container.NewHBox(saveBtn, syncBtn))
}

func (u *App) showProfileDialog(title string, existing *config.Profile, onSave func(config.Profile)) {
    name := widget.NewEntry()
    enabled := widget.NewCheck("Enabled", nil)
    frpcPath := widget.NewEntry()
    configPath := widget.NewEntry()
    remoteConfig := widget.NewEntry()
    serverAddr := widget.NewEntry()
    serverPort := widget.NewEntry()
    localPorts := widget.NewEntry()
    startTimeout := widget.NewEntry()
    healthTimeout := widget.NewEntry()
    requireStatus := widget.NewCheck("Require Status Check", nil)
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

    form := dialog.NewForm(title, "Save", "Cancel", []*widget.FormItem{
        {Text: "Name", Widget: name},
        {Text: "Enabled", Widget: enabled},
        {Text: "frpc Path", Widget: frpcRow},
        {Text: "Config Path", Widget: configRow},
        {Text: "Remote Config", Widget: remoteConfig},
        {Text: "Server Addr", Widget: serverAddr},
        {Text: "Server Port", Widget: serverPort},
        {Text: "Local Ports", Widget: localPorts},
        {Text: "Start Timeout", Widget: startTimeout},
        {Text: "Health Timeout", Widget: healthTimeout},
        {Text: "Status Check", Widget: requireStatus},
        {Text: "Status Timeout", Widget: statusTimeout},
        {Text: "Status Interval", Widget: statusInterval},
        {Text: "Extra Args", Widget: extraArgs},
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
            dialog.ShowError(fmt.Errorf("name is required"), u.win)
            return
        }
        onSave(p)
    }, u.win)

    form.Resize(fyne.NewSize(460, 520))
    form.Show()
}

func (u *App) filePickerRow(entry *widget.Entry, filter storage.FileFilter) fyne.CanvasObject {
    browse := widget.NewButton("Browse", func() {
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
                u.statusLabel.SetText(snap.Status)
                if snap.ProfileName == "" {
                    u.profileLabel.SetText("-")
                } else {
                    u.profileLabel.SetText(snap.ProfileName)
                }
                u.errorLabel.SetText(snap.LastError)
                u.healthLabel.SetText(snap.Health)
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
        showItem := fyne.NewMenuItem("Show", func() { u.win.Show() })
        hideItem := fyne.NewMenuItem("Hide", func() { u.win.Hide() })
        startItem := fyne.NewMenuItem("Start", func() { u.mgr.StartAuto() })
        stopItem := fyne.NewMenuItem("Stop", func() { u.mgr.Stop() })
        syncItem := fyne.NewMenuItem("Sync", func() { u.syncWebDAV() })
        quitItem := fyne.NewMenuItem("Quit", func() { u.app.Quit() })

        menu := fyne.NewMenu("frpcx", showItem, hideItem, startItem, stopItem, syncItem, quitItem)
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
        dialog.ShowInformation("WebDAV", "No profiles updated", u.win)
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
    dialog.ShowInformation("WebDAV", "Sync completed", u.win)
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
