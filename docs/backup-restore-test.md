# 备份/恢复/导出功能验证报告

> 生成: 2026-10-28 22:26:28  BASE=http://127.0.0.1:9090

**汇总**: 6 项 / PASS 6 / FAIL 0 / SKIP 0

有无备份端点: 是（/api/v1/config/export、/api/v1/config/preview、/api/v1/config/restore）
备份文件: /tmp/scratchpad/router-re-backup.json

| # | 检查项 | 结果 |
|---|---|---|
| 1 | 1. GET /api/v1/config/export | PASS |
| 2 | 2. 备份结构完整性 (JSON合法/非空/含section.option键) | PASS |
| 3 | 3. 修改配置 (ntp enabled -> True) | PASS |
| 4 | 4. POST /api/v1/config/restore (confirm=true) | PASS |
| 5 | 5. 恢复后重导出对比 | PASS |
| 6 | 6. POST /api/v1/config/preview | PASS |

## 1. 1. GET /api/v1/config/export — PASS

- 详情: HTTP 200, sections=53, has_keys=True

```
{"sections_count": 53, "sample_keys": ["alerts.alerts/rules", "alerts.alerts/rules.alerts.alerts/rules", "billing.billing/bills", "billing.billing/bills.billing.billing/bills", "ddns.config", "ddns.config.ddns.config", "ddns.records", "ddns.records.ddns.records"]}
```

## 2. 2. 备份结构完整性 (JSON合法/非空/含section.option键) — PASS

- 详情: sections=53, all_keys_have_dot=True, valid_json=True

```
["alerts.alerts/rules", "alerts.alerts/rules.alerts.alerts/rules", "billing.billing/bills", "billing.billing/bills.billing.billing/bills", "ddns.config", "ddns.config.ddns.config", "ddns.records", "ddns.records.ddns.records", "dns.proxy", "dns.proxy.dns.proxy", "linkcheck.states", "linkcheck.states....
```

## 3. 3. 修改配置 (ntp enabled -> True) — PASS

- 详情: POST=200 verify_GET=200 enabled_now=True

```
{"config": {"servers": null, "enabled": true}}
```

## 4. 4. POST /api/v1/config/restore (confirm=true) — PASS

- 详情: HTTP 200 restored=53

```
{"count": 53, "detail": "已写回 store。注意：部分 section 需要重启对应服务或重新下发才会真正生效。", "restored": ["alerts.alerts/rules", "alerts.alerts/rules.alerts.alerts/rules", "billing.billing/bills", "billing.billing/bills.billing.billing/bills", "ddns.config", "ddns.config.ddns.config", "ddns.records", "ddns.records.ddns....
```

## 5. 5. 恢复后重导出对比 — PASS

- 详情: ntp restored to orig=True (orig=False), sections_equal=True

```
{"orig_enabled": false, "after_enabled": false}
```

## 6. 6. POST /api/v1/config/preview — PASS

- 详情: HTTP 200

```
{"sections": ["alerts.alerts/rules", "alerts.alerts/rules.alerts.alerts/rules", "billing.billing/bills", "billing.billing/bills.billing.billing/bills", "ddns.config", "ddns.config.ddns.config", "ddns.records", "ddns.records.ddns.records", "dns.proxy", "dns.proxy.dns.proxy", "linkcheck.states", "link...
```

