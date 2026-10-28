# 备份/恢复/导出功能验证报告

> 生成: 2026-10-28 22:24:22  BASE=http://127.0.0.1:9090

**汇总**: 6 项 / PASS 4 / FAIL 2 / SKIP 0

有无备份端点: 是（/api/v1/config/export、/api/v1/config/preview、/api/v1/config/restore）
备份文件: /tmp/scratchpad/router-re-backup.json

| # | 检查项 | 结果 |
|---|---|---|
| 1 | 1. GET /api/v1/config/export | PASS |
| 2 | 2. 备份结构完整性 (JSON合法/非空/含section.option键) | FAIL |
| 3 | 3. 修改配置 (ntp enabled -> False) | PASS |
| 4 | 4. POST /api/v1/config/restore (confirm=true) | PASS |
| 5 | 5. 恢复后重导出对比 | FAIL |
| 6 | 6. POST /api/v1/config/preview | PASS |

## 1. 1. GET /api/v1/config/export — PASS

- 详情: HTTP 200, sections=26, has_keys=True

```
{"sections_count": 26, "sample_keys": ["alerts.alerts/rules", "billing.billing/bills", "ddns.config", "ddns.records", "dns.proxy", "linkcheck.states", "linkcheck.targets", "multiwan.multiwan/members"]}
```

## 2. 2. 备份结构完整性 (JSON合法/非空/含section.option键) — FAIL

- 详情: sections=26, all_keys_have_dot=True, valid_json=False

```
["alerts.alerts/rules", "billing.billing/bills", "ddns.config", "ddns.records", "dns.proxy", "linkcheck.states", "linkcheck.targets", "multiwan.multiwan/members", "multiwan.multiwan/probe", "multiwan.multiwan/states", "ntp.config", "policyroute.policyroute/rules"]
```

## 3. 3. 修改配置 (ntp enabled -> False) — PASS

- 详情: POST=200 verify_GET=200 enabled_now=False

```
{"config": {"servers": null, "enabled": false}}
```

## 4. 4. POST /api/v1/config/restore (confirm=true) — PASS

- 详情: HTTP 200 restored=26

```
{"count": 26, "detail": "已写回 store。注意：部分 section 需要重启对应服务或重新下发才会真正生效。", "restored": ["alerts.alerts/rules", "billing.billing/bills", "ddns.config", "ddns.records", "dns.proxy", "linkcheck.states", "linkcheck.targets", "multiwan.multiwan/members", "multiwan.multiwan/probe", "multiwan.multiwan/states"...
```

## 5. 5. 恢复后重导出对比 — FAIL

- 详情: ntp restored to orig=False (orig=True), sections_equal=False

```
{"orig_enabled": true, "after_enabled": false}
```

## 6. 6. POST /api/v1/config/preview — PASS

- 详情: HTTP 200

```
{"sections": ["alerts.alerts/rules", "billing.billing/bills", "ddns.config", "ddns.records", "dns.proxy", "linkcheck.states", "linkcheck.targets", "multiwan.multiwan/members", "multiwan.multiwan/probe", "multiwan.multiwan/states", "ntp.config", "policyroute.policyroute/rules", "policyroute.policyrou...
```

