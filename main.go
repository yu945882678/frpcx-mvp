package main

import (
    "log"

    "frpcx/internal/config"
    "frpcx/internal/ui"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("加载配置失败: %v", err)
    }
    ui.Run(cfg)
}
