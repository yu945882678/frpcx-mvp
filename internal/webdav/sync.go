package webdav

import (
    "errors"
    "path"
    "strings"

    "github.com/studio-b12/gowebdav"

    "frpcx/internal/config"
)

func SyncProfiles(cfg *config.AppConfig) (map[string]string, error) {
    if cfg.WebDAV.URL == "" || cfg.WebDAV.Username == "" || cfg.WebDAV.Password == "" {
        return nil, errors.New("webdav config is incomplete")
    }

    client := gowebdav.NewClient(cfg.WebDAV.URL, cfg.WebDAV.Username, cfg.WebDAV.Password)

    cacheDir, err := config.CacheDir()
    if err != nil {
        return nil, err
    }

    if err := ensureDir(cacheDir); err != nil {
        return nil, err
    }

    updated := map[string]string{}
    for _, p := range cfg.Profiles {
        if p.RemoteConfigPath == "" {
            continue
        }
        remote := p.RemoteConfigPath
        if cfg.WebDAV.RemoteBase != "" && !strings.HasPrefix(remote, "/") {
            remote = path.Join(cfg.WebDAV.RemoteBase, remote)
        }
        if !strings.HasPrefix(remote, "/") {
            remote = "/" + remote
        }
        data, err := client.Read(remote)
        if err != nil {
            return updated, err
        }
        localPath, err := cachedPathForProfile(p.Name)
        if err != nil {
            return updated, err
        }
        if err := writeFile(localPath, data); err != nil {
            return updated, err
        }
        updated[p.Name] = localPath
    }
    return updated, nil
}
