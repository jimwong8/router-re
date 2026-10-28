# 用户旅程端到端测试报告

> 生成: 2026-10-28 22:31:38

- J1-portmap: 5 PASS / 0 FAIL / 0 SKIP  (全通)
- J2-timesched: 6 PASS / 0 FAIL / 0 SKIP  (全通)
- J3-voucher: 5 PASS / 0 FAIL / 0 SKIP  (全通)

**汇总**: 16 步 | PASS 16 / FAIL 0 / SKIP 0

| # | 旅程 | 步骤 | 结果 | 说明 |
|---|------|------|------|------|
| 1 | J1-portmap | 1 POST 建 portmap 规则 | PASS | HTTP 200 |
| 2 | J1-portmap | 2 GET 列表确认存在 | PASS | HTTP 200 exists=True |
| 3 | J1-portmap | 3 nft 预览含规则 | PASS | nft contains 8080/192.168.1.10 = True |
| 4 | J1-portmap | 4 DELETE 规则 | PASS | HTTP 200 deleted=True |
| 5 | J1-portmap | 5 GET 确认消失 | PASS | HTTP 200 gone=True |
| 6 | J2-timesched | 1 POST 预览 (dry_run) | PASS | HTTP 200 dry_run=True |
| 7 | J2-timesched | 2 nft 预览生成 | PASS | nft ok=True |
| 8 | J2-timesched | 3 POST apply+save | PASS | HTTP 200 applied=True |
| 9 | J2-timesched | 4 GET 确认存在 | PASS | HTTP 200 present=True |
| 10 | J2-timesched | 5 DELETE 规则 | PASS | HTTP 200 deleted=True |
| 11 | J2-timesched | 6 GET 确认消失 | PASS | HTTP 200 gone=True |
| 12 | J3-voucher | 1 生成 60min voucher | PASS | HTTP 200 code=BE42ZAHK |
| 13 | J3-voucher | 2 portal/authorize MAC | PASS | HTTP 200 |
| 14 | J3-voucher | 3 GET portal/clients 确认在线 | PASS | HTTP 200 online=True count=1 |
| 15 | J3-voucher | 4 revoke MAC | PASS | HTTP 200 revoked=True |
| 16 | J3-voucher | 5 GET 确认下线 | PASS | HTTP 200 offline=True count=0 |

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

## 6. [J2-timesched] 1 POST 预览 (dry_run)

- 结果: **PASS** — HTTP 200 dry_run=True

```
{"applied": false, "count": 1, "dry_run": true, "nft": "table inet timesched {\n\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n\t\t# active rul...
```

## 7. [J2-timesched] 2 nft 预览生成

- 结果: **PASS** — nft ok=True

```
table inet timesched {
	chain forward {
		type filter hook forward priority filter; policy accept;
		# active rule: journey2
	}
}

```

## 8. [J2-timesched] 3 POST apply+save

- 结果: **PASS** — HTTP 200 applied=True

```
{"applied": true, "count": 1, "nft": "table inet timesched {\n\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n\t\t# active rule: journey2\n\t}\n...
```

## 9. [J2-timesched] 4 GET 确认存在

- 结果: **PASS** — HTTP 200 present=True

```
{"count": 1, "now": "2026-10-28 22:31:38 Wed", "rules": [{"id": "journey2", "name": "child-bedtime", "days": [0, 1, 2, 3, 4, 5, 6], "start_min": 1260, "end_min": 1439, "targets"...
```

## 10. [J2-timesched] 5 DELETE 规则

- 结果: **PASS** — HTTP 200 deleted=True

```
{"deleted": true, "rules": []}
```

## 11. [J2-timesched] 6 GET 确认消失

- 结果: **PASS** — HTTP 200 gone=True

```
{"count": 0, "now": "2026-10-28 22:31:38 Wed", "rules": [], "timezone": "Asia/Shanghai"}
```

## 12. [J3-voucher] 1 生成 60min voucher

- 结果: **PASS** — HTTP 200 code=BE42ZAHK

```
{"generated": 1, "note": "卡密明文只返回这一次，请立即保存；之后查询只能看到打码版本", "vouchers": [{"code": "BE42ZAHK", "password": "Q89PGGDK", "minutes": 60, "traffic_mb": 0, "device_limit": 1, "created_a...
```

## 13. [J3-voucher] 2 portal/authorize MAC

- 结果: **PASS** — HTTP 200

```
{"applied": true, "client": {"mac": "aa:bb:cc:dd:ee:ff", "ip": "192.168.1.200", "authorized": true, "authorized_at": 1793197898, "expires_at": 1793201498}, "nft": "table inet po...
```

## 14. [J3-voucher] 3 GET portal/clients 确认在线

- 结果: **PASS** — HTTP 200 online=True count=1

```
{"clients": [{"mac": "aa:bb:cc:dd:ee:ff", "ip": "192.168.1.200", "authorized": true, "authorized_at": 1793197898, "expires_at": 1793201498}]}
```

## 15. [J3-voucher] 4 revoke MAC

- 结果: **PASS** — HTTP 200 revoked=True

```
{"nft": "table inet portal {\n\tset authorized {\n\t\ttype ether_addr;\n\t\telements = {  }\n\t}\n\tchain forward {\n\t\ttype filter hook forward priority filter;\n\t\tether sad...
```

## 16. [J3-voucher] 5 GET 确认下线

- 结果: **PASS** — HTTP 200 offline=True count=0

```
{"clients": []}
```
