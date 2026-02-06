package frpc

import (
    "crypto/sha256"
    "fmt"
    "os"
    "path/filepath"
    "runtime"

    "frpcx/internal/config"
)

func ResolveBinaryPath(userPath string) (string, error) {
    if userPath != "" {
        return userPath, nil
    }

    name, data, ok := embeddedBinary()
    if !ok {
        return defaultBinaryName(), nil
    }

    dir, err := config.CacheDir()
    if err != nil {
        return "", err
    }
    binDir := filepath.Join(dir, "bin")
    if err := os.MkdirAll(binDir, 0o755); err != nil {
        return "", err
    }

    dst := filepath.Join(binDir, name)
    if err := writeIfChanged(dst, data); err != nil {
        return "", err
    }

    if runtime.GOOS != "windows" {
        _ = os.Chmod(dst, 0o755)
    }

    return dst, nil
}

func defaultBinaryName() string {
    if runtime.GOOS == "windows" {
        return "frpc.exe"
    }
    return "frpc"
}

func writeIfChanged(path string, data []byte) error {
    existing, err := os.ReadFile(path)
    if err == nil {
        if sha256.Sum256(existing) == sha256.Sum256(data) {
            return nil
        }
    }
    if err := os.WriteFile(path, data, 0o700); err != nil {
        return fmt.Errorf("write embedded frpc: %w", err)
    }
    return nil
}
