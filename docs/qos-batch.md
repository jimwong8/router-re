# p93-QoS 批测试报告（htb/lb/l7）

> 生成: 2026-10-29 04:48:19  BASE=http://127.0.0.1:9090  iface=qosdummy（dummy，安全）

**汇总**: 17 步 | PASS 16 / FAIL 0 / PARTIAL 1 / SKIP 0

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | 登录 admin | PASS | Bearer token 获取成功 |
| 2 | htb/apply dry-run | PASS | confirm=false 仅生成脚本不真下发 |
| 3 | htb/apply 真下发(p93test标记) | PASS | applied=true applied 1 classes on qosdummy (root 100mbit) |
| 4 | htb/config 读回确认 | PASS | enabled=True iface=qosdummy classes=1 |
| 5 | htb 真实 tc 状态 | PASS | tc qdisc show dev qosdummy 含 htb |
| 6 | htb/cleanup | PASS | applied=true iface=qosdummy |
| 7 | htb 清理后确认空 | PASS | tc 无 htb qdisc |
| 8 | lb/config 保存(setup) | PASS | HTTP 200 marks={'wan1': 150} |
| 9 | lb/apply | PASS | HTTP 200 applied=True marks={'wan1': 150} mark table + ip rules applied |
| 10 | lb/check 确认 | PASS | HTTP 200 status条数=1 up=True |
| 11 | lb 清理 | PASS | wan1(fwmark 150) 已清除; nft router_lb_marks 已删 |
| 12 | l7/rules 保存(setup) | PASS | HTTP 200 rules=1 |
| 13 | l7/apply | PARTIAL | HTTP 200 applied=False conf写入=True 配置已写入 /etc/dnsmasq.d/router-re-l7.conf，但 dnsmasq 未重启：systemctl: /usr/bin/systemctl restart dnsmasq: Failed to restart dnsmasq.service: Unit dnsmasq.service not found.；init.d: /etc/init.d/dnsmasq restart: fork/exec /etc/init.d/dnsmasq: no such file or directory |
| 14 | l7/test 命中阻断 | PASS | hit=True blocked=True by=p93test-block |
| 15 | l7/test 不命中反例 | PASS | 无关域名未被命中 |
| 16 | l7/rules 清理 | PASS | rules 已清空 |
| 17 | l7/apply 清理复写 | PASS | HTTP 200 applied=False conf为空=True |

## 清理状态（终态）

### HTB
- `qosdummy` tc qdisc: `<none>` → 干净
- `lo` tc qdisc: `qdisc noqueue 0: root refcnt 2` → 干净

### LB
- `ip rule` fwmark 残留: <none> → 干净
- nft table 残留: `<none>` → 干净

### L7
- `/etc/dnsmasq.d/router-re-l7.conf`: 存在（0 字节）→ 干净（空文件）

## 详情

### 1. 登录 admin

- 结果: **PASS** — Bearer token 获取成功

```
token:<redacted>
```

### 2. htb/apply dry-run

- 结果: **PASS** — confirm=false 仅生成脚本不真下发

```
{"applied": false, "detail": "dry-run：未下发。要用同一份 config 真正应用，请带 confirm=true。这会重置该接口现有的 qdisc。", "dry_run": true, "script": "# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+add 幂等）\ntc qdisc del dev qosdummy root 2>/dev/null || true\n# 建立根 HTB qdisc，默认类 1:10\ntc qdisc add dev qosdummy root handle 1: htb d...
```

### 3. htb/apply 真下发(p93test标记)

- 结果: **PASS** — applied=true applied 1 classes on qosdummy (root 100mbit)

```
{"applied": true, "detail": "applied 1 classes on qosdummy (root 100mbit)", "dry_run": false, "script": "# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+add 幂等）\ntc qdisc del dev qosdummy root 2>/dev/null || true\n# 建立根 HTB qdisc，默认类 1:10\ntc qdisc add dev qosdummy root handle 1: htb default 10\n# 根类：总带宽...
```

### 4. htb/config 读回确认

- 结果: **PASS** — enabled=True iface=qosdummy classes=1

```
{"applied": true, "config": {"iface": "qosdummy", "root_rate_mbit": 100, "default_class": "p93test", "classes": [{"name": "p93test", "rate_mbit": 20, "ceil_mbit": 80, "priority": 2, "match_ports": [8080], "match_dst_ips": ["127.0.0.1"]}], "enabled": true}}
```

### 5. htb 真实 tc 状态

- 结果: **PASS** — tc qdisc show dev qosdummy 含 htb

```
qdisc htb 1: root refcnt 2 r2q 10 default 0x10 direct_packets_stat 0 direct_qlen 1000
```

### 6. htb/cleanup

- 结果: **PASS** — applied=true iface=qosdummy

```
{"applied": true, "iface": "qosdummy"}
```

### 7. htb 清理后确认空

- 结果: **PASS** — tc 无 htb qdisc

```

```

### 8. lb/config 保存(setup)

- 结果: **PASS** — HTTP 200 marks={'wan1': 150}

```
{"applied": true, "config": {"mode": "weighted", "members": [{"name": "wan1", "iface": "ens18", "gateway": "10.100.1.1", "weight": 2, "enabled": true}], "health": {"target": "10.100.1.1", "interval_sec": 5, "timeout_sec": 1, "fail_threshold": 3, "rise_threshold": 2}}, "detail": "mark table + ip r...
```

### 9. lb/apply

- 结果: **PASS** — HTTP 200 applied=True marks={'wan1': 150} mark table + ip rules applied

```
{"applied": true, "config": {"mode": "weighted", "members": [{"name": "wan1", "iface": "ens18", "gateway": "10.100.1.1", "weight": 2, "enabled": true}], "health": {"target": "10.100.1.1", "interval_sec": 5, "timeout_sec": 1, "fail_threshold": 3, "rise_threshold": 2}}, "detail": "mark table + ip r...
```

### 10. lb/check 确认

- 结果: **PASS** — HTTP 200 status条数=1 up=True

```
[{"name": "wan1", "up": true, "fail_count": 0, "rise_count": 1, "last_check": 1793220498, "latency_ms": 281, "last_error": ""}]
```

### 11. lb 清理

- 结果: **PASS** — wan1(fwmark 150) 已清除; nft router_lb_marks 已删

```
marks={'wan1': 150}
```

### 12. l7/rules 保存(setup)

- 结果: **PASS** — HTTP 200 rules=1

```
{"applied": false, "detail": "规则已保存；调用 /api/v1/l7/apply 写入 dnsmasq 并重启", "dnsmasq": "address=/p93test.example.com/0.0.0.0\naddress=/p93test.example.com/::\n", "rules": [{"id": "p93test", "name": "p93test-block", "match_type": "dns", "pattern": "p93test.example.com", "action": "block", "enabled": ...
```

### 13. l7/apply

- 结果: **PARTIAL** — HTTP 200 applied=False conf写入=True 配置已写入 /etc/dnsmasq.d/router-re-l7.conf，但 dnsmasq 未重启：systemctl: /usr/bin/systemctl restart dnsmasq: Failed to restart dnsmasq.service: Unit dnsmasq.service not found.；init.d: /etc/init.d/dnsmasq restart: fork/exec /etc/init.d/dnsmasq: no such file or directory

```
{"applied":false,"detail":"配置已写入 /etc/dnsmasq.d/router-re-l7.conf，但 dnsmasq 未重启：systemctl: /usr/bin/systemctl restart dnsmasq: Failed to restart dnsmasq.service: Unit dnsmasq.service not found.；init.d: /etc/init.d/dnsmasq restart: fork/exec /etc/init.d/dnsmasq: no such file or directory","dnsmasq...
```

### 14. l7/test 命中阻断

- 结果: **PASS** — hit=True blocked=True by=p93test-block

```
{"blocked": true, "by": "p93test-block", "hit": [{"id": "p93test", "name": "p93test-block", "match_type": "dns", "pattern": "p93test.example.com", "action": "block", "enabled": true}]}
```

### 15. l7/test 不命中反例

- 结果: **PASS** — 无关域名未被命中

```
{"blocked": false, "by": "", "hit": null}
```

### 16. l7/rules 清理

- 结果: **PASS** — rules 已清空

```
{"applied": false, "detail": "规则已保存；调用 /api/v1/l7/apply 写入 dnsmasq 并重启", "dnsmasq": "", "rules": []}
```

### 17. l7/apply 清理复写

- 结果: **PASS** — HTTP 200 applied=False conf为空=True

```
{"applied":false,"detail":"配置已写入 /etc/dnsmasq.d/router-re-l7.conf，但 dnsmasq 未重启：systemctl: /usr/bin/systemctl restart dnsmasq: Failed to restart dnsmasq.service: Unit dnsmasq.service not found.；init.d: /etc/init.d/dnsmasq restart: fork/exec /etc/init.d/dnsmasq: no such file or directory","dnsmasq...
```
