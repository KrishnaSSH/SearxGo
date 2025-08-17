package main

import (
    "log"
    "net/http"
    "os"
    "strconv"

    "searxgo/internal/config"
    "searxgo/internal/server"
)

func main() {
    // Load configuration from TOML file
    cfg, err := config.LoadConfig("config.toml")
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    // Load enabled engines based on configuration
    engs := cfg.LoadEnabledEngines()
    if len(engs) == 0 {
        log.Fatal("No search engines enabled in configuration")
    }
    srv := server.NewServer(engs)
    srv.Timeout = cfg.GetTimeout()
    srv.StaticDir = cfg.Server.StaticDir
    srv.DefaultSize = cfg.Server.DefaultSize
    
    handler := srv.Handler()
    port := cfg.Server.Port
    addr := ":" + strconv.Itoa(port)
    if v := os.Getenv("PORT"); v != "" {
        addr = ":" + v
    }

    log.Printf("listening on %s", addr)
    if err := http.ListenAndServe(addr, handler); err != nil {
        log.Fatal(err)
    }
}
