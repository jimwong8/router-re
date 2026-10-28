# Real-Environment Verification Report

- Host: 10.100.1.132 (router-re-test)
- Date: 2026-10-28 19:53:41
- mgmtd: bin/mgmtd-132 (CGO=0), :9090
- Token: admin/admin123 -> OK

## 1. Health check
- HTTP 200: `{"status":"ok"}`
- **PASS**

## 2. Login
- HTTP 200: `{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3OTMxODkzMTksImlhdCI6M`
- **PASS**

## 3. GET /api/v1/terminals (real ARP/conntrack)
- HTTP 200
- count: 3
- terminals elements: 3
- has real MAC: True
- sample: `[{"hostname": "", "iface": "eth0", "ip": "10.100.1.1", "mac": "64:51:06:3f:e1:62", "online": true, "source": "arp"}, {"hostname": "", "iface": "eth0", "ip": "10.100.1.18", "mac": "bc:24:11:35:82:d1", "online": true, "source": "arp"}]`
- **PASS** (count>0 and real MAC present)

## 4. POST /api/v1/conntrack/acct {on:true, persist:true}
- HTTP 200: `{"applied":true,"on":true,"persisted":true}`
- applied=true: True
- **PASS**

## 5. GET /api/v1/conntrack/acct (readback)
- HTTP 200: `{"on":true,"path":"/proc/sys/net/netfilter/nf_conntrack_acct","persisted":true}`
- on=true: True
- persisted: True
- **PASS**

## 6. GET /api/v1/online (sessions structure)
- HTTP 200
- sessions count: 0
- sample: `[]`
- **PASS**

## 7. POST /api/v1/firmware/confirm (bad sha256 -> 422/10500)
- HTTP 422: `{"code":"10500","detail":"body sha256=0000000000000000000000000000000000000000000000000000000000000000 but inspected image sha256=81a8d54107fbd58bfd3a56deee80a503f7a9ed3ae69b8e2d0664a290fda824f2","message":"sha256 mismatch"}`
- code=10500: True
- **PASS**

## 8. GET /api/v1/system/info
- HTTP 200: `{"hostname":"router-re-test","cpu_model":"Intel(R) Xeon(R) CPU E5-2697 v2 @ 2.70GHz","cpu_cores":4,"mem_total_kb":4194304,"mem_avail_kb":4039252,"os_version":"Debian GNU/Linux 12 (bookworm)","go_version":"go1.26.10"}`
- **PASS**

## Summary
- PASS: 6, FAIL: 0
- Total checks: 6

## Differences vs local 127.0.0.1:9090
- 132 has real network stack (ARP/conntrack), terminals count reflects real devices
- 132 runs as root with CGO=0 build (bin/mgmtd-132); local dev runs CGO=1
- conntrack/acct write tested for real on 132 (local dev may lack nf_conntrack module)
