package config

import (
    "encoding/json"
    "os"
    "path/filepath"
)

type AppConfig struct {
    Version        int          `json:"version"`
    AutoSwitch     bool         `json:"auto_switch"`
    ActiveProfile  string       `json:"active_profile"`
    Profiles       []Profile    `json:"profiles"`
    WebDAV         WebDAVConfig `json:"webdav"`
}

type WebDAVConfig struct {
    URL        string `json:"url"`
    Username   string `json:"username"`
    Password   string `json:"password"`
    RemoteBase string `json:"remote_base"`
}

type Profile struct {
    Name              string   `json:"name"`
    Enabled           bool     `json:"enabled"`
    FrpcPath          string   `json:"frpc_path"`
    ConfigPath        string   `json:"config_path"`
    RemoteConfigPath  string   `json:"remote_config_path"`
    ServerAddr        string   `json:"server_addr"`
    ServerPort        int      `json:"server_port"`
    LocalCheckPorts   []int    `json:"local_check_ports"`
    StartTimeoutSec   int      `json:"start_timeout_sec"`
    HealthTimeoutSec  int      `json:"health_timeout_sec"`
    RequireStatus     bool     `json:"require_status"`
    StatusTimeoutSec  int      `json:"status_timeout_sec"`
    StatusIntervalSec int      `json:"status_interval_sec"`
    ExtraArgs         []string `json:"extra_args"`
}

func DefaultConfig() *AppConfig {
    return &AppConfig{
        Version:       1,
        AutoSwitch:    true,
        ActiveProfile: "",
        Profiles:      []Profile{},
        WebDAV:        WebDAVConfig{},
    }
}

func ConfigDir() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "frpcx"), nil
}

func ConfigPath() (string, error) {
    dir, err := ConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "config.json"), nil
}

func CacheDir() (string, error) {
    dir, err := ConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "cache"), nil
}

func (c *AppConfig) Clone() *AppConfig {
    b, _ := json.Marshal(c)
    var out AppConfig
    _ = json.Unmarshal(b, &out)
    return &out
}
