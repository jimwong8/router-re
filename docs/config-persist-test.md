# Config Persist Test (重启后持久化)

- Base: http://127.0.0.1:9090
- Date: 2026-10-28 22:14:43

## 1. 写配置（标记值）

- POST /api/v1/ddns/config → HTTP 200 PASS
- POST /api/v1/portal/config → HTTP 200 PASS

## 2. 重启前读回

- GET /api/v1/ddns/config → provider=p71test PASS
- GET /api/v1/portal/config → port=9999 min=123 PASS

## 3. 重启 mgmtd

- 重启: 重启成功 PASS

## 4. 重启后读回

- GET /api/v1/ddns/config → provider=p71test domain=p71test.example.com PASS
- GET /api/v1/portal/config → port=9999 min=123 PASS

## 5. 重启前后对比

- ddns provider: before=p71test, after=p71test
- portal port: before=9999, after=9999
- portal minutes: before=123, after=123

## 6. 结论
- 配置持久化: PASS
- 重启后配置一致: 是
