# frpcx (MVP)

A small cross-platform desktop client for frpc with auto-switching profiles and WebDAV sync.

## Features
- Single-window compact UI + system tray menu
- Multiple profiles (priority by list order)
- Auto-switch on startup failure or runtime exit
- WebDAV sync to pull config files (e.g., Jianguoyun)

## Notes
- This MVP expects an external `frpc` binary. Set `frpc Path` per profile or ensure `frpc` is in PATH.
- Configs are stored in the user's config directory under `frpcx/config.json`.
- WebDAV sync downloads remote configs into `frpcx/cache/` and maps them to profiles.

## Build
```bash
# requires Go 1.22+ and Fyne
go mod tidy
GOOS=darwin GOARCH=amd64 go build -o frpcx
```

## Single-File Build (embed frpc)
Place the platform binary under:
- `internal/frpc/assets/frpc/<goos>_<goarch>/frpc` (or `frpc.exe` on Windows)

Then build with:
```bash
go build -tags with_embedded_frpc -o frpcx
```

## Run
```bash
./frpcx
```

## Auto-switch logic
- Pre-checks server connectivity (if server addr/port set)
- Pre-checks local ports (if provided)
- Starts frpc and parses logs for failure patterns
- On exit/failure, attempts the next enabled profile

## Status Health Check
- Optional per-profile status check uses `frpc status -c <config>` to verify admin API availability.
- This requires `webServer` enabled in your frpc config.
- When enabled, startup waits for status success, and runtime monitor switches after repeated failures.
