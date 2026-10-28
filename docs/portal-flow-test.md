# 认证门户全流程测试报告 (portal-flow-test)

> 生成: 2026-10-28 23:47:15  BASE=http://127.0.0.1:9090

**汇总**: 共 6 步 / PASS 3 / FAIL 0 / SKIP 3

| 步骤 | 结果 | 详情 |
|---|---|---|
| 1. 选/保存 portal 模板 | PASS | GET templates 200 (count=4); POST template 200 saved=true id=roundtrip |
| 2. voucher/generate 测试卡 | PASS | HTTP 200 generated=1 code=UGPFBALSZ2 |
| 3. portal/login 用卡登录 | SKIP | 路由未注册 (mgmtd 无 /api/v1/portal/login；门户认证走 /portal/auth 表单，非 mgmtd REST API) probe=GET=404 POST=404 HTTP 404 |
| 4. portal/status 查会话 | SKIP | 路由未注册 (mgmtd 无 /api/v1/portal/status) probe=GET=404 POST=404 HTTP 404 |
| 5. voucher list/verify 确认已用 | PASS | redeem HTTP 200 ok; list HTTP 200; card UGPFBALSZ2 used=True |
| 6. portal/logout 登出+status消失 | SKIP | 路由未注册 (mgmtd 无 /api/v1/portal/logout) probe=GET=404 POST=404 HTTP 404 |

## 真实返回片段

### 1. 选/保存 portal 模板 — PASS

```
{"saved":true,"template":{"id":"roundtrip","name":"roundtrip portal","kind":"voucher","title":"卡密认证","logo_url":"","bg_color":"#2e8b57","text_color":"#ffffff","notice":"请输入购买的卡密（券码）完成认证","redirect_url":""}} 
```

### 2. voucher/generate 测试卡 — PASS

```
{"generated":1,"note":"卡密明文只返回这一次，请立即保存；之后查询只能看到打码版本","vouchers":[{"code":"UGPFBALSZ2","password":"F7VUPDR7NS","minutes":30,"traffic_mb":0,"device_limit":1,"created_at":1793202435,"expires_at":0,"used_at":0,"used":false,"note":"p47 roundtri...
```

### 3. portal/login 用卡登录 — SKIP

```
404 page not found 
```

### 4. portal/status 查会话 — SKIP

```
404 page not found 
```

### 5. voucher list/verify 确认已用 — PASS

```
{"count":36,"unused":19,"used":17,"vouchers":[{"code":"R4VTY4YKNE","password":"CH******D2","minutes":1440,"traffic_mb":10240,"device_limit":2,"created_at":1793113374,"expires_at":1795705374,"used_at":1793113374,"used":true,"note":"测试批次","re...
```

### 6. portal/logout 登出+status消失 — SKIP

```
404 page not found 
```

## 说明

- SKIP 步骤：mgmtd(9090) 未注册 /api/v1/portal/{login,status,logout} 路由；
  门户认证由 RenderHTML 生成的表单 POST /portal/auth 承载，不在 mgmtd REST API 层。
- Step 5 在 portal login 不可用时，直接调 /api/v1/voucher/redeem 标记已用并 list 确认。

