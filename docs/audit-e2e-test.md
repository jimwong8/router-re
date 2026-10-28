# 审计日志端到端测试报告

> 生成: 2026-10-28 23:22:06

**汇总**: 7 步 | PASS 7 / FAIL 0

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | 0 捕获基线 (fw/ddns/qos + audit) | PASS | fw_rules=1 qos=0 audit_count=3660 latest_ts=1793200925 |
| 2 | 1a POST ddns/config | PASS | HTTP 200 |
| 3 | 1b POST qos/limits | PASS | HTTP 200 |
| 4 | 1c POST firewall/rules | PASS | HTTP 200 |
| 5 | 1d 恢复 firewall 基线 | PASS | HTTP 200 |
| 6 | 2 GET audit 新增记录 | PASS | HTTP 200 new=7 actions=['ddns.config.set', 'firewall.rules.set', 'qos.limits.set'] expected_subset=True |
| 7 | 3 审计记录含 actor=admin 与 ts | PASS | all expected events have actor=admin & ts>0: True |

## 1. 0 捕获基线 (fw/ddns/qos + audit)

- 结果: **PASS** — fw_rules=1 qos=0 audit_count=3660 latest_ts=1793200925

```
{"fw": [{"Chain": "input", "Src": "", "Dst": "", "Proto": "tcp", "DPort": "8080", "Action": "accept"}], "ddns": {"provider": "none", "domain": "audit-test.example.com", "subdomain": "t", "token": "12****78", "record_id": "", "zone_id": "", "enabled": false}, "qos": [], "audit_count": 3660}
```

## 2. 1a POST ddns/config

- 结果: **PASS** — HTTP 200

```
{"config": {"provider": "none", "domain": "audit-test.example.com", "subdomain": "t", "token": "12****78", "record_id": "", "zone_id": "", "enabled": false}, "saved": true}
```

## 3. 1b POST qos/limits

- 结果: **PASS** — HTTP 200

```
{"limits": [], "nft": "table inet qos_limit {\n}\n", "tc": []}
```

## 4. 1c POST firewall/rules

- 结果: **PASS** — HTTP 200

```
{"nft": "table inet router {\n  chain input {\n    type filter hook input priority 0; policy accept;\n    tcp dport 8080 accept\n  }\n  chain forward {\n    type filter hook forward priority 0; policy accept;\n    ip saddr 10.99.99.99 tc...
```

## 5. 1d 恢复 firewall 基线

- 结果: **PASS** — HTTP 200

```
{"nft": "table inet router {\n  chain input {\n    type filter hook input priority 0; policy accept;\n    tcp dport 8080 accept\n  }\n  chain forward {\n    type filter hook forward priority 0; policy accept;\n  }\n  chain output {\n    ...
```

## 6. 2 GET audit 新增记录

- 结果: **PASS** — HTTP 200 new=7 actions=['ddns.config.set', 'firewall.rules.set', 'qos.limits.set'] expected_subset=True

```
[{"ts": 1793200925, "actor": "admin", "action": "firewall.rules.set", "target": "/api/v1/firewall/rules", "result": "ok", "detail": "HTTP 200", "remote_ip": "127.0.0.1"}, {"ts": 1793200925, "actor": "admin", "action": "firewall.rules.set", "target": "1 rules", "result": "ok", "detail": "", "remote_ip": "127.0.0.1"}, {"ts": 1793200925, "actor": "admin", "action": "firewall.rules.set", "target": "/api/v1/firewall/rules", "result": "ok", "detail": "HTTP 200", "remote_ip": "127.0.0.1"}, {"ts": 17...
```

## 7. 3 审计记录含 actor=admin 与 ts

- 结果: **PASS** — all expected events have actor=admin & ts>0: True

```
[{"action": "firewall.rules.set", "actor": "admin", "ts": 1793200925, "target": "/api/v1/firewall/rules"}, {"action": "firewall.rules.set", "actor": "admin", "ts": 1793200925, "target": "1 rules"}, {"action": "firewall.rules.set", "actor": "admin", "ts": 1793200925, "target": "/api/v1/firewall/rules"}, {"action": "firewall.rules.set", "actor": "admin", "ts": 1793200925, "target": "2 rules"}, {"...
```
