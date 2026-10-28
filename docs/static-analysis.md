# Static Analysis Report

- Date: 2026-10-28
- Module: mgmtd/

## 1. go vet ./...

- 结果: **PASS** (0 警告)
- 退出码: 0

## 2. staticcheck ./...

- 结果: **25 条问题** (RC=1)
- 其中测试文件 10 条，生产代码 15 条

### 问题列表

| # | 文件:行 | 规则 | 说明 | 严重性 |
|---|---------|------|------|--------|
| 1 | bond/bond_test.go:39:27 | U1000 | func hasCall is unused | style |
| 2 | ips/rule.go:256:25 | SA6005 | should use !strings.EqualFold | warning |
| 3 | ipv6/nd.go:95:6 | U1000 | func ipv6Bool is unused | style |
| 4 | logcenter/logcenter_test.go:66:3 | SA4006 | entries never used | warning |
| 5 | logcenter/logcenter_test.go:123:3 | SA4006 | units never used | warning |
| 6 | online/session.go:60:2 | S1008 | simplify if/return | style |
| 7 | portal/apply.go:32:17 | S1025 | fmt.Sprintf on string | style |
| 8 | quota/apply_test.go:12:2 | U1000 | field runSeq unused | style |
| 9 | quota/apply_test.go:90:6 | U1000 | func renderWithOneDrop unused | style |
| 10 | quota/render_test.go:75:46 | SA4017 | HasPrefix return ignored | warning |
| 11 | rbac/rbac_test.go:62:6 | U1000 | func sha256Hash unused | style |
| 12 | server/ddns.go:13:7 | U1000 | const ddnsConfigKey unused | style |
| 13 | server/ddns.go:14:7 | U1000 | const ntpConfigKey unused | style |
| 14 | server/dns.go:11:7 | U1000 | const dnsConfigKey unused | style |
| 15 | server/l7.go:14:7 | U1000 | const l7RulesKey unused | style |
| 16 | server/p10.go:240:5 | U1000 | var sectionKeys unused | style |
| 17 | server/p11.go:118:13 | SA4010 | append result never used | warning |
| 18 | server/p15.go:495:6 | U1000 | func netSplitHostPort unused | style |
| 19 | server/p32_handler_test.go:134:2 | SA4006 | okArr never used | warning |
| 20 | server/server.go:1055:18 | U1000 | func dpiRulesFromStore unused | style |
| 21 | server/server.go:1076:6 | U1000 | func atoi unused | style |
| 22 | server/server.go:2315:2 | SA4006 | total never used | warning |
| 23 | server/upnp.go:11:7 | U1000 | const upnpConfigKey unused | style |
| 24 | server/upnp.go:12:7 | U1000 | const upnpMappingsKey unused | style |
| 25 | vpnclient/client_test.go:29:2 | U1000 | var pskKey unused | style |
| 26 | webadmin/listen_test.go:29:27 | U1000 | func hasCall unused | style |

## 3. 汇总

- go vet: 0 警告
- staticcheck: 25 条（warning 7 条，style 18 条，error 0 条）
- 需修: 0 条 error，7 条 warning 建议修，18 条 style 可选