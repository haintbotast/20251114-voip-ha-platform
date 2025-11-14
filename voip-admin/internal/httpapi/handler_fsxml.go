package httpapi

import (
    "encoding/xml"
    "net/http"

    "github.com/jackc/pgx/v5/pgxpool"
    "voip-admin/internal/config"
    "voip-admin/internal/fsxml"
)

func DirectoryHandler(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
    svc := &fsxml.DirectoryService{Pool: pool}

    return func(w http.ResponseWriter, r *http.Request) {
        user := r.URL.Query().Get("user")
        domain := r.URL.Query().Get("domain")

        if user == "" || domain == "" {
            http.Error(w, "missing user or domain", http.StatusBadRequest)
            return
        }

        doc, err := svc.BuildDirectory(r.Context(), user, domain)
        if err != nil {
            http.Error(w, "not found", http.StatusNotFound)
            return
        }

        w.Header().Set("Content-Type", "application/xml")
        enc := xml.NewEncoder(w)
        enc.Indent("", "  ")
        _ = enc.Encode(doc)
    }
}

func DialplanHandler(cfg *config.Config, pool *pgxpool.Pool) http.HandlerFunc {
    svc := &fsxml.DialplanService{Pool: pool}

    return func(w http.ResponseWriter, r *http.Request) {
        caller := r.URL.Query().Get("caller")
        callee := r.URL.Query().Get("callee")
        contextName := r.URL.Query().Get("context")
        if contextName == "" {
            contextName = "default"
        }

        if callee == "" {
            http.Error(w, "missing callee", http.StatusBadRequest)
            return
        }

        doc, err := svc.BuildDialplan(r.Context(), caller, callee, contextName)
        if err != nil {
            http.Error(w, "no route", http.StatusNotFound)
            return
        }

        w.Header().Set("Content-Type", "application/xml")
        enc := xml.NewEncoder(w)
        enc.Indent("", "  ")
        _ = enc.Encode(doc)
    }
}
