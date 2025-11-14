package httpapi

import (
    "io"
    "net/http"

    "github.com/jackc/pgx/v5/pgxpool"
    "voip-admin/internal/cdr"
    "voip-admin/internal/config"
)

func CDRIngestHandler(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        body, err := io.ReadAll(r.Body)
        if err != nil {
            http.Error(w, "invalid body", http.StatusBadRequest)
            return
        }
        defer r.Body.Close()

        if err := cdr.InsertCDR(r.Context(), pool, body); err != nil {
            http.Error(w, "failed to insert cdr", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("OK"))
    }
}
