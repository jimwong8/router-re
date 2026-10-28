# Web UI Smoke Test (真实渲染)

- Frontend: http://127.0.0.1:4173
- API: http://127.0.0.1:9090
- Date: 2026-10-28 23:15:55

## / (首页)
- URL: http://127.0.0.1:4173/
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/system/info)
- 结果: PASS

## /system (系统)
- URL: http://127.0.0.1:4173/system
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/system/info)
- 结果: PASS

## /firewall (防火墙)
- URL: http://127.0.0.1:4173/firewall
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/firewall/rules)
- 结果: PASS

## /routing (路由)
- URL: http://127.0.0.1:4173/routing
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/routing/tables)
- 结果: PASS

## /terminals (终端)
- URL: http://127.0.0.1:4173/terminals
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/conntrack/terminals)
- 结果: PASS

## /logs (日志)
- URL: http://127.0.0.1:4173/logs
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/logs)
- 结果: PASS

## /services (服务)
- URL: http://127.0.0.1:4173/services
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/system/services)
- 结果: PASS

## /security-plus (安全Plus)
- URL: http://127.0.0.1:4173/security-plus
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/policy/timeschedule)
- 结果: PASS

## /portal (Portal)
- URL: http://127.0.0.1:4173/portal
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/portal/clients)
- 结果: PASS

## /qos (QoS)
- URL: http://127.0.0.1:4173/qos
- 白屏: 否
- 挂载点: 有
- Bundle 可达: 是
- API 对接: OK (/api/v1/qos/limits)
- 结果: PASS

## 汇总

| 路由 | 页面 | 结果 | 详情 |
|------|------|------|------|
| / | 首页 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /system | 系统 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /firewall | 防火墙 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /routing | 路由 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /terminals | 终端 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /logs | 日志 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /services | 服务 | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /security-plus | 安全Plus | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /portal | Portal | PASS | blank=False, mount=True, api_ok=True, bundle=True |
| /qos | QoS | PASS | blank=False, mount=True, api_ok=True, bundle=True |

- PASS: 10 / FAIL: 0
- 白屏: 无
