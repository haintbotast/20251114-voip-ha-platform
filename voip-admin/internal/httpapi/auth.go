package httpapi

import (
    "encoding/base64"
    "net/http"
    "strings"

    "voip-admin/internal/config"
)

func XMLCurlBasicAuth(cfg *config.Config) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            auth := r.Header.Get("Authorization")
            if !strings.HasPrefix(auth, "Basic ") {
                w.Header().Set("WWW-Authenticate", `Basic realm="fsxml"`)
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            payload, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
            parts := strings.SplitN(string(payload), ":", 2)
            if len(parts) != 2 || parts[0] != cfg.XMLCurlUser || parts[1] != cfg.XMLCurlPass {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

func CDRTokenAuth(cfg *config.Config) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
            if token == "" {
                token = r.Header.Get("X-CDR-Token")
            }
            if token == "" || token != cfg.CDRAuthorization {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

func APIKeyAuth(cfg *config.Config) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := r.Header.Get("X-API-Key")
            if key == "" {
                http.Error(w, "api key required", http.StatusUnauthorized)
                return
            }
            ok := false
            for _, k := range cfg.APIKeys {
                if k.Key == key {
                    ok = true
                    break
                }
            }
            if !ok {
                http.Error(w, "invalid api key", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
