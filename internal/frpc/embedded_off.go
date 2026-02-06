//go:build !with_embedded_frpc

package frpc

func embeddedBinary() (string, []byte, bool) {
    return "", nil, false
}
