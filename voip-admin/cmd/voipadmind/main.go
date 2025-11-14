package main

import (
    "context"
    "flag"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "voip-admin/internal/config"
    "voip-admin/internal/db"
    "voip-admin/internal/httpapi"
)

func main() {
    cfgPath := flag.String("config", "/etc/voipadmind.yaml", "config file path")
    flag.Parse()

    cfg, err := config.Load(*cfgPath)
    if err != nil {
        log.Fatalf("load config: %v", err)
    }

    pool, err := db.NewPool(cfg.DBDSN)
    if err != nil {
        log.Fatalf("db connect: %v", err)
    }
    defer pool.Close()

    router := httpapi.NewRouter(cfg, pool)

    srv := &http.Server{
        Addr:         cfg.ListenAddr,
        Handler:      router,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        log.Printf("VoIP Admin Service listening on %s", cfg.ListenAddr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("ListenAndServe: %v", err)
        }
    }()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Printf("server shutdown error: %v", err)
    }
}
