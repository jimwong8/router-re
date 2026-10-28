# 新路由冒烟报告 (smoke-new-routes)

> 生成: 2026-10-28 21:10:18  BASE=http://127.0.0.1:9090

**汇总**: 覆盖 28 条 / PASS 28 / FAIL 0

| 路由 | 说明 | 结果 | 详情 |
|---|---|---|---|
| `/api/v1/portal/templates` | 门户模板列表 | PASS | HTTP 200 (预期 {200}) 关键字段OK |
| `/api/v1/portal/preview` | 门户预览(HTML) | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/portal/template` | 门户模板空body→422(Validate挡) | PASS | HTTP 422 (预期 {422}) |
| `/api/v1/firewall/log/analysis` | fwlog分析(可用→验summary/count，不可用→验reason) | PASS | HTTP 200 (预期 {200}) 关键字段OK |
| `/api/v1/firewall/log/suggestions` | fwlog建议(applied=false) | PASS | HTTP 200 (预期 {200}) 关键字段OK |
| `/api/v1/terminals` | 终端聚合列表 | PASS | HTTP 200 (预期 {200}) 关键字段OK |
| `/api/v1/voucher/list` | 卡密列表 | PASS | HTTP 200 (预期 {200}) 关键字段OK |
| `/api/v1/voucher/redeem` | 空卡密核销→422(防枚举统一错误) | PASS | HTTP 422 (预期 {422}) |
| `/api/v1/voucher/generate` | 生成0张→422校验 | PASS | HTTP 422 (预期 {422}) |
| `/api/v1/linkcheck/targets` | 探测目标列表 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/linkcheck/status` | 探测状态 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/linkcheck/targets` | 空targets→422 | PASS | HTTP 422 (预期 {422}) |
| `/api/v1/ddns/records` | DDNS记录 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/ddns/status` | DDNS状态 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/ddns/ip` | DDNS当前IP | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/ddns/check` | DDNS检查 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/vpn/client` | VPN客户端 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/security/arp/neighbors` | ARP邻居 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/security/attack-guard` | 攻击防护配置 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/security/ddos` | DDoS配置 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/policy/timeschedule` | 时间计划 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/accounting/status` | 计费状态 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/accounting/report` | 计费报告 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/conntrack/acct` | conntrack计费开关 | PASS | HTTP 200 (预期 {200}) |
| `/api/v1/wol/wake` | WOL唤醒(已注册即PASS) | PASS | 已注册 |
| `/api/v1/firmware/apply` | 固件升级(已注册即PASS) | PASS | 已注册 |
| `/api/v1/firmware/confirm` | 固件确认(已注册即PASS) | PASS | 已注册 |
| `/api/v1/firmware/plan` | 固件计划(已注册即PASS) | PASS | 已注册 |

