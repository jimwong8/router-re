# 用户旅程端到端测试报告

> 生成: 2026-10-28 22:25:01

- J1-portmap: 5 PASS / 0 FAIL / 0 SKIP  (全通)
- J2-timesched: 3 PASS / 2 FAIL / 0 SKIP  (部分)
- J3-voucher: 5 PASS / 0 FAIL / 0 SKIP  (全通)

**汇总**: 15 步 | PASS 13 / FAIL 2 / SKIP 0

| # | 旅程 | 步骤 | 结果 | 说明 |
|---|------|------|------|------|
| 1 | J1-portmap | 1 POST 建 portmap 规则 | PASS | HTTP 200 |
| 2 | J1-portmap | 2 GET 列表确认存在 | PASS | HTTP 200 exists=True |
| 3 | J1-portmap | 3 nft 预览含规则 | PASS | nft contains 8080/192.168.1.10 = True |
| 4 | J1-portmap | 4 DELETE 规则 | PASS | HTTP 200 deleted=True |
| 5 | J1-portmap | 5 GET 确认消失 | PASS | HTTP 200 gone=True |
| 6 | J2-timesched | 1 POST 建时段规则 (dry_run) | PASS | HTTP 200 dry_run=True |
| 7 | J2-timesched | 2 GET 确认存在 | FAIL | HTTP 200 present=False |
| 8 | J2-timesched | 3 nft 预览生成 | FAIL | nft ok=False |
| 9 | J2-timesched | 4 清理 (enabled:false, 无 DELETE 端点) | PASS | HTTP 200 (timeschedule 无 DELETE，用 disable 清理) |
| 10 | J2-timesched | 5 GET 确认 (disabled) | PASS | HTTP 200 count=1 |
| 11 | J3-voucher | 1 生成 60min voucher | PASS | HTTP 200 code=BE6SNJQS |
| 12 | J3-voucher | 2 portal/authorize MAC | PASS | HTTP 200 |
| 13 | J3-voucher | 3 GET portal/clients 确认在线 | PASS | HTTP 200 online=True count=1 |
| 14 | J3-voucher | 4 revoke MAC | PASS | HTTP 200 revoked=True |
| 15 | J3-voucher | 5 GET 确认下线 | PASS | HTTP 200 offline=True count=0 |

## 1. [J1-portmap] 1 POST 建 portmap 规则

- 结果: **PASS** — HTTP 200

```
{"applied": true, "nft": "table inet router_portmap {\n\tchain prerouting {\n\t\ttype nat hook prerouting priority dstnat; policy accept;\n\t\ttcp dport 8080 dnat ip to 192.168....
```

## 2. [J1-portmap] 2 GET 列表确认存在

- 结果: **PASS** — HTTP 200 exists=True

```
{"nft": "table inet router_portmap {\n\tchain prerouting {\n\t\ttype nat hook prerouting priority dstnat; policy accept;\n\t\ttcp dport 8080 dnat ip to 192.168.1.10:80\n\t}\n\n\...
```

## 3. [J1-portmap] 3 nft 预览含规则

- 结果: **PASS** — nft contains 8080/192.168.1.10 = True

```
table inet router_portmap {
	chain prerouting {
		type nat hook prerouting priority dstnat; policy accept;
		tcp dport 8080 dnat ip to 192.168.1.10:80
	}

	chain forward {
		typ...
```

## 4. [J1-portmap] 4 DELETE 规则

- 结果: **PASS** — HTTP 200 deleted=True

```
{"applied": true, "deleted": true, "rules": []}
```

## 5. [J1-portmap] 5 GET 确认消失

- 结果: **PASS** — HTTP 200 gone=True

```
{"nft": "table inet router_portmap {\n\tchain prerouting {\n\t\ttype nat hook prerouting priority dstnat; policy accept;\n\t}\n\n\tchain forward {\n\t\ttype filter hook forward ...
```

## 6. [J2-timesched] 1 POST 建时段规则 (dry_run)

- 结果: **PASS** — HTTP 200 dry_run=True

```
{"applied": false, "count": 1, "dry_run": true, "nft": "table inet timesched {\n\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n\t\t# active rul...
```

## 7. [J2-timesched] 2 GET 确认存在

- 结果: **FAIL** — HTTP 200 present=False

```
{"count": 1, "now": "2026-10-28 22:25:00 Wed", "rules": [{"id": "rt-rule-1", "name": "roundtrip", "days": [1, 2, 3, 4, 5], "start_min": 60, "end_min": 120, "targets": ["10.0.0.0...
```

## 8. [J2-timesched] 3 nft 预览生成

- 结果: **FAIL** — nft ok=False

```
table inet timesched {
	chain forward {
		type filter hook forward priority filter; policy accept;
		# active rule: journey2
	}
}

```

## 9. [J2-timesched] 4 清理 (enabled:false, 无 DELETE 端点)

- 结果: **PASS** — HTTP 200 (timeschedule 无 DELETE，用 disable 清理)

```
{"applied": false, "count": 1, "dry_run": true, "nft": "table inet timesched {\n\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n\t\t# no active ...
```

## 10. [J2-timesched] 5 GET 确认 (disabled)

- 结果: **PASS** — HTTP 200 count=1

```
{"count": 1, "now": "2026-10-28 22:25:00 Wed", "rules": [{"id": "rt-rule-1", "name": "roundtrip", "days": [1, 2, 3, 4, 5], "start_min": 60, "end_min": 120, "targets": ["10.0.0.0...
```

## 11. [J3-voucher] 1 生成 60min voucher

- 结果: **PASS** — HTTP 200 code=BE6SNJQS

```
{"generated": 1, "note": "卡密明文只返回这一次，请立即保存；之后查询只能看到打码版本", "vouchers": [{"code": "BE6SNJQS", "password": "H8DPG4CH", "minutes": 60, "traffic_mb": 0, "device_limit": 1, "created_a...
```

## 12. [J3-voucher] 2 portal/authorize MAC

- 结果: **PASS** — HTTP 200

```
{"applied": true, "client": {"mac": "aa:bb:cc:dd:ee:ff", "ip": "192.168.1.200", "authorized": true, "authorized_at": 1793197500, "expires_at": 1793201100}, "nft": "table inet po...
```

## 13. [J3-voucher] 3 GET portal/clients 确认在线

- 结果: **PASS** — HTTP 200 online=True count=1

```
{"clients": [{"mac": "aa:bb:cc:dd:ee:ff", "ip": "192.168.1.200", "authorized": true, "authorized_at": 1793197500, "expires_at": 1793201100}]}
```

## 14. [J3-voucher] 4 revoke MAC

- 结果: **PASS** — HTTP 200 revoked=True

```
{"nft": "table inet portal {\n\tset authorized {\n\t\ttype ether_addr;\n\t\telements = {  }\n\t}\n\tchain forward {\n\t\ttype filter hook forward priority filter;\n\t\tether sad...
```

## 15. [J3-voucher] 5 GET 确认下线

- 结果: **PASS** — HTTP 200 offline=True count=0

```
{"clients": []}
```
