package webdav

import (
    "os"
    "path/filepath"
    "strings"

    "frpcx/internal/config"
)

func ensureDir(dir string) error {
    return os.MkdirAll(dir, 0o755)
}

func cachedPathForProfile(name string) (string, error) {
    dir, err := config.CacheDir()
    if err != nil {
        return "", err
    }
    safe := strings.ReplaceAll(strings.ToLower(name), " ", "_")
    return filepath.Join(dir, safe+".toml"), nil
}

func writeFile(path string, data []byte) error {
    return os.WriteFile(path, data, 0o600)
}
