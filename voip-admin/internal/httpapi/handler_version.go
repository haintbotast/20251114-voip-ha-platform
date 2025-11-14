package httpapi

import (
    "net/http"
)

func VersionHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"name":"voip-admin-service","version":"1.0.0"}`))
    }
}
