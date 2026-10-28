# 认证门户会话管理生命周期测试报告 (portal-lifecycle-test)

> 生成: 2026-10-28 20:43:04  BASE=http://127.0.0.1:9090

**汇总**: 共 8 步 / PASS 8 / FAIL 0 / SKIP 0

| 步骤 | 结果 | 详情 |
|---|---|---|
| 1. GET portal/clients 初始 count | PASS | HTTP 200 initial_count=0 macs=[] |
| 2. POST portal/authorize | PASS | HTTP 200 applied=True client={'mac': 'aa:bb:cc:dd:ee:ff', 'ip': '192.168.9.9', 'authorized': True, 'authorized_at': 1793191384, 'expires_at': 1793193184} |
| 3. GET portal/clients 含 mac/count+1 | PASS | HTTP 200 count=1 (init 0) present=True increased=True |
| 4. POST portal/revoke | PASS | HTTP 200 revoked=True |
| 5. GET portal/clients mac消失/count恢复 | PASS | HTTP 200 count=0 (init 0) gone=True restored=True |
| 6. GET portal/config 字段 | PASS | HTTP 200 fields=['default_minutes', 'enabled', 'portal_port'] |
| 7. POST portal/config 写+读回 | PASS | write HTTP 200 (enabled=True, minutes=45); readback HTTP 200 (enabled=True, minutes=45) |
| 8. 负向 authorize 空 mac → 400/10048 | PASS | HTTP 400 code=10048 (expect 400 / 10048) |

## 真实返回片段

### 1. GET portal/clients 初始 count — PASS

```
{"clients":[]} 
```

### 2. POST portal/authorize — PASS

```
{"applied":true,"client":{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.9.9","authorized":true,"authorized_at":1793191384,"expires_at":1793193184},"nft":"table inet portal {\n\tset authorized {\n\t\ttype ether_addr;\n\t\telements = { aa:bb:cc:dd:ee:ff }\n\t}\n\tchai...
```

### 3. GET portal/clients 含 mac/count+1 — PASS

```
{"clients":[{"mac":"aa:bb:cc:dd:ee:ff","ip":"192.168.9.9","authorized":true,"authorized_at":1793191384,"expires_at":1793193184}]} 
```

### 4. POST portal/revoke — PASS

```
{"nft":"table inet portal {\n\tset authorized {\n\t\ttype ether_addr;\n\t\telements = {  }\n\t}\n\tchain forward {\n\t\ttype filter hook forward priority filter;\n\t\tether saddr @authorized accept\n\t\tudp dport 53 accept\n\t\ttcp dport 8080 accept\n\t\tip pr...
```

### 5. GET portal/clients mac消失/count恢复 — PASS

```
{"clients":[]} 
```

### 6. GET portal/config 字段 — PASS

```
{"default_minutes":60,"enabled":false,"portal_port":8080} 
```

### 7. POST portal/config 写+读回 — PASS

```
{"default_minutes":45,"enabled":true,"portal_port":8080} 
```

### 8. 负向 authorize 空 mac → 400/10048 — PASS

```
{"code":"10048","detail":"invalid MAC address","message":"invalid MAC"} 
```

## 说明

- applied=false 表示本机无 nft 权限，属设计允许（client 已记录），不算失败。
- Step 7 将 default_minutes 设为 45 并读回验证。

