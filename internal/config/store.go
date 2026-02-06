package config

import (
    "encoding/json"
    "errors"
    "os"
)

func Load() (*AppConfig, error) {
    path, err := ConfigPath()
    if err != nil {
        return nil, err
    }
    b, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            cfg := DefaultConfig()
            _ = Save(cfg)
            return cfg, nil
        }
        return nil, err
    }
    var cfg AppConfig
    if err := json.Unmarshal(b, &cfg); err != nil {
        return nil, err
    }
    if cfg.Version == 0 {
        cfg.Version = 1
    }
    return &cfg, nil
}

func Save(cfg *AppConfig) error {
    path, err := ConfigPath()
    if err != nil {
        return err
    }
    dir, err := ConfigDir()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    b, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, b, 0o600)
}
