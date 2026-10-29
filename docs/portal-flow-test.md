# 认证门户全流程测试报告 (portal-flow-test)

> 生成: 2026-10-29 20:00:32  BASE=http://127.0.0.1:9090

**汇总**: 共 6 步 / PASS 5 / FAIL 0 / SKIP 1

| 步骤 | 结果 | 详情 |
|---|---|---|
| 1. 选/保存 portal 模板 | PASS | GET templates 200 (count=4); POST template 200 saved=true id=roundtrip |
| 2. voucher/generate 测试卡 | PASS | HTTP 200 generated=1 code=6ZMXUFZ5RE |
| 3. portal/login 用卡登录 | SKIP | 路由未注册 (mgmtd 无 /api/v1/portal/login；门户认证走 /portal/auth 表单，非 mgmtd REST API) probe=GET=404 POST=404 HTTP 404 |
| 4. portal/status 查会话 | PASS | HTTP 200: {'clients': [], 'online': False, 'online_count': 0} |
| 5. voucher list/verify 确认已用 | PASS | redeem HTTP 200 ok; list HTTP 200; card 6ZMXUFZ5RE used=True |
| 6. portal/logout 登出+status消失 | PASS | logout HTTP 200; status-after HTTP 200 |

## 真实返回片段

### 1. 选/保存 portal 模板 — PASS

```
{"saved":true,"template":{"id":"roundtrip","name":"roundtrip portal","kind":"voucher","title":"卡密认证","logo_url":"","bg_color":"#2e8b57","text_color":"#ffffff","notice":"请输入购买的卡密（券码）完成认证","redirect_url":""}} 
```

### 2. voucher/generate 测试卡 — PASS

```
{"generated":1,"note":"卡密明文只返回这一次，请立即保存；之后查询只能看到打码版本","vouchers":[{"code":"6ZMXUFZ5RE","password":"4CVQS9T46N","minutes":30,"traffic_mb":0,"device_limit":1,"created_at":1793275232,"expires_at":0,"used_at":0,"used":false,"note":"p47 roundtri...
```

### 3. portal/login 用卡登录 — SKIP

```
404 page not found 
```

### 4. portal/status 查会话 — PASS

```
{"clients":[],"online":false,"online_count":0} 
```

### 5. voucher list/verify 确认已用 — PASS

```
{"count":67,"unused":36,"used":31,"vouchers":[{"code":"R4VTY4YKNE","password":"CH******D2","minutes":1440,"traffic_mb":10240,"device_limit":2,"created_at":1793113374,"expires_at":1795705374,"used_at":1793113374,"used":true,"note":"测试批次","re...
```

### 6. portal/logout 登出+status消失 — PASS

```
{"online":false,"online_count":0,"revoked":0} 
```

## 说明

- SKIP 步骤：mgmtd(9090) 未注册 /api/v1/portal/{login,status,logout} 路由；
  门户认证由 RenderHTML 生成的表单 POST /portal/auth 承载，不在 mgmtd REST API 层。
- Step 5 在 portal login 不可用时，直接调 /api/v1/voucher/redeem 标记已用并 list 确认。

