# Regression Summary

- Generated: 2026-10-29 01:53:17
- Base: http://127.0.0.1:9090
- Commit: 203487a (+未提交改动)
- 未提交改动: 11 个文件

  > 注意：以下文件在本次回归运行时处于未提交状态，相关模块的结果只代表当时的工作区快照：
  >  M docs/linkcheck-ddns-test.md
  >  M docs/portal-flow-test.md
  >  M docs/regression-summary.md
  >  M docs/roundtrip-test.md
  >  M docs/smoke-new-routes.md
  >  M docs/ui-coverage-extended.md
  >  M docs/ui-terminals-acceptance.md
  >  M ops/lane-w1-pN.json
  >  M ops/lane-w1-pP.json
  >  M ops/lane-w1-pQ.json
- Total modules: 15
- PASS: 15 / FAIL: 0 / SKIP: 0 / UNSTABLE: 0

## Module Results

| # | Module | Status | Duration |
|---|--------|--------|----------|
| 1 | e2e_test | PASS | — |
| 2 | smoke_new_routes | PASS | 1588ms |
| 3 | roundtrip_test | PASS | 559ms |
| 4 | portal_flow_test | PASS | 491ms |
| 5 | linkcheck_ddns_test | PASS | 3305ms |
| 6 | rbac_e2e | PASS | 2214ms |
| 7 | go_test | PASS | 656ms |
| 8 | vue_typecheck | PASS | 16484ms |
| 9 | ui_acceptance | PASS | 32458ms |
| 10 | ui_acceptance_ext | PASS | 91843ms |
| 11 | ui_acceptance_terminals | PASS | 10085ms |
| 12 | users_edit_e2e | PASS | 10179ms |
| 13 | dialog_smoke | PASS | 36222ms |
| 14 | form_submit_e2e | PASS | 23423ms |
| 15 | rbac_ui_e2e | PASS | 25031ms |

## Details (tail of each log)

### e2e_test — PASS

```
OK     200      0.01s /api/v1/diag/interfaces 
OK     200      0.00s /api/v1/nac/config 
OK     200      0.01s /api/v1/nac/neighbors 
OK     200      0.00s /api/v1/nac/connlimits (空: limits)
OK     200      0.03s /api/v1/nac/top-connections 
OK     200      0.00s /api/v1/lb/config 
OK     200      0.00s /api/v1/lb/status (空: status)
SKIP   422      0.00s /api/v1/lb/preview  (缺前置条件，拒绝正确)
OK     200      0.02s /api/v1/ddns/status 
OK     200      0.00s /api/v1/accounts 
OK     200      0.00s /api/v1/billing/plans 
OK     200      0.00s /api/v1/billing/summary 
OK     200      0.00s /api/v1/users 
OK     200      0.00s /api/v1/ips/rules 
OK     200      0.00s /api/v1/ips/stats 
OK     200      0.00s /api/v1/ips/blocklist 
------------------------------------------------------------------------------
通过 139 / 空数据 19 / 失败 0

E2E: PASS
```

### smoke_new_routes — PASS

```
PASS HTTP 422 (预期 {422})                      /api/v1/linkcheck/targets — 空targets→422
PASS HTTP 200 (预期 {200})                      /api/v1/ddns/records — DDNS记录
PASS HTTP 200 (预期 {200})                      /api/v1/ddns/status — DDNS状态
PASS HTTP 200 (预期 {200})                      /api/v1/ddns/ip — DDNS当前IP
PASS HTTP 200 (预期 {200})                      /api/v1/ddns/check — DDNS检查
PASS HTTP 200 (预期 {200})                      /api/v1/vpn/client — VPN客户端
PASS HTTP 200 (预期 {200})                      /api/v1/security/arp/neighbors — ARP邻居
PASS HTTP 200 (预期 {200})                      /api/v1/security/attack-guard — 攻击防护配置
PASS HTTP 200 (预期 {200})                      /api/v1/security/ddos — DDoS配置
PASS HTTP 200 (预期 {200})                      /api/v1/policy/timeschedule — 时间计划
PASS HTTP 200 (预期 {200})                      /api/v1/accounting/status — 计费状态
PASS HTTP 200 (预期 {200})                      /api/v1/accounting/report — 计费报告
PASS HTTP 200 (预期 {200})                      /api/v1/conntrack/acct — conntrack计费开关
PASS 已注册                                      /api/v1/wol/wake — WOL唤醒(已注册即PASS)
PASS 已注册                                      /api/v1/firmware/apply — 固件升级(已注册即PASS)
PASS 已注册                                      /api/v1/firmware/confirm — 固件确认(已注册即PASS)
PASS 已注册                                      /api/v1/firmware/plan — 固件计划(已注册即PASS)

汇总: 28 条路由 / PASS 28 / FAIL 0
报告: /home/jimwong/projects/router-re/docs/smoke-new-routes.md
```

### roundtrip_test — PASS

```
PASS voucher/generate→list: write=200 read=200 verified=True
PASS portal/template→templates: write=200 read=200 verified=True
PASS linkcheck/targets→targets: write=200 read=200 verified=True
PASS ddns/records→records: write=200 read=200 verified=True
PASS policy/timeschedule→timeschedule: write=200 read=200 verified=True
PASS voucher/generate→list: write=200 read=200 verified=True
PASS portal/template→templates: write=200 read=200 verified=True
PASS linkcheck/targets→targets: write=200 read=200 verified=True
PASS ddns/records→records: write=200 read=200 verified=True
PASS policy/timeschedule→timeschedule: write=200 read=200 verified=True

汇总: 5 个写端点 / PASS 5 / FAIL 0
报告: /home/jimwong/projects/router-re/docs/roundtrip-test.md
```

### portal_flow_test — PASS

```
PASS 1. 选/保存 portal 模板 — GET templates 200 (count=4); POST template 200 saved=true id=roundtrip
PASS 2. voucher/generate 测试卡 — HTTP 200 generated=1 code=63CG2WXV7V
SKIP 3. portal/login 用卡登录 — 路由未注册 (mgmtd 无 /api/v1/portal/login；门户认证走 /portal/auth 表单，非 mgmtd REST API) probe=GET=404 POST=404 HTTP 404
SKIP 4. portal/status 查会话 — 路由未注册 (mgmtd 无 /api/v1/portal/status) probe=GET=404 POST=404 HTTP 404
PASS 5. voucher list/verify 确认已用 — redeem HTTP 200 ok; list HTTP 200; card 63CG2WXV7V used=True
SKIP 6. portal/logout 登出+status消失 — 路由未注册 (mgmtd 无 /api/v1/portal/logout) probe=GET=404 POST=404 HTTP 404

汇总: 6 步 / PASS 3 / FAIL 0 / SKIP 3
报告: /home/jimwong/projects/router-re/docs/portal-flow-test.md
```

### linkcheck_ddns_test — PASS

```
PASS 1. linkcheck 建 target (POST targets): write=200 read=200 found=True
PASS 2. linkcheck probe + status (states 含该 target): probe=200 status=200 state_present=True
PASS 3. ddns 建 record (POST records): write=200 read=200 found=True
PASS 4. ddns check + update 结构 (非 500 崩): check=200 update=400
PASS 5. 清理 (DELETE 端点): target 已删=是 / record 已删=是
报告已写 /home/jimwong/projects/router-re/docs/linkcheck-ddns-test.md
SUMMARY: PASS=5 FAIL=0 SKIP=0
```

### rbac_e2e — PASS

```
OK   viewer 读系统信息: 期望 200 实际 200
OK   viewer 改防火墙被拒: 期望 403 实际 403
OK   viewer 恢复配置被拒: 期望 403 实际 403
OK   viewer 读用户列表被拒: 期望 403 实际 403
OK   operator 改防火墙放行: 期望 200 实际 200
OK   operator 管用户被拒: 期望 403 实际 403
OK   admin 读用户列表: 期望 200 实际 200
OK   清理测试账号: 期望 200 实际 200
报告: /home/jimwong/projects/router-re/docs/rbac-e2e.md

PASS 8/8
```

### go_test — PASS

```
ok  	github.com/jimwong/router-re/mgmtd/internal/radius	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/rbac	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/remote	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/route	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/server	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/snmp	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/stats	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/sysctl	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/system	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/threat	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/timesched	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/traffic	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/upnp	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/voucher	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/vpn	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/vpnclient	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/wan	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/webadmin	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/webfilter	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/wireless	(cached)
```

### vue_typecheck — PASS

```
```

### ui_acceptance — PASS

```
  OK   /services          接口  527 条 -> 页面   50 行   systemd 单元列表 [分页]
  OK   /online-users      接口   22 条 -> 页面   23 行   在线会话
  OK   /routing-policy    接口    1 条 -> 页面    2 行   策略路由规则(可为空)
  SKIP /cert              接口取数失败: 没有可识别的列表字段: detail,exists,path
  OK   /alerts            接口    1 条 -> 页面    1 行   告警规则(可为空)
  OK   /voucher           接口   52 条 -> 页面   52 行   卡密列表(可能为空)
  OK   /ddns              接口    0 条 -> 页面    0 行   DDNS 记录(可为空)
  OK   /vpn-client        接口    0 条 -> 页面    0 行   VPN 客户端 peers(可为空)
  OK   /security-plus     接口    1 条 -> 页面    2 行   时间段策略 rules(可为空)
  SKIP /network-tools     接口取数失败: 无取数接口
  SKIP /network-advanced  接口取数失败: 无取数接口

✅ UI 验收通过（8 个页面，渲染行数与接口一致）
```

### ui_acceptance_ext — PASS

```
  OK   /ipv6              接口    5 条 -> 页面    9 行   IPv6 地址(可为空)
  OK   /l7                接口    0 条 -> 页面    0 行   L7 规则(可为空)
  OK   /logs              接口  100 条 -> 页面  100 行   日志条目(可为空)
  OK   /voucher           接口   52 条 -> 页面   52 行   卡密列表(可为空)
  SKIP /nac               SKIP: NAC 多表格分页 (devices/limits/top/neighbors)，接口 neighbors=45 非可见表，无法准确比对
  SKIP /network           SKIP: portmap/rules.rules=None(主机无 NAT 规则)
  OK   /pppoe             接口    0 条 -> 页面    0 行   PPPoE 会话(可为空)
  OK   /qos               接口    0 条 -> 页面    0 行   QoS 限速(可为空)
  SKIP /remote            接口取数失败: 没有可识别的列表字段: access,warnings
  OK   /security          接口   11 条 -> 页面   37 行   接口流量(ACL 表格取 acl/rules、主体表格取 stats/interfaces)
  OK   /threat            接口    2 条 -> 页面    2 行   威胁列表(可为空)
  OK   /upnp              接口    0 条 -> 页面    1 行   UPnP 映射(可为空)
  OK   /users             接口    3 条 -> 页面    3 行   用户列表
  OK   /wireless          接口    0 条 -> 页面    0 行   Wi-Fi SSID(可为空)
  SKIP /static-route      接口取数失败: 没有可识别的列表字段: commands,routes
  OK   /wan               接口    1 条 -> 页面   10 行   SKIP: wan/links links=None(主机无多WAN)
  SKIP /dhcp-server       接口取数失败: 没有可识别的列表字段: applied,config

✅ UI 验收扩展通过（24 个页面 OK，9 个 SKIP，0 FAIL）
报告已写 docs/ui-coverage-extended.md
```

### ui_acceptance_terminals — PASS

```
  OK   /terminals       接口   20 条 -> 页面   20 行   终端列表
  OK   /online-users    接口   26 条 -> 页面   27 行   字节统计块=存在

✅ terminals 验收通过（2 个页面 OK，0 个 FAIL）
报告已写 docs/ui-terminals-acceptance.md
```

### users_edit_e2e — PASS

```
OK   列表里能看到目标账号
OK   行内有编辑按钮
OK   弹窗里有角色下拉和启用开关 — select=True switch=True
OK   保存后后端角色真的变了 — role=operator
OK   表格已刷新显示新角色

PASS 5/5
```

### dialog_smoke — PASS

```
OK   /users 新建用户 弹出对话框 — 可见 overlay=1
OK   /voucher 批量生成 弹出对话框 — 可见 overlay=1
OK   /routing-policy 新增 弹出对话框 — 可见 overlay=1
OK   /alerts 新增 弹出对话框 — 可见 overlay=1
OK   /cert 生成自签证书 弹出对话框 — 可见 overlay=1
OK   /security-plus 新增 弹出对话框 — 可见 overlay=1
OK   /linkcheck 新增目标 弹出对话框 — 可见 overlay=1

PASS 7/7
```

### form_submit_e2e — PASS

```
OK   告警规则保存后后端有这条 — rules=2
OK   告警页表格出现新规则
OK   批量生成后卡密多了 2 条 — 52 → 54
OK   生成的卡密已清理 — 52 条（基线 52）
OK   链路目标保存后后端有这条 — targets=1
OK   链路检测页显示新目标

PASS 6/6
```

### rbac_ui_e2e — PASS

```
OK   viewer 保存 ACL 被拒并给出提示 — 提示='当前角色无权执行该操作'
OK   错误文案是中文说明而不是 forbidden — 提示='当前角色无权执行该操作'
OK   viewer 保存 ACL 没有误报成功 — 成功提示=''
OK   admin 保存 ACL 没有被权限拒绝 — 错误提示='nft apply failed: /usr/sbin/nft -f -: /d'
OK   viewer 建用户被拒且有中文说明 — 提示='当前角色无权执行该操作'
OK   viewer 建用户没有误报成功 — 成功提示=''

PASS 6/6
```

