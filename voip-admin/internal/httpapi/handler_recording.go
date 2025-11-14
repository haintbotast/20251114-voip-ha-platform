package httpapi

import (
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "voip-admin/internal/config"
)

func RecordingHandler(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        idStr := chi.URLParam(r, "id")
        id, err := strconv.ParseInt(idStr, 10, 64)
        if err != nil {
            http.Error(w, "invalid id", http.StatusBadRequest)
            return
        }

        var path string
        err = pool.QueryRow(r.Context(), `
            SELECT path FROM voip.recordings WHERE id=$1
        `, id).Scan(&path)
        if err != nil {
            http.Error(w, "not found", http.StatusNotFound)
            return
        }

        fullPath := filepath.Join(cfg.Recordings.BasePath, path)
        f, err := os.Open(fullPath)
        if err != nil {
            http.Error(w, "file not found", http.StatusNotFound)
            return
        }
        defer f.Close()

        http.ServeContent(w, r, filepath.Base(fullPath), time.Time{}, f)
    }
}
