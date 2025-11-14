
# TÀI LIỆU THIẾT KẾ & MÃ NGUỒN
## VoIP Admin Service / API Gateway (Go)

**Perspective:** Senior Go Backend Engineer + FreeSWITCH Integration Engineer  
**Mục tiêu:** Thiết kế chi tiết và cung cấp mã nguồn mẫu đầy đủ cho dịch vụ VoIP Admin Service:

- XML backend cho FreeSWITCH (mod_xml_curl) – `directory` + `dialplan`.
- HTTP endpoint nhận CDR JSON từ FreeSWITCH (mod_json_cdr).
- REST API cho CDR & file recordings.

> Lưu ý: Đây là skeleton “production-oriented”, có thể build & mở rộng. Bạn nên đưa vào Git, thêm CI/CD, logging, observability theo chuẩn nội bộ.

---

## 1. TỔNG QUAN KIẾN TRÚC

### 1.1. Nhiệm vụ

1. **XML_CURL backend:**  
   - FreeSWITCH gọi:  
     - `/fs/xml/directory?user=1001&domain=bsv.local`  
     - `/fs/xml/dialplan?caller=1001&callee=8001&context=from-kamailio`  
   - Service trả về XML tương thích với FreeSWITCH (theo spec mod_xml_curl).

2. **CDR collector:**  
   - FreeSWITCH (mod_json_cdr) POST JSON tới `/fs/cdr`.  
   - Service nhận, parse, map sang DB schema `voip.cdr` & `voip.recordings`.

3. **API Gateway:**  
   - `/api/cdr` – query CDR theo nhiều filter (thời gian, queue, agent, trunk…).  
   - `/api/recordings/{id}` – trả metadata và/hoặc stream file ghi âm.

### 1.2. Công nghệ

- Ngôn ngữ: **Go 1.25.x**
- Router: `github.com/go-chi/chi/v5`
- DB: PostgreSQL (driver `github.com/jackc/pgx/v5/pgxpool`)
- Logging: `log/slog`
- Config: file YAML + env

---

## 2. CẤU TRÚC PROJECT

```text
voip-admin/
  cmd/voipadmind/
    main.go
  internal/
    config/
      config.go
    db/
      db.go
    fsxml/
      directory.go
      dialplan.go
      model_xml.go
    cdr/
      ingest.go
      query.go
    httpapi/
      router.go
      middleware.go
      handler_fsxml.go
      handler_cdr.go
      handler_recording.go
      handler_health.go
      auth.go
    models/
      voip.go
```

---

## 3. CONFIG

### 3.1. Cấu trúc YAML

`/etc/voipadmind.yaml`:

```yaml
listen_addr: ":8080"

db_dsn: "postgres://app_user:StrongAppPass@172.16.91.100:5432/voipdb?sslmode=disable"

xmlcurl_basic_user: "fsxml"
xmlcurl_basic_pass: "VerySecret"

cdr_auth_token: "ChangeThisForCDR"

api_keys:
  - name: "billing-system"
    key: "BillingApiKeyVerySecret"
    role: "billing"
  - name: "crm-system"
    key: "CrmApiKeyVerySecret"
    role: "crm"

recordings:
  base_path: "/srv/recordings"
```

### 3.2. Struct Config (Go)

`internal/config/config.go`:

```go
package config

import (
    "log"
    "os"

    "gopkg.in/yaml.v3"
)

type APIKey struct {
    Name string `yaml:"name"`
    Key  string `yaml:"key"`
    Role string `yaml:"role"`
}

type RecordingConfig struct {
    BasePath string `yaml:"base_path"`
}

type Config struct {
    ListenAddr       string          `yaml:"listen_addr"`
    DBDSN            string          `yaml:"db_dsn"`
    XMLCurlUser      string          `yaml:"xmlcurl_basic_user"`
    XMLCurlPass      string          `yaml:"xmlcurl_basic_pass"`
    CDRAuthorization string          `yaml:"cdr_auth_token"`
    APIKeys          []APIKey        `yaml:"api_keys"`
    Recordings       RecordingConfig `yaml:"recordings"`
}

func Load(path string) (*Config, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    dec := yaml.NewDecoder(f)
    var cfg Config
    if err := dec.Decode(&cfg); err != nil {
        return nil, err
    }

    if cfg.ListenAddr == "" {
        cfg.ListenAddr = ":8080"
    }

    if cfg.DBDSN == "" {
        log.Println("WARNING: DB DSN empty")
    }

    return &cfg, nil
}
```

---

## 4. KẾT NỐI DB

`internal/db/db.go`:

```go
package db

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(dsn string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        return nil, err
    }
    cfg.MaxConns = 20
    cfg.MinConns = 5
    cfg.MaxConnLifetime = 30 * time.Minute

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, err
    }

    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, err
    }

    return pool, nil
}
```

---

## 5. MAIN & ROUTER

### 5.1. `cmd/voipadmind/main.go`

```go
package main

import (
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
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        log.Printf("VoIP Admin Service listening on %s", cfg.ListenAddr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("ListenAndServe: %v", err)
        }
    }()

    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    ctx, cancel := time.WithTimeout(time.Background(), 10*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Printf("server shutdown error: %v", err)
    }
}
```

### 5.2. `internal/httpapi/router.go`

```go
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

    // XML_CURL endpoints (directory/dialplan)
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
```

---

## 6. MIDDLEWARE & AUTH

`internal/httpapi/middleware.go`:

```go
package httpapi

import (
    "log"
    "net/http"
    "time"
)

func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
    })
}

func RecoverMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Printf("panic: %v", rec)
                http.Error(w, "internal server error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

`internal/httpapi/auth.go`:

```go
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
                // cũng cho phép dùng header đơn giản
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
```

---

## 7. MODELS (MAPPING DB)

`internal/models/voip.go` (rút gọn các struct chính):

```go
package models

import "time"

type Domain struct {
    ID        int64     `db:"id"`
    Name      string    `db:"name"`
    IsActive  bool      `db:"is_active"`
    CreatedAt time.Time `db:"created_at"`
    UpdatedAt time.Time `db:"updated_at"`
}

type ExtensionType string

const (
    ExtensionTypeUser      ExtensionType = "user"
    ExtensionTypeQueue     ExtensionType = "queue"
    ExtensionTypeIVR       ExtensionType = "ivr"
    ExtensionTypeVoicemail ExtensionType = "voicemail"
    ExtensionTypeService   ExtensionType = "service"
    ExtensionTypeTrunkOut  ExtensionType = "trunk_out"
)

type Extension struct {
    ID        int64         `db:"id"`
    DomainID  int64         `db:"domain_id"`
    Exten     string        `db:"exten"`
    Type      ExtensionType `db:"type"`
    ServiceRef []byte       `db:"service_ref"`
    NeedMedia bool          `db:"need_media"`
    IsActive  bool          `db:"is_active"`
}

type CDR struct {
    ID               int64      `db:"id"`
    CallUUID         string     `db:"call_uuid"`
    Direction        string     `db:"direction"`
    CallerIDNumber   *string    `db:"caller_id_number"`
    DestinationNumber *string   `db:"destination_number"`
    StartTime        time.Time  `db:"start_time"`
    AnswerTime       *time.Time `db:"answer_time"`
    EndTime          time.Time  `db:"end_time"`
    Duration         int        `db:"duration"`
    BillSec          int        `db:"billsec"`
    HangupCause      *string    `db:"hangup_cause"`
    QueueID          *int64     `db:"queue_id"`
    AgentUserID      *int64     `db:"agent_user_id"`
    TrunkID          *int64     `db:"trunk_id"`
    RecordingID      *int64     `db:"recording_id"`
    CreatedAt        time.Time  `db:"created_at"`
}

type Recording struct {
    ID        int64     `db:"id"`
    CallUUID  string    `db:"call_uuid"`
    Path      string    `db:"path"`
    Backend   string    `db:"backend"`
    SizeBytes *int64    `db:"size_bytes"`
    CreatedAt time.Time `db:"created_at"`
}
```

---

## 8. XML CẤP CHO FREESWITCH

### 8.1. Model XML

`internal/fsxml/model_xml.go`:

```go
package fsxml

import "encoding/xml"

type Document struct {
    XMLName xml.Name `xml:"document"`
    Type    string   `xml:"type,attr"`
    Section []Section `xml:"section"`
}

type Section struct {
    Name        string       `xml:"name,attr"`
    Description string       `xml:"description,attr,omitempty"`
    Domain      *DomainNode  `xml:"domain,omitempty"`
    Context     *ContextNode `xml:"context,omitempty"`
}

type DomainNode struct {
    Name string     `xml:"name,attr"`
    User []UserNode `xml:"user"`
}

type UserNode struct {
    ID      string         `xml:"id,attr"`
    Params  []ParamNode    `xml:"params>param,omitempty"`
    Vars    []VariableNode `xml:"variables>variable,omitempty"`
}

type ParamNode struct {
    Name  string `xml:"name,attr"`
    Value string `xml:"value,attr"`
}

type VariableNode struct {
    Name  string `xml:"name,attr"`
    Value string `xml:"value,attr"`
}

type ContextNode struct {
    Name      string           `xml:"name,attr"`
    Extension []ExtensionNode  `xml:"extension"`
}

type ExtensionNode struct {
    Name      string            `xml:"name,attr"`
    Condition []ConditionNode   `xml:"condition"`
}

type ConditionNode struct {
    Field     string         `xml:"field,attr,omitempty"`
    Expr      string         `xml:"expression,attr,omitempty"`
    Action    []ActionNode   `xml:"action"`
}

type ActionNode struct {
    App  string `xml:"application,attr"`
    Data string `xml:"data,attr"`
}
```

### 8.2. Directory builder (ví dụ tối giản)

`internal/fsxml/directory.go`:

```go
package fsxml

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

type DirectoryService struct {
    Pool *pgxpool.Pool
}

func (d *DirectoryService) BuildDirectory(ctx context.Context, user, domain string) (*Document, error) {
    // ví dụ: query password & info từ DB (tối giản)
    var password, fullName string
    err := d.Pool.QueryRow(ctx, `
        SELECT u.username, '1234' as password
        FROM voip.users u
        JOIN voip.domains d ON d.id = u.domain_id
        WHERE u.username=$1 AND d.name=$2 AND u.is_active=TRUE
    `, user, domain).Scan(&fullName, &password)
    if err != nil {
        return nil, err
    }

    doc := &Document{
        Type: "freeswitch/xml",
        Section: []Section{
            {
                Name: "directory",
                Domain: &DomainNode{
                    Name: domain,
                    User: []UserNode{
                        {
                            ID: user,
                            Params: []ParamNode{
                                {Name: "password", Value: password},
                            },
                            Vars: []VariableNode{
                                {Name: "effective_caller_id_name", Value: fullName},
                                {Name: "effective_caller_id_number", Value: user},
                            },
                        },
                    },
                },
            },
        },
    }

    return doc, nil
}
```

### 8.3. Dialplan builder (ví dụ inbound queue)

`internal/fsxml/dialplan.go` (khung đơn giản):

```go
package fsxml

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

type DialplanService struct {
    Pool *pgxpool.Pool
}

func (s *DialplanService) BuildDialplan(ctx context.Context, caller, callee, contextName string) (*Document, error) {
    // Ví dụ: tìm extension type
    var extType, serviceRef string
    err := s.Pool.QueryRow(ctx, `
        SELECT type::text, COALESCE(service_ref::text, '')
        FROM voip.extensions e
        JOIN voip.domains d ON d.id=e.domain_id
        WHERE e.exten=$1 AND e.is_active=TRUE
        LIMIT 1
    `, callee).Scan(&extType, &serviceRef)
    if err != nil {
        return nil, err
    }

    // Ví dụ: nếu type=queue thì gọi mod_callcenter
    var extensionNode ExtensionNode
    switch extType {
    case "queue":
        extensionNode = ExtensionNode{
            Name: fmt.Sprintf("queue_%s", callee),
            Condition: []ConditionNode{
                {
                    Field: "destination_number",
                    Expr:  fmt.Sprintf("^%s$", callee),
                    Action: []ActionNode{
                        {App: "answer", Data: ""},
                        {App: "set", Data: "queue_name=Support_L1@default"},
                        {App: "callcenter", Data: "${queue_name}"},
                    },
                },
            },
        }
    default:
        extensionNode = ExtensionNode{
            Name: fmt.Sprintf("user_%s", callee),
            Condition: []ConditionNode{
                {
                    Field: "destination_number",
                    Expr:  fmt.Sprintf("^%s$", callee),
                    Action: []ActionNode{
                        {App: "bridge", Data: fmt.Sprintf("user/%s", callee)},
                    },
                },
            },
        }
    }

    doc := &Document{
        Type: "freeswitch/xml",
        Section: []Section{
            {
                Name:    "dialplan",
                Context: &ContextNode{Name: contextName, Extension: []ExtensionNode{extensionNode}},
            },
        },
    }

    return doc, nil
}
```

---

## 9. HANDLERS HTTP

### 9.1. Health & Version

`internal/httpapi/handler_health.go`:

```go
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

func VersionHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"version":"1.0.0","name":"voip-admin-service"}`))
    }
}
```

### 9.2. XML handlers

`internal/httpapi/handler_fsxml.go`:

```go
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
```

### 9.3. CDR ingest

`internal/cdr/ingest.go` (logic):

```go
package cdr

import (
    "context"
    "encoding/json"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type FreeSwitchCDR struct {
    Variables struct {
        UUID            string `json:"uuid"`
        Direction       string `json:"direction"`
        CallerIDNumber  string `json:"caller_id_number"`
        DestinationNumber string `json:"destination_number"`
        StartStamp      string `json:"start_stamp"`
        AnswerStamp     string `json:"answer_stamp"`
        EndStamp        string `json:"end_stamp"`
        Duration        string `json:"duration"`
        BillSec         string `json:"billsec"`
        HangupCause     string `json:"hangup_cause"`
        QueueName       string `json:"queue_name"`
        AgentID         string `json:"agent_id"`
        RecordingFile   string `json:"recording_file"`
    } `json:"variables"`
}

func InsertCDR(ctx context.Context, pool *pgxpool.Pool, raw []byte) error {
    var fs FreeSwitchCDR
    if err := json.Unmarshal(raw, &fs); err != nil {
        return err
    }

    // parse times & duration
    layout := "2006-01-02 15:04:05"
    start, _ := time.ParseInLocation(layout, fs.Variables.StartStamp, time.Local)
    end, _ := time.ParseInLocation(layout, fs.Variables.EndStamp, time.Local)
    answer, _ := time.ParseInLocation(layout, fs.Variables.AnswerStamp, time.Local)

    // parse int
    // (trong triển khai thực tế cần check lỗi từng bước)
    dur := atoiSafe(fs.Variables.Duration)
    bill := atoiSafe(fs.Variables.BillSec)

    // Insert recording trước (nếu có)
    var recordingID *int64
    if fs.Variables.RecordingFile != "" {
        var id int64
        err := pool.QueryRow(ctx, `
            INSERT INTO voip.recordings (call_uuid, path, backend)
            VALUES ($1, $2, 'local')
            ON CONFLICT (call_uuid, path) DO UPDATE SET path=EXCLUDED.path
            RETURNING id
        `, fs.Variables.UUID, fs.Variables.RecordingFile).Scan(&id)
        if err == nil {
            recordingID = &id
        }
    }

    _, err := pool.Exec(ctx, `
        INSERT INTO voip.cdr (
            call_uuid, direction,
            caller_id_number, destination_number,
            start_time, answer_time, end_time,
            duration, billsec, hangup_cause, recording_id, raw_json
        ) VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
        )
        ON CONFLICT (call_uuid) DO NOTHING
    `,
        fs.Variables.UUID,
        fs.Variables.Direction,
        fs.Variables.CallerIDNumber,
        fs.Variables.DestinationNumber,
        start,
        answer,
        end,
        dur,
        bill,
        fs.Variables.HangupCause,
        recordingID,
        raw,
    )
    return err
}

func atoiSafe(s string) int {
    var n int
    _, _ = fmt.Sscanf(s, "%d", &n)
    return n
}
```

`internal/httpapi/handler_cdr.go`:

```go
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
```

### 9.4. Query CDR & Recordings

`internal/httpapi/handler_cdr_query.go` (ví dụ đơn giản):

```go
package httpapi

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "voip-admin/internal/models"
)

type CDRResponse struct {
    Items []models.CDR `json:"items"`
}

func CDRQueryHandler(pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        q := r.URL.Query()

        fromStr := q.Get("from")
        toStr := q.Get("to")
        caller := q.Get("caller")
        callee := q.Get("callee")

        limitStr := q.Get("limit")
        if limitStr == "" {
            limitStr = "100"
        }
        limit, _ := strconv.Atoi(limitStr)
        if limit > 1000 {
            limit = 1000
        }

        var (
            where  []string
            args   []interface{}
            idx    = 1
        )

        layout := time.RFC3339
        if fromStr != "" {
            t, _ := time.Parse(layout, fromStr)
            where = append(where, "start_time >= $"+strconv.Itoa(idx))
            args = append(args, t)
            idx++
        }
        if toStr != "" {
            t, _ := time.Parse(layout, toStr)
            where = append(where, "start_time <= $"+strconv.Itoa(idx))
            args = append(args, t)
            idx++
        }
        if caller != "" {
            where = append(where, "caller_id_number = $"+strconv.Itoa(idx))
            args = append(args, caller)
            idx++
        }
        if callee != "" {
            where = append(where, "destination_number = $"+strconv.Itoa(idx))
            args = append(args, callee)
            idx++
        }

        query := "SELECT id, call_uuid, direction, caller_id_number, destination_number, start_time, answer_time, end_time, duration, billsec, hangup_cause, queue_id, agent_user_id, trunk_id, recording_id, created_at FROM voip.cdr"
        if len(where) > 0 {
            query += " WHERE " + strings.Join(where, " AND ")
        }
        query += " ORDER BY start_time DESC LIMIT " + strconv.Itoa(limit)

        rows, err := pool.Query(r.Context(), query, args...)
        if err != nil {
            http.Error(w, "query error", http.StatusInternalServerError)
            return
        }
        defer rows.Close()

        res := CDRResponse{}
        for rows.Next() {
            var c models.CDR
            if err := rows.Scan(
                &c.ID, &c.CallUUID, &c.Direction,
                &c.CallerIDNumber, &c.DestinationNumber,
                &c.StartTime, &c.AnswerTime, &c.EndTime,
                &c.Duration, &c.BillSec, &c.HangupCause,
                &c.QueueID, &c.AgentUserID, &c.TrunkID, &c.RecordingID,
                &c.CreatedAt,
            ); err != nil {
                http.Error(w, "scan error", http.StatusInternalServerError)
                return
            }
            res.Items = append(res.Items, c)
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(res)
    }
}
```

`internal/httpapi/handler_recording.go`:

```go
package httpapi

import (
    "net/http"
    "os"
    "path/filepath"
    "strconv"

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
```

---

## 10. TRIỂN KHAI & VẬN HÀNH

### 10.1. Build & cài đặt

```bash
cd /opt/voip-admin
go build ./cmd/voipadmind

cp voipadmind /usr/local/bin/
cp configs/voipadmind.yaml /etc/voipadmind.yaml
useradd --system --home /var/lib/voipadmind --shell /usr/sbin/nologin voipadmin
mkdir -p /srv/recordings
chown -R voipadmin:voipadmin /srv/recordings
```

Systemd unit như đã mô tả trong tài liệu OS/HA (Doc 01).

### 10.2. Test tích hợp với FreeSWITCH

1. Freeswitch bật `mod_xml_curl` & `mod_json_cdr`.
2. Kiểm tra `/health`:

```bash
curl http://127.0.0.1:8080/health
```

3. Test directory:

```bash
curl -u fsxml:VerySecret "http://127.0.0.1:8080/fs/xml/directory?user=1001&domain=bsv.local"
```

4. Test dialplan (giả lập):

```bash
curl -u fsxml:VerySecret "http://127.0.0.1:8080/fs/xml/dialplan?caller=1001&callee=8001&context=from-kamailio"
```

5. Thực hiện cuộc gọi thật, check CDR:

```bash
curl -H "X-API-Key: BillingApiKeyVerySecret" \
  "http://127.0.0.1:8080/api/cdr?limit=10"
```

6. Lấy recording:

```bash
curl -H "X-API-Key: BillingApiKeyVerySecret" \
  -o test.wav "http://127.0.0.1:8080/api/recordings/1"
```

---

## 11. TÓM TẮT

Tài liệu này cung cấp:

- Thiết kế chi tiết của **VoIP Admin Service** từ góc độ backend Go.
- Cấu trúc project chuẩn, mapping DB, XML builder cho FreeSWITCH, CDR ingest & APIs.
- Mã nguồn skeleton đủ hoàn chỉnh để:
  - Build binary.
  - Kết nối FreeSWITCH/DB.
  - Từng bước mở rộng cho logic phức tạp hơn (multi-tenant, multi-queue, SLA, BI…).