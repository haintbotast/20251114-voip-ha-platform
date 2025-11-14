package httpapi

import (
    "net/http"

    "github.com/jackc/pgx/v5/pgxpool"
)

func HealthHandler(pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if pool != nil {
            if err := pool.Ping(r.Context()); err != nil {
                http.Error(w, "db not ok", http.StatusServiceUnavailable)
                return
            }
        }
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("OK"))
    }
}
