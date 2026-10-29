# E2E Security Report

- Base: http://127.0.0.1:9090
- 通过: 17 / 失败: 0 / 跳过: 3

| Method | Path | Code | Result | 说明 |
|---|---|---|---|---|
| GET | /api/v1/nac/config | 200 | PASS | NAC 全局配置 |
| GET | /api/v1/nac/connlimits | 200 | PASS | 连接数限制 |
| POST | /api/v1/nac/connlimits/apply | 200 | PASS | 下发连接限制 |
| GET | /api/v1/nac/top-connections | 200 | PASS | Top 连接 |
| GET | /api/v1/acl/mac-filter | 200 | PASS | MAC 过滤 |
| GET | /api/v1/quota/policies | 200 | PASS | 配额策略 |
| GET | /api/v1/quota/report | 200 | PASS | 配额用量 |
| POST | /api/v1/quota/apply | 409 | PASS | 破坏性下发必须要求 confirm |
| GET | /api/v1/quota/nft | 200 | PASS | 配额 nft 表 |
| GET | /api/v1/security/attack-guard | 200 | PASS | 攻击防护 |
| GET | /api/v1/security/ddos | 200 | PASS | DDoS 防护 |
| GET | /api/v1/alg | 200 | PASS | ALG |
| GET | /api/v1/ips/blocklist | 200 | PASS | IPS 黑名单 |
| POST | /api/v1/ips/test | 200 | PASS | IPS 测试命中 |
| GET | /api/v1/webfilter/rules | 200 | PASS | 域名过滤规则 |
| POST | /api/v1/webfilter/apply | 403 | PASS | 下发过滤（403=目录不可写） |
| GET | /api/v1/webfilter/apply-status | 200 | PASS | 下发状态 |
| GET | /api/v1/security/neighbors | SKIP | SKIP | hardware-dependent |
| GET | /api/v1/security/arp | SKIP | SKIP | hardware-dependent |
| GET | /api/v1/nac/neighbors | SKIP | SKIP | hardware-dependent |
