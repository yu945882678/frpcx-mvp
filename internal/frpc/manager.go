package frpc

import (
    "bufio"
    "context"
    "errors"
    "fmt"
    "net"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "frpcx/internal/config"
)

type StatusSnapshot struct {
    Status      string
    ProfileName string
    LastError   string
    Health      string
    HealthError string
    LogLines    []string
}

type Manager struct {
    mu           sync.Mutex
    cfg          *config.AppConfig
    cmd          *exec.Cmd
    cancel       context.CancelFunc
    status       string
    profileName  string
    lastError    string
    health       string
    healthError  string
    activeCfg    string
    activeFrpc   string
    logLines     []string
    lastIndex    int
    autoSwitch   bool
    startRunning bool
}

func NewManager(cfg *config.AppConfig) *Manager {
    return &Manager{
        cfg:        cfg,
        status:     "stopped",
        health:     "unknown",
        lastIndex:  -1,
        autoSwitch: cfg.AutoSwitch,
        logLines:   []string{},
    }
}

func (m *Manager) SetConfig(cfg *config.AppConfig) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.cfg = cfg
    m.autoSwitch = cfg.AutoSwitch
}

func (m *Manager) Status() StatusSnapshot {
    m.mu.Lock()
    defer m.mu.Unlock()
    return StatusSnapshot{
        Status:      m.status,
        ProfileName: m.profileName,
        LastError:   m.lastError,
        Health:      m.health,
        HealthError: m.healthError,
        LogLines:    append([]string{}, m.logLines...),
    }
}

func (m *Manager) Start() {
    m.StartAuto()
}

func (m *Manager) StartAuto() {
    m.mu.Lock()
    if m.status == "starting" || m.status == "running" {
        m.mu.Unlock()
        return
    }
    m.status = "starting"
    m.lastError = ""
    auto := m.autoSwitch
    m.mu.Unlock()
    if auto {
        go m.startAutoFromIndex(-1)
    } else {
        go m.startSingle()
    }
}

func (m *Manager) startAutoFromIndex(startIndex int) {
    profiles := enabledProfiles(m.cfg.Profiles)
    if len(profiles) == 0 {
        m.setError("no enabled profiles")
        return
    }

    idx := 0
    if startIndex >= 0 {
        idx = startIndex
    } else if m.cfg.ActiveProfile != "" {
        for i, p := range profiles {
            if p.Name == m.cfg.ActiveProfile {
                idx = i
                break
            }
        }
    }

    for i := 0; i < len(profiles); i++ {
        tryIndex := (idx + i) % len(profiles)
        p := profiles[tryIndex]
        if err := m.startProfile(&p, tryIndex); err != nil {
            m.appendLog(fmt.Sprintf("profile '%s' failed: %v", p.Name, err))
            continue
        }
        return
    }

    m.setError("all profiles failed")
}

func (m *Manager) startSingle() {
    profiles := enabledProfiles(m.cfg.Profiles)
    if len(profiles) == 0 {
        m.setError("no enabled profiles")
        return
    }

    idx := 0
    if m.cfg.ActiveProfile != "" {
        for i, p := range profiles {
            if p.Name == m.cfg.ActiveProfile {
                idx = i
                break
            }
        }
    }

    p := profiles[idx]
    if err := m.startProfile(&p, idx); err != nil {
        m.setError(err.Error())
    }
}

func (m *Manager) StartNext() {
    m.mu.Lock()
    nextIndex := m.lastIndex + 1
    m.mu.Unlock()
    go m.startAutoFromIndex(nextIndex)
}

func (m *Manager) Stop() {
    m.mu.Lock()
    if m.cancel != nil {
        m.cancel()
    }
    cmd := m.cmd
    m.cmd = nil
    m.status = "stopped"
    m.profileName = ""
    m.health = "stopped"
    m.healthError = ""
    m.activeCfg = ""
    m.activeFrpc = ""
    m.mu.Unlock()

    if cmd != nil && cmd.Process != nil {
        _ = cmd.Process.Kill()
    }
}

func (m *Manager) startProfile(p *config.Profile, index int) error {
    if err := preCheck(p); err != nil {
        return err
    }

    cfgPath, err := resolveConfigPath(p)
    if err != nil {
        return err
    }

    frpcPath, err := ResolveBinaryPath(p.FrpcPath)
    if err != nil {
        return err
    }

    ctx, cancel := context.WithCancel(context.Background())
    cmd := exec.CommandContext(ctx, frpcPath, append([]string{"-c", cfgPath}, p.ExtraArgs...)...)
    cmd.Env = os.Environ()

    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()

    if err := cmd.Start(); err != nil {
        cancel()
        return err
    }

    m.mu.Lock()
    m.cmd = cmd
    m.cancel = cancel
    m.status = "starting"
    m.profileName = p.Name
    m.lastIndex = index
    m.activeCfg = cfgPath
    m.activeFrpc = frpcPath
    m.mu.Unlock()

    readyCh := make(chan struct{})
    failCh := make(chan error, 1)

    startTimeout := time.Duration(defaultInt(p.StartTimeoutSec, 8)) * time.Second

    scan := func(r *bufio.Scanner) {
        for r.Scan() {
            line := r.Text()
            m.appendLog(line)
            if ok, err := classifyLog(line); ok {
                select {
                case <-readyCh:
                default:
                    close(readyCh)
                }
            } else if err != nil {
                select {
                case failCh <- err:
                default:
                }
            }
        }
    }

    go scan(bufio.NewScanner(stdout))
    go scan(bufio.NewScanner(stderr))

    exitCh := make(chan error, 1)
    go func() {
        exitCh <- cmd.Wait()
    }()

    select {
    case err := <-failCh:
        m.appendLog(fmt.Sprintf("startup failed: %v", err))
        _ = cmd.Process.Kill()
        return err
    case <-readyCh:
        m.setRunning(p.Name)
        if p.RequireStatus {
            m.setHealth("checking", "")
        } else {
            m.setHealth("disabled", "")
        }
        if err := m.waitForStatusOK(frpcPath, cfgPath, p); err != nil {
            m.appendLog(fmt.Sprintf("status check failed: %v", err))
            m.setHealth("fail", err.Error())
            _ = cmd.Process.Kill()
            return err
        }
    case <-time.After(startTimeout):
        err := errors.New("start timeout")
        m.appendLog("start timeout")
        _ = cmd.Process.Kill()
        return err
    case err := <-exitCh:
        if err != nil {
            return fmt.Errorf("process exited early: %w", err)
        }
        return errors.New("process exited")
    }

    go func() {
        err := <-exitCh
        if err == nil {
            m.setError("process exited")
        } else {
            m.setError(fmt.Sprintf("process exited: %v", err))
        }
        if m.autoSwitch {
            m.StartNext()
        }
    }()

    if p.RequireStatus {
        go m.monitorStatus(ctx, frpcPath, cfgPath, p)
    }

    return nil
}

func enabledProfiles(profiles []config.Profile) []config.Profile {
    out := make([]config.Profile, 0, len(profiles))
    for _, p := range profiles {
        if p.Enabled {
            out = append(out, p)
        }
    }
    return out
}

func preCheck(p *config.Profile) error {
    if p.ServerAddr != "" && p.ServerPort > 0 {
        addr := fmt.Sprintf("%s:%d", p.ServerAddr, p.ServerPort)
        d := time.Duration(defaultInt(p.HealthTimeoutSec, 5)) * time.Second
        conn, err := net.DialTimeout("tcp", addr, d)
        if err != nil {
            return fmt.Errorf("server connect failed: %w", err)
        }
        _ = conn.Close()
    }

    if len(p.LocalCheckPorts) > 0 {
        d := time.Duration(defaultInt(p.HealthTimeoutSec, 2)) * time.Second
        for _, port := range p.LocalCheckPorts {
            addr := fmt.Sprintf("127.0.0.1:%d", port)
            conn, err := net.DialTimeout("tcp", addr, d)
            if err != nil {
                return fmt.Errorf("local service not reachable on %d: %w", port, err)
            }
            _ = conn.Close()
        }
    }

    return nil
}

func classifyLog(line string) (bool, error) {
    l := strings.ToLower(line)
    if strings.Contains(l, "login to server success") || strings.Contains(l, "start proxy success") || strings.Contains(l, "proxy added") {
        return true, nil
    }

    failurePatterns := []string{
        "port already used",
        "proxy name" + " already exists",
        "connect to local service",
        "connection refused",
        "i/o timeout",
        "timeout",
        "login to server failed",
        "authentication failed",
        "invalid token",
        "failed to" ,
    }
    for _, p := range failurePatterns {
        if strings.Contains(l, p) {
            return false, errors.New(strings.TrimSpace(line))
        }
    }

    if strings.Contains(l, "error") && strings.Contains(l, "proxy") {
        return false, errors.New(strings.TrimSpace(line))
    }

    return false, nil
}

func (m *Manager) appendLog(line string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.logLines = append(m.logLines, line)
    if len(m.logLines) > 200 {
        m.logLines = m.logLines[len(m.logLines)-200:]
    }
}

func (m *Manager) setRunning(profile string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.status = "running"
    m.profileName = profile
    m.lastError = ""
}

func (m *Manager) setError(msg string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.status = "error"
    m.lastError = msg
}

func (m *Manager) setHealth(status, err string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.health = status
    m.healthError = err
}

func (m *Manager) failAndSwitch(msg string) {
    m.setError(msg)
    m.setHealth("fail", msg)
    m.mu.Lock()
    cmd := m.cmd
    m.mu.Unlock()
    if cmd != nil && cmd.Process != nil {
        _ = cmd.Process.Kill()
    }
    if m.autoSwitch {
        m.StartNext()
    }
}

func defaultInt(v, d int) int {
    if v <= 0 {
        return d
    }
    return v
}

func resolveConfigPath(p *config.Profile) (string, error) {
    cfgPath := p.ConfigPath
    if cfgPath == "" && p.RemoteConfigPath != "" {
        if local, err := cachedConfigPath(p.Name); err == nil {
            cfgPath = local
        }
    }
    if cfgPath == "" {
        return "", errors.New("config path is empty")
    }
    if _, err := os.Stat(cfgPath); err != nil {
        return "", fmt.Errorf("config not found: %w", err)
    }
    return cfgPath, nil
}

func (m *Manager) waitForStatusOK(frpcPath, cfgPath string, p *config.Profile) error {
    if !p.RequireStatus {
        return nil
    }
    timeout := time.Duration(defaultInt(p.StatusTimeoutSec, 10)) * time.Second
    deadline := time.Now().Add(timeout)
    var lastErr error
    for time.Now().Before(deadline) {
        if err := m.checkStatusOnce(frpcPath, cfgPath, p); err == nil {
            m.appendLog("status check ok")
            m.setHealth("ok", "")
            return nil
        } else {
            lastErr = err
            m.setHealth("fail", err.Error())
        }
        time.Sleep(500 * time.Millisecond)
    }
    if lastErr != nil {
        return lastErr
    }
    return errors.New("status check timeout")
}

func (m *Manager) monitorStatus(ctx context.Context, frpcPath, cfgPath string, p *config.Profile) {
    interval := time.Duration(defaultInt(p.StatusIntervalSec, 5)) * time.Second
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    failures := 0
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := m.checkStatusOnce(frpcPath, cfgPath, p); err != nil {
                failures++
                m.setHealth("fail", err.Error())
                if failures >= 3 {
                    m.appendLog(fmt.Sprintf("status monitor failed: %v", err))
                    m.failAndSwitch("status monitor failed")
                    return
                }
            } else {
                failures = 0
                m.setHealth("ok", "")
            }
        }
    }
}

func (m *Manager) checkStatusOnce(frpcPath, cfgPath string, p *config.Profile) error {
    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(defaultInt(p.HealthTimeoutSec, 3))*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, frpcPath, "status", "-c", cfgPath)
    out, err := cmd.CombinedOutput()
    if err != nil {
        msg := strings.TrimSpace(string(out))
        if msg == "" {
            msg = err.Error()
        }
        return errors.New(msg)
    }
    return nil
}

func (m *Manager) CheckStatusNow() error {
    m.mu.Lock()
    name := m.profileName
    cfg := m.cfg
    m.mu.Unlock()

    if name == "" {
        return errors.New("no running profile")
    }
    var p *config.Profile
    for i := range cfg.Profiles {
        if cfg.Profiles[i].Name == name {
            p = &cfg.Profiles[i]
            break
        }
    }
    if p == nil {
        return errors.New("active profile not found")
    }
    cfgPath, err := resolveConfigPath(p)
    if err != nil {
        m.setHealth("fail", err.Error())
        return err
    }
    frpcPath, err := ResolveBinaryPath(p.FrpcPath)
    if err != nil {
        m.setHealth("fail", err.Error())
        return err
    }
    if err := m.checkStatusOnce(frpcPath, cfgPath, p); err != nil {
        m.setHealth("fail", err.Error())
        return err
    }
    m.setHealth("ok", "")
    return nil
}

func cachedConfigPath(name string) (string, error) {
    dir, err := config.CacheDir()
    if err != nil {
        return "", err
    }
    safe := strings.ReplaceAll(strings.ToLower(name), " ", "_")
    return filepath.Join(dir, safe+".toml"), nil
}
