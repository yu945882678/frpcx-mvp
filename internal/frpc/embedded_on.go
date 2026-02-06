//go:build with_embedded_frpc

package frpc

import (
    "embed"
    "path/filepath"
    "runtime"
)

//go:embed assets/frpc/*/*
var embeddedFS embed.FS

func embeddedBinary() (string, []byte, bool) {
    dir := filepath.ToSlash(filepath.Join("assets", "frpc", runtime.GOOS+"_"+runtime.GOARCH))
    name := "frpc"
    if runtime.GOOS == "windows" {
        name = "frpc.exe"
    }
    path := filepath.ToSlash(filepath.Join(dir, name))
    data, err := embeddedFS.ReadFile(path)
    if err != nil {
        return "", nil, false
    }
    return name, data, true
}
