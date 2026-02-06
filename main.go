package main

import (
    "log"

    "frpcx/internal/config"
    "frpcx/internal/ui"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("load config: %v", err)
    }
    ui.Run(cfg)
}
