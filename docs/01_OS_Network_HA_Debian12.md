
# TÀI LIỆU TRIỂN KHAI HẠ TẦNG OS / NETWORK / HA
## Cụm VoIP 2 Node Debian 12 + Keepalived VIP 172.16.91.100

**Perspective:** Senior Linux/Network/HA Engineer  
**Mục tiêu:** Chuẩn hóa hệ điều hành, network, HA layer (VIP, failover) cho cụm Kamailio + FreeSWITCH + PostgreSQL + VoIP Admin Service chạy trên **2 node Debian 12**.

---

## 1. MÔ HÌNH HẠ TẦNG

### 1.1. Node & IP

- Node1
  - Hostname: `voip-node1`
  - IP: `172.16.91.101/24`
- Node2
  - Hostname: `voip-node2`
  - IP: `172.16.91.102/24`
- VIP (VRRP/Keepalived)
  - `172.16.91.100/24` (gắn trên interface, ví dụ: `ens192`)

Các thành phần chạy trên cả 2 node:

- Debian 12 (Bookworm)
- PostgreSQL 18 (Primary/Standby)
- Kamailio 6
- FreeSWITCH 1.10.x
- VoIP Admin Service (Go)
- Keepalived

Luồng traffic chính:

- SIP/SRTP từ thiết bị/Softphone → VIP 172.16.91.100 → Kamailio node active.
- Kamailio → FreeSWITCH (cùng node) cho các cuộc gọi cần media/IVR/Queue/Recording.
- FreeSWITCH → VoIP Admin Service (HTTP) → PostgreSQL.

---

## 2. CHUẨN HÓA HỆ ĐIỀU HÀNH DEBIAN 12

### 2.1. Cài đặt cơ bản

1. Cài Debian 12 minimal, chỉ chọn SSH server và các công cụ cơ bản.
2. Cấu hình timezone:

```bash
timedatectl set-timezone Asia/Ho_Chi_Minh
timedatectl status
```

3. Cấu hình NTP (dùng `systemd-timesyncd` hoặc `chrony`):

```bash
apt update
apt install -y chrony
systemctl enable --now chronyd
chronyc sources
```

### 2.2. Cấu hình IP tĩnh

Ví dụ `/etc/network/interfaces.d/ens192` (Node1):

```text
auto ens192
iface ens192 inet static
    address 172.16.91.101/24
    gateway 172.16.91.1
    dns-nameservers 8.8.8.8 1.1.1.1
```

Node2 thay `address` thành `172.16.91.102/24`.

Sau thay đổi:

```bash
systemctl restart networking
ip addr show ens192
```

---

## 3. TỐI ƯU KERNEL / SYSCTL CHO VoIP

### 3.1. Sysctl cho toàn hệ thống

Tạo file `/etc/sysctl.d/99-voip.conf` trên **cả 2 node**:

```conf
fs.file-max = 1048576

net.core.rmem_max = 26214400
net.core.wmem_max = 26214400
net.core.rmem_default = 26214400
net.core.wmem_default = 26214400
net.core.netdev_max_backlog = 5000

net.ipv4.udp_rmem_min = 16384
net.ipv4.udp_wmem_min = 16384

net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_tw_reuse = 1

net.ipv4.ip_local_port_range = 10240 65000

net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.default.accept_source_route = 0

net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0

net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
```

Apply:

```bash
sysctl --system
```

### 3.2. Giới hạn file descriptors / process

File `/etc/security/limits.d/99-voip.conf`:

```conf
* soft nofile  102400
* hard nofile  102400
* soft nproc   65535
* hard nproc   65535

freeswitch soft nofile 200000
freeswitch hard nofile 200000

kamailio soft nofile 200000
kamailio hard nofile 200000
```

Đảm bảo `pam_limits.so` được load trong `common-session` và `common-session-noninteractive`.

---

## 4. FIREWALL (NFTABLES)

### 4.1. Danh sách port cần mở

- SSH quản trị: 22/tcp (giới hạn dải IP quản trị)
- SIP: 5060/udp, 5060/tcp (và 5061 nếu dùng TLS)
- RTP FreeSWITCH: 10000–20000/udp
- PostgreSQL: 5432/tcp (chỉ 2 node & VoIP Admin nếu khác subnet)
- VoIP Admin HTTP: 8080/tcp (chỉ nội bộ)
- VRRP (Keepalived): protocol 112/ipv4

### 4.2. Cấu hình mẫu `/etc/nftables.conf`

```nft
table inet filter {
  chain input {
    type filter hook input priority 0;
    policy drop;

    iif lo accept
    ct state established,related accept

    # SSH quản trị
    tcp dport 22 ip saddr { 172.16.0.0/16 } accept

    # Ping nội bộ
    ip protocol icmp accept

    # SIP
    udp dport { 5060, 5061 } ip saddr { 172.16.0.0/16 } accept
    tcp dport { 5060, 5061 } ip saddr { 172.16.0.0/16 } accept

    # RTP
    udp dport 10000-20000 ip saddr { 172.16.0.0/16 } accept

    # PostgreSQL (2 node)
    tcp dport 5432 ip saddr { 172.16.91.101, 172.16.91.102 } accept

    # VoIP Admin (internal)
    tcp dport 8080 ip saddr { 172.16.0.0/16 } accept

    # VRRP
    ip protocol vrrp accept

    reject with icmpx type admin-prohibited
  }

  chain forward {
    type filter hook forward priority 0;
    policy drop;
  }

  chain output {
    type filter hook output priority 0;
    policy accept;
  }
}
```

Áp dụng:

```bash
nft -f /etc/nftables.conf
systemctl enable nftables
```

---

## 5. KEEPALIVED – VIP 172.16.91.100

### 5.1. Cài đặt

```bash
apt update
apt install -y keepalived
```

### 5.2. Script health-check stack VoIP

Tạo `/usr/local/sbin/check_voip_stack.sh`:

```bash
#!/bin/bash

pg_isready -h 127.0.0.1 -p 5432 >/dev/null 2>&1 || exit 1
pgrep kamailio >/dev/null 2>&1 || exit 1
pgrep freeswitch >/dev/null 2>&1 || exit 1
curl -sSf http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

exit 0
```

```bash
chmod +x /usr/local/sbin/check_voip_stack.sh
```

### 5.3. Cấu hình Node1 (MASTER)

`/etc/keepalived/keepalived.conf`:

```conf
vrrp_script chk_voip {
    script "/usr/local/sbin/check_voip_stack.sh"
    interval 5
    timeout 3
    fall 2
    rise 2
}

vrrp_instance VI_VOIP {
    state MASTER
    interface ens192
    virtual_router_id 51
    priority 150
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass ChangeThisPass
    }
    virtual_ipaddress {
        172.16.91.100/24
    }
    track_script {
        chk_voip
    }
}
```

### 5.4. Cấu hình Node2 (BACKUP)

Giống Node1 nhưng:

```conf
    state BACKUP
    priority 100
```

Enable & start:

```bash
systemctl enable keepalived
systemctl start keepalived
```

Kiểm tra VIP:

```bash
ip addr show ens192
```

---

## 6. BOTHWAY AUDIO SYNC GIỮA 2 NODE

### 6.1. Khái niệm

**Bothway audio sync giữa 2 node** trong ngữ cảnh này gồm:

1. **Không bị one-way audio** khi gọi qua VIP và khi VIP failover giữa 2 node.
2. **Media chảy đúng node FreeSWITCH** tương ứng với node Kamailio đang xử lý cuộc gọi.
3. **Cấu hình đồng bộ** giữa 2 FreeSWITCH để mọi cuộc gọi mới sau failover vẫn có đầy đủ dialplan/directory/IVR/queue giống nhau.

### 6.2. Thiết kế media & signaling

- SIP signaling:
  - Thiết bị SIP luôn đăng ký tới VIP `172.16.91.100` (Kamailio).
  - Kamailio active node chịu trách nhiệm xử lý signaling.
- Media (RTP):
  - Kamailio route INVITE sang FreeSWITCH **cùng node** (local IP 172.16.91.101 hoặc 172.16.91.102).
  - FreeSWITCH khai báo trong SDP (c=IN IP4) IP local của node, không dùng VIP.
- Khi failover:
  - VIP chuyển sang node còn lại → Kamailio trên node đó trở thành active.
  - FreeSWITCH trên node active xử lý *các cuộc gọi mới*; các cuộc gọi cũ (đang chạy) trên node trước đó sẽ tự giải phóng theo logic HA bạn chọn (thường không “teleport” media sang node mới).

### 6.3. Checklist đảm bảo bothway audio

1. **Đồng bộ cấu hình FreeSWITCH** (autoload configs, module list, `vars.xml`, v.v.) giữa 2 node:
   - Sử dụng Git, rsync, hoặc Ansible.
2. **Đồng bộ VoIP Admin Service + DB**:
   - Cả 2 node dùng chung PostgreSQL cluster.
   - VoIP Admin trên mỗi node kết nối cùng DB, logic directory/dialplan giống nhau.
3. **Kamailio dispatcher / routing**:
   - Cho cuộc gọi cần media, route sang FreeSWITCH `localhost` trên cùng node, không gửi sang node kia, để media không đi vòng.
4. **Firewall**:
   - Đảm bảo RTP UDP 10000–20000 được phép giữa:
     - FreeSWITCH ↔ Client SIP
     - FreeSWITCH ↔ SBC/Carrier
5. **Test manual**:
   - Gọi nội bộ trước khi failover.
   - Failover VIP (stop keepalived Node1), gọi cuộc mới, kiểm tra audio 2 chiều.
   - Kiểm tra không có trường hợp audio chỉ đi 1 chiều sau failover.

Nhận xét:
- HA theo kiểu VRRP ở đây là **connection-level HA**, không phải *media-level seamless failover* cho cuộc đang active (để làm được cần thêm SBC, media proxy chuyên dụng). Tuy nhiên cách này đảm bảo:
  - Cuộc cũ không làm hỏng nút mới.
  - Mọi cuộc mới đều có bothway audio và dialplan đầy đủ.

---

## 7. RUNBOOK HẠ TẦNG (VIEW TỪ GÓC ĐỘ SYSADMIN/NETWORK)

### 7.1. Khởi động hệ thống

1. Khởi động PostgreSQL trên cả 2 node.
2. Khởi động Kamailio trên cả 2 node.
3. Khởi động FreeSWITCH trên cả 2 node.
4. Khởi động VoIP Admin Service trên cả 2 node.
5. Khởi động Keepalived trên cả 2 node.
6. Xác nhận VIP nằm trên node mong muốn (ban đầu là Node1).

### 7.2. Failover có kiểm soát

- Muốn chuyển VIP từ Node1 sang Node2:

```bash
# trên Node1
systemctl stop keepalived
```

- Chờ 5–10 giây, kiểm tra VIP trên Node2:
```bash
ip addr show ens192
```

- Kiểm tra đăng ký SIP & test cuộc gọi.

### 7.3. Bảo trì Node (OS update)

1. Failover VIP sang node còn lại.
2. Dừng các service trên node cần bảo trì (Kamailio, FreeSWITCH, VoIP Admin, PostgreSQL nếu cần).
3. Thực hiện update (apt upgrade, reboot).
4. Khởi động lại dịch vụ, kiểm tra OK.
5. Quyết định có trả VIP về node này hay không.

### 7.4. Monitoring cơ bản

- Dùng Zabbix/Prometheus hoặc hệ thống sẵn có để giám sát:
  - Ping 172.16.91.100 (VIP).
  - Trạng thái service: `kamailio`, `freeswitch`, `voipadmind`, `postgresql`, `keepalived`.
  - Load, CPU, RAM, Disk, Network.
  - Số lượng REGISTER và active calls (từ Kamailio/FreeSWITCH).

---

## 8. TÓM TẮT

Tài liệu này chuẩn hóa lớp **OS/Network/HA** cho cụm VoIP 2 node:

- Tối ưu kernel & sysctl cho VoIP.
- Cấu hình firewall (nftables) cho SIP/RTP/DB/API.
- Cấu hình Keepalived với health-check stack hoàn chỉnh.
- Giải thích rõ khái niệm **bothway audio sync giữa 2 node** và cách đảm bảo thông qua:
  - Routing hợp lý Kamailio → FreeSWITCH.
  - Đồng bộ cấu hình FreeSWITCH & VoIP Admin.
  - Thực hành failover & test audio 2 chiều.

Các phần tiếp theo (tài liệu khác) sẽ tập trung lần lượt vào:
- PostgreSQL HA (DBA view).
- Kamailio/FreeSWITCH thiết kế logic SIP (SIP architect view).
- VoIP Admin Service (Go) – thiết kế & mã nguồn chi tiết (Backend engineer view).
