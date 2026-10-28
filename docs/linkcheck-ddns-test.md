# linkcheck + DDNS 状态流转闭环验证报告

> 生成: 2026-10-28 23:48:24  BASE=http://127.0.0.1:9090

**汇总**: 5 步 | PASS 5 / FAIL 0 / SKIP 0

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | 1. linkcheck 建 target (POST targets) | PASS | write=200 read=200 found=True |
| 2 | 2. linkcheck probe + status (states 含该 target) | PASS | probe=200 status=200 state_present=True |
| 3 | 3. ddns 建 record (POST records) | PASS | write=200 read=200 found=True |
| 4 | 4. ddns check + update 结构 (非 500 崩) | PASS | check=200 update=400 |
| 5 | 5. 清理 (DELETE 端点) | PASS | target 已删=是 / record 已删=是 |

## 1. 1. linkcheck 建 target (POST targets)

- 结果: **PASS** — write=200 read=200 found=True

```
write_resp={"count": 1, "targets": [{"id": "p48-icmp-1", "name": "p48 roundtrip", "iface": "lo", "kind": "icmp", "host": "127.0.0.1", "url": "", "interval": 5, "timeout": 3, "fail_threshold": 2, "recover_threshold": 1, "enabled": true}]}
read_targets={"count": 1, "states": {}, "targets": [{"id": "p48-icmp-1", "name": "p48 roundtrip", "iface": "lo", "kind": "icmp", "host": "127.0.0.1", "url": "", "interval": 5, "timeout": 3, "fail_threshold": 2, "recover_threshold": 1, "enabled": true}]}
```

## 2. 2. linkcheck probe + status (states 含该 target)

- 结果: **PASS** — probe=200 status=200 state_present=True

```
probe_resp={"id": "p48-icmp-1", "ok": true, "output": "PING 127.0.0.1 (127.0.0.1) 56(84) bytes of data.\n64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.157 ms\n\n--- 127.0.0.1 ping statistics ---\n1 packets transmitted, 1 received, 0% packet loss, time 0ms\nrtt min/avg/max/mdev = 0.157/0.157/0.157/0.000 ms", "reason": "icmp ok", "state": {"up": true, "consec_fail": 0, "consec_ok": 1, "last_change": 179...
status={"count": 1, "down": 0, "states": {"p48-icmp-1": {"up": true, "consec_fail": 0, "consec_ok": 1, "last_change": 1793202501, "last_error": "", "reason": "recovered"}}, "up": 1}
```

## 3. 3. ddns 建 record (POST records)

- 结果: **PASS** — write=200 read=200 found=True

```
write_resp={"count": 1, "records": [{"id": "p48-ddns-1", "provider": "cloudflare", "domain": "example.com", "subdomain": "p48", "token": "p48r*************1234", "enabled": true, "last_ip": "", "last_ok": 0, "last_error": ""}]}
read_records={"count": 1, "providers": [{"key": "cloudflare", "name": "Cloudflare", "needs_token": true, "needs_user": false, "endpoint": "https://api.cloudflare.com/client/v4/zones/{zone}/dns_records/{id}"}, {"key": "aliyun", "name": "Aliyun", "needs_token": true, "needs_user": false, "endpoint": "https://alidns.aliyuncs.com/"}, {"key": "duckdns", "name": "DuckDNS", "needs_token": true, "needs_user": false...
```

## 4. 4. ddns check + update 结构 (非 500 崩)

- 结果: **PASS** — check=200 update=400

```
check_resp={"available": true, "current_ip": "209.33.171.48", "need_update": true, "note": "这里只判断是否需要更新，不代为调用各服务商 API", "records": [{"id": "p48-ddns-1", "domain": "p48.example.com", "last_ip": "", "needs_update": true}], "source": "api.ipify.org"}
update_resp={"code": "10113", "detail": "none", "message": "unsupported provider"}
```

## 5. 5. 清理 (DELETE 端点)

- 结果: **PASS** — target 已删=是 / record 已删=是

```
linkcheck DELETE -> 200 {"deleted": true}
ddns DELETE -> 200 {"deleted": true}
```

## 附：linkcheck status 原始 states

```json
{
  "count": 1,
  "down": 0,
  "states": {
    "p48-icmp-1": {
      "up": true,
      "consec_fail": 0,
      "consec_ok": 1,
      "last_change": 1793202501,
      "last_error": "",
      "reason": "recovered"
    }
  },
  "up": 1
}
```
