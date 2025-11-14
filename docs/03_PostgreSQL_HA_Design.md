# TÀI LIỆU THIẾT KẾ HA POSTGRESQL CHO CỤM VoIP
## Mô hình Primary/Standby – Debian 12 – Tích hợp Kamailio/FreeSWITCH/VoIP Admin

**Perspective:** Senior PostgreSQL DBA / HA Engineer  

Mục tiêu:

- Cung cấp thiết kế **HA 2 node PostgreSQL** (Primary/Standby) trên Debian 12.
- Hỗ trợ cho:
  - Kamailio: lưu cấu hình users, domains, trunks, routing logic (schema `voip`, `kamailio`).
  - FreeSWITCH: mapping extension/queue/IVR (qua VoIP Admin – không đọc file config trực tiếp).
  - VoIP Admin Service: CDR, recordings, metadata quản lý VoIP.
- Mô hình **active/passive**:
  - 1 Primary nhận ghi.
  - 1 Standby streaming replication.
  - Có cơ chế phát hiện lỗi & promote Standby thành Primary.
  - Tích hợp với VIP (Keepalived) hoặc connection string để ứng dụng failover tối giản.

---

## 1. TOPOLOGY & IP

Giả định:

- Node1 DB: `voip-db1` – IP: `172.16.91.111`
- Node2 DB: `voip-db2` – IP: `172.16.91.112`
- Optional DB VIP (nếu dùng Keepalived riêng cho lớp DB): `172.16.91.110`
- OS: Debian 12 (Bookworm)
- PostgreSQL: bản stable mới nhất tại thời điểm triển khai (ví dụ 16.x), nhưng tài liệu này mô tả theo cách **không khóa chặt vào minor version**.

Kết nối ứng dụng:

- Kamailio, FreeSWITCH, VoIP Admin Service đều dùng DSN:
  - Nếu dùng VIP DB: `postgres://app_user:***@172.16.91.110:5432/voipdb`
  - Nếu không dùng VIP: dùng connect string có `host=voip-db1,voip-db2` (load balancing/failover logic do application/driver xử lý, tuỳ khả năng pgx/Kamailio module).

---

## 2. CÀI ĐẶT POSTGRESQL TRÊN DEBIAN 12

### 2.1. Cài đặt repo chính thức (khuyến nghị)

Trên mỗi node:

```bash
sudo apt update
sudo apt install -y curl ca-certificates gnupg

curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg

echo "deb http://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" |   sudo tee /etc/apt/sources.list.d/pgdg.list

sudo apt update
sudo apt install -y postgresql-16 postgresql-client-16
```

> Nếu tại thời điểm triển khai đã có bản cao hơn (17, 18), thay `16` bằng phiên bản bạn chọn, toàn bộ logic HA vẫn giữ nguyên.

Dịch vụ:

```bash
systemctl enable postgresql
systemctl start postgresql
```

### 2.2. Layout dữ liệu

Mặc định Debian đặt data tại: `/var/lib/postgresql/16/main`.  
Có thể tùy chỉnh sang LVM hoặc mount riêng nếu cần IOPS cao.

---

## 3. NGƯỜI DÙNG & DATABASE

Trên Primary:

```bash
sudo -u postgres psql
```

Tạo DB & user:

```sql
CREATE DATABASE voipdb ENCODING 'UTF8' TEMPLATE template0;

CREATE ROLE app_user LOGIN PASSWORD 'StrongAppPass';
GRANT CONNECT ON DATABASE voipdb TO app_user;

\c voipdb

GRANT USAGE ON SCHEMA public TO app_user;
ALTER ROLE app_user SET search_path = public, voip, kamailio;
```

> Schema chi tiết cho `voip`, `kamailio` sẽ được định nghĩa trong tài liệu DB schema riêng, ở đây chỉ tập trung vào HA.

---

## 4. CẤU HÌNH PRIMARY

File `postgresql.conf` (Primary):

```conf
listen_addresses = '*'
port = 5432

max_connections = 500
shared_buffers = 4GB
effective_cache_size = 8GB
work_mem = 16MB
maintenance_work_mem = 512MB

wal_level = replica
archive_mode = on
archive_command = 'test ! -f /var/lib/postgresql/16/archive/%f && cp %p /var/lib/postgresql/16/archive/%f'
max_wal_senders = 10
max_replication_slots = 10
hot_standby = on

synchronous_commit = remote_apply
synchronous_standby_names = 'FIRST 1 (voip_standby_1)'
```

Tạo thư mục archive:

```bash
mkdir -p /var/lib/postgresql/16/archive
chown -R postgres:postgres /var/lib/postgresql/16/archive
chmod 700 /var/lib/postgresql/16/archive
```

File `pg_hba.conf` (Primary):

```conf
# Cho replication từ Standby
host    replication     replicator      172.16.91.112/32        md5

# Cho ứng dụng Kamailio/FS/VoIP Admin
host    voipdb          app_user        172.16.91.0/24          md5
```

Tạo user replication:

```bash
sudo -u postgres psql

CREATE ROLE replicator WITH REPLICATION LOGIN ENCRYPTED PASSWORD 'ReplStrongPass';
```

Reload:

```bash
sudo systemctl reload postgresql
```

---

## 5. CHUẨN BỊ STANDBY

### 5.1. Dừng PostgreSQL trên Standby

```bash
sudo systemctl stop postgresql
sudo rm -rf /var/lib/postgresql/16/main/*
```

### 5.2. Base backup từ Primary

Trên Standby:

```bash
sudo -u postgres pg_basebackup   -h 172.16.91.111 -p 5432   -D /var/lib/postgresql/16/main   -U replicator   -Fp -Xs -P
```

Khi được hỏi password, nhập `ReplStrongPass`.

### 5.3. Cấu hình `standby.signal` & `postgresql.auto.conf`

Từ PostgreSQL 12+, replication dùng `standby.signal`.  
Trong thư mục data đã được tạo sẵn file `standby.signal` khi chạy `pg_basebackup -R`, nếu chưa thì tạo:

```bash
sudo -u postgres touch /var/lib/postgresql/16/main/standby.signal
```

Kiểm tra `postgresql.auto.conf` trên Standby đã có dòng tương tự:

```conf
primary_conninfo = 'host=172.16.91.111 port=5432 user=replicator password=ReplStrongPass application_name=voip_standby_1'
primary_slot_name = 'voip_slot_1'
```

Trên Primary, tạo replication slot:

```bash
sudo -u postgres psql -c "SELECT * FROM pg_create_physical_replication_slot('voip_slot_1');"
```

### 5.4. Cấu hình tham số tương tự Primary

Chỉnh `postgresql.conf` trên Standby:

- Các tham số resources (shared_buffers, work_mem…) giống Primary.
- `hot_standby = on`.

Start Standby:

```bash
sudo systemctl start postgresql
sudo systemctl status postgresql
```

Kiểm tra replication:

```bash
sudo -u postgres psql -c "SELECT pid, state, sync_state, application_name FROM pg_stat_replication;"
```

Trên Primary phải thấy dòng cho `voip_standby_1` với `sync_state = sync` (nếu dùng sync).

---

## 6. CHIẾN LƯỢC FAILOVER & PROMOTE

### 6.1. Nguyên tắc

- Chỉ có **1 Primary** đang ghi tại mọi thời điểm.
- Standby chỉ đọc, nhưng có thể `hot_standby` dùng cho báo cáo/BI.
- Khi Primary hỏng: **Promote Standby** → trở thành Primary mới.
- Sau khi Primary cũ trở lại, cần rebuild lại như Standby (không auto rejoin một cách mù quáng).

### 6.2. Promote Standby thủ công

Trên Standby:

```bash
sudo -u postgres pg_ctlcluster 16 main promote
```

Hoặc:

```bash
sudo -u postgres psql -c "SELECT pg_promote();"
```

Kiểm tra:

```bash
sudo -u postgres psql -c "SELECT pg_is_in_recovery();"
```

Nếu trả về `f` (false) → đã là Primary.

### 6.3. Tích hợp Keepalived cho DB VIP (tùy chọn)

Có 2 approach:

1. **Không dùng VIP DB**: ứng dụng dùng DSN có nhiều host (và tự xử lý failover).  
2. **Dùng VIP DB** (phù hợp với mô hình HA layer đã dùng cho Kamailio/FS):

   - Keepalived chạy trên cả 2 node DB:
     - Node Primary: state MASTER, priority cao hơn.
     - Node Standby: state BACKUP.
   - Script health-check DB:

```bash
#!/bin/bash
# /usr/local/sbin/check_pg_primary.sh

ROLE=$(sudo -u postgres psql -tAc "SELECT pg_is_in_recovery();" 2>/dev/null)

if [ "$ROLE" = "f" ]; then
  exit 0  # primary
fi

exit 1  # standby hoặc lỗi
```

   - `keepalived.conf` (ví dụ trên Primary):

```conf
vrrp_script chk_pg {
    script "/usr/local/sbin/check_pg_primary.sh"
    interval 5
    timeout 3
    fall 2
    rise 2
}

vrrp_instance VI_PG {
    state MASTER
    interface ens192
    virtual_router_id 52
    priority 150
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass PgVIPSecret
    }
    virtual_ipaddress {
        172.16.91.110/24
    }
    track_script {
        chk_pg
    }
}
```

Node Standby:

```conf
vrrp_instance VI_PG {
    state BACKUP
    interface ens192
    virtual_router_id 52
    priority 100
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass PgVIPSecret
    }
    virtual_ipaddress {
        172.16.91.110/24
    }
    track_script {
        chk_pg
    }
}
```

Như vậy, VIP DB **luôn nằm trên node đang ở trạng thái Primary thật sự** (theo `pg_is_in_recovery()`).

Ứng dụng sẽ dùng DSN:

```text
postgres://app_user:StrongAppPass@172.16.91.110:5432/voipdb?sslmode=disable
```

---

## 7. SAO LƯU & KHÔI PHỤC

### 7.1. Sao lưu base (logical hoặc physical)

- Dùng `pg_dump` / `pg_dumpall` cho logical backup (schema, data).
- Dùng `pg_basebackup` định kỳ hoặc snapshot LVM/ZFS cho physical backup (nhanh & nhất quán).

Ví dụ logical backup:

```bash
pg_dump -h 172.16.91.110 -U app_user -Fc -f /backup/voipdb_$(date +%F).dump voipdb
```

### 7.2. WAL archive

Đã cấu hình `archive_mode = on` và `archive_command`:

- Đảm bảo monitor dung lượng thư mục archive.
- Điều này cho phép restore theo thời gian (PITR) trong trường hợp cần khôi phục dữ liệu CDR/recording metadata tới một thời điểm cụ thể.

---

## 8. MONITORING & ALERTING

Các chỉ số nên giám sát:

- Trên Primary:
  - `pg_stat_replication`: state, sync_state, write/flush/replay lag.
- Trên Standby:
  - `pg_last_wal_receive_lsn()`, `pg_last_wal_replay_lsn()` và độ trễ so với Primary.
- Dung lượng WAL archive.
- Tuổi backup gần nhất (RPO).
- Kết nối active, lỗi auth, lỗi deadlocks,…

Tất cả có thể đưa về hệ thống giám sát chung (Zabbix/Prometheus/Wazuh…) và thiết lập alert:

- Mất replication.
- Replication lag quá ngưỡng.
- Thay đổi vai trò bất thường (promote không theo quy trình).

---

## 9. RUNBOOK DBA

### 9.1. Quy trình failover có kiểm soát (Primary có thể shutdown bình thường)

1. Tạm dừng ghi từ ứng dụng (nếu có thể – chế độ bảo trì).
2. Đảm bảo Standby sync: replication lag ~ 0.
3. Promote Standby:

```bash
sudo -u postgres psql -c "SELECT pg_promote();"
```

4. Chờ Keepalived chuyển VIP DB sang node Standby (nếu dùng VIP).
5. Update mọi doc/monitor: Standby cũ trở thành Primary mới.
6. Rebuild node cũ thành Standby (pg_basebackup từ Primary mới).

### 9.2. Primary chết đột ngột

1. Xác nhận thật sự mất (hardware/network) – tránh split-brain.
2. Promote Standby.
3. VIP DB (nếu dùng) tự chuyển qua Standby.
4. Ứng dụng reconnect (theo DSN VIP).
5. Khi Primary cũ quay lại:
   - Stop PostgreSQL.
   - Xoá data cũ.
   - `pg_basebackup` lại từ Primary mới.
   - Khởi động như Standby.

---

## 10. LIÊN KẾT VỚI STACK KAMAILIO / FREESWITCH / VOIP ADMIN

- Tất cả config logic (users, extensions, queues, IVR, trunks, routing, CDR, recordings…) **đều được lưu trên PostgreSQL**.
- Kamailio & VoIP Admin chỉ cần dùng DSN trỏ tới VIP DB (hoặc danh sách host) để:
  - Đảm bảo **không phải sửa config** khi DB failover.
  - Tránh trạng thái lệch dữ liệu giữa các node.
- FreeSWITCH:
  - Không đọc file cấu hình tĩnh (trừ phần core bắt buộc).
  - Lấy thông tin directory/dialplan qua **VoIP Admin Service** (mod_xml_curl).
  - Gửi CDR JSON về VoIP Admin (mod_json_cdr) → VoIP Admin ghi vào PostgreSQL.

Kết quả:

- Lớp DB đảm bảo **tính nhất quán dữ liệu** và **khả năng chịu lỗi**.
- Các service VoIP phía trên giữ được kiến trúc **stateless** hoặc “ít state” nhất có thể, dễ scale & HA.

---

## 11. TÓM TẮT

Tài liệu này xác định rõ:

- Mô hình PostgreSQL HA 2 node (Primary/Standby) trên Debian 12.
- Cấu hình cần thiết cho replication, sync, slot, standby.
- Cơ chế failover bằng promote + Keepalived health-check để gắn VIP vào Primary thực.
- Chiến lược backup/restore, monitoring, runbook DBA.
- Cách tích hợp với Kamailio/FreeSWITCH/VoIP Admin trong kiến trúc **VoIP HA 2 node** mà toàn bộ logic được điều khiển bởi cơ sở dữ liệu, không phụ thuộc file cấu hình tĩnh.

Các tài liệu tiếp theo sẽ chi tiết hóa schema (`voip`, `kamailio`) và mapping trực tiếp sang code Go của VoIP Admin Service.
