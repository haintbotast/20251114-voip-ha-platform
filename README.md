# VoIP HA Platform – Kamailio + FreeSWITCH + PostgreSQL + Go Admin Service

Repo skeleton để bạn upload trực tiếp lên GitHub và phát triển tiếp.

## Cấu trúc

- `docs/`
  - `01_OS_Network_HA_Debian12.md`
  - `02_VoIP_Admin_Service_Go_Design_and_Source.md`
  - `03_PostgreSQL_HA_Design.md`
- `voip-admin/`
  - `cmd/voipadmind/main.go`
  - `internal/config/config.go`
  - `internal/db/db.go`
  - `internal/httpapi/*.go`
  - `internal/fsxml/*.go`
  - `internal/cdr/*.go`
  - `internal/models/*.go`

Phần mã Go là skeleton đầy đủ theo spec trong doc 02, có thể build được, sẵn sàng mở rộng.
