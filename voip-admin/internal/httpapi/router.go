package httpapi

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "voip-admin/internal/config"
)

func NewRouter(cfg *config.Config, pool *pgxpool.Pool) http.Handler {
    r := chi.NewRouter()

    r.Use(LoggingMiddleware)
    r.Use(RecoverMiddleware)

    r.Get("/health", HealthHandler(pool))
    r.Get("/version", VersionHandler())

    // XML_CURL endpoints
    r.Route("/fs/xml", func(fs chi.Router) {
        fs.With(XMLCurlBasicAuth(cfg)).Get("/directory", DirectoryHandler(cfg, pool))
        fs.With(XMLCurlBasicAuth(cfg)).Get("/dialplan", DialplanHandler(cfg, pool))
    })

    // CDR ingest
    r.With(CDRTokenAuth(cfg)).Post("/fs/cdr", CDRIngestHandler(cfg, pool))

    // External APIs
    r.Route("/api", func(api chi.Router) {
        api.With(APIKeyAuth(cfg)).Get("/cdr", CDRQueryHandler(pool))
        api.With(APIKeyAuth(cfg)).Get("/recordings/{id}", RecordingHandler(cfg, pool))
    })

    return r
}
