# Regression Summary

- Generated: 2026-10-28 21:50:20
- Base: http://127.0.0.1:9090
- Total modules: 9
- PASS: 6 / FAIL: 3 / SKIP: 0

## Module Results

| # | Module | Status | Duration |
|---|--------|--------|----------|
| 1 | e2e_test | PASS | — |
| 2 | smoke_new_routes | PASS | 1732ms |
| 3 | roundtrip_test | PASS | 569ms |
| 4 | portal_flow_test | PASS | 574ms |
| 5 | linkcheck_ddns_test | PASS | 3316ms |
| 6 | go_test | PASS | 20334ms |
| 7 | ui_acceptance | FAIL(rc=1) | 6900ms |
| 8 | ui_acceptance_ext | FAIL(rc=1) | 365ms |
| 9 | ui_acceptance_terminals | FAIL(rc=1) | 292ms |

## Details (tail of each log)

### e2e_test — PASS

```
OK     200      0.01s /api/v1/diag/interfaces 
OK     200      0.00s /api/v1/nac/config 
OK     200      0.00s /api/v1/nac/neighbors 
OK     200      0.00s /api/v1/nac/connlimits (空: limits)
OK     200      0.03s /api/v1/nac/top-connections 
OK     200      0.00s /api/v1/lb/config 
OK     200      0.00s /api/v1/lb/status (空: status)
SKIP   422      0.00s /api/v1/lb/preview  (缺前置条件，拒绝正确)
OK     200      0.02s /api/v1/ddns/status 
OK     200      0.00s /api/v1/accounts (空: accounts)
OK     200      0.00s /api/v1/billing/plans 
OK     200      0.00s /api/v1/billing/summary 
OK     200      0.00s /api/v1/users 
OK     200      0.00s /api/v1/ips/rules 
OK     200      0.00s /api/v1/ips/stats 
OK     200      0.00s /api/v1/ips/blocklist 
------------------------------------------------------------------------------
通过 138 / 空数据 19 / 失败 0

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
PASS 2. voucher/generate 测试卡 — HTTP 200 generated=1 code=KVHC2JXZ8F
SKIP 3. portal/login 用卡登录 — 路由未注册 (mgmtd 无 /api/v1/portal/login；门户认证走 /portal/auth 表单，非 mgmtd REST API) probe=GET=404 POST=404 HTTP 404
SKIP 4. portal/status 查会话 — 路由未注册 (mgmtd 无 /api/v1/portal/status) probe=GET=404 POST=404 HTTP 404
PASS 5. voucher list/verify 确认已用 — redeem HTTP 200 ok; list HTTP 200; card KVHC2JXZ8F used=True
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

### go_test — PASS

```
ok  	github.com/jimwong/router-re/mgmtd/internal/radius	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/rbac	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/remote	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/route	(cached)
ok  	github.com/jimwong/router-re/mgmtd/internal/server	14.172s
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

### ui_acceptance — FAIL(rc=1)

```
  FAIL /services          接口  525 条 -> 页面    0 行   systemd 单元列表
  SKIP /online-users      接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /routing-policy    接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /cert              接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /alerts            接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /voucher           接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /ddns              接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /vpn-client        接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /security-plus     接口取数失败: <urlopen error [Errno 111] Connection refused>
  SKIP /network-tools     接口取数失败: 无取数接口
  SKIP /network-advanced  接口取数失败: 无取数接口

❌ 1/1 个页面渲染与接口不一致
   /services: 接口 525 条，页面只有 0 行

常见原因: 前端读了后端没返回的字段名（表格空）、字段名拼错（显示 NaN）、
          el-tabs 之外的选择器作用域不对。
```

### ui_acceptance_ext — FAIL(rc=1)

```
    token = login()
  File "/home/jimwong/projects/router-re/scripts/ui_acceptance_ext.py", line 78, in login
    with urllib.request.urlopen(req, timeout=20) as r:
         ~~~~~~~~~~~~~~~~~~~~~~^^^^^^^^^^^^^^^^^
  File "/usr/lib/python3.13/urllib/request.py", line 189, in urlopen
    return opener.open(url, data, timeout)
           ~~~~~~~~~~~^^^^^^^^^^^^^^^^^^^^
  File "/usr/lib/python3.13/urllib/request.py", line 489, in open
    response = self._open(req, data)
  File "/usr/lib/python3.13/urllib/request.py", line 506, in _open
    result = self._call_chain(self.handle_open, protocol, protocol +
                              '_open', req)
  File "/usr/lib/python3.13/urllib/request.py", line 466, in _call_chain
    result = func(*args)
  File "/usr/lib/python3.13/urllib/request.py", line 1348, in http_open
    return self.do_open(http.client.HTTPConnection, req)
           ~~~~~~~~~~~~^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
  File "/usr/lib/python3.13/urllib/request.py", line 1322, in do_open
    raise URLError(err)
urllib.error.URLError: <urlopen error [Errno 111] Connection refused>
```

### ui_acceptance_terminals — FAIL(rc=1)

```
    token = login()
  File "/home/jimwong/projects/router-re/scripts/ui_acceptance_terminals.py", line 62, in login
    with urllib.request.urlopen(req, timeout=20) as r:
         ~~~~~~~~~~~~~~~~~~~~~~^^^^^^^^^^^^^^^^^
  File "/usr/lib/python3.13/urllib/request.py", line 189, in urlopen
    return opener.open(url, data, timeout)
           ~~~~~~~~~~~^^^^^^^^^^^^^^^^^^^^
  File "/usr/lib/python3.13/urllib/request.py", line 489, in open
    response = self._open(req, data)
  File "/usr/lib/python3.13/urllib/request.py", line 506, in _open
    result = self._call_chain(self.handle_open, protocol, protocol +
                              '_open', req)
  File "/usr/lib/python3.13/urllib/request.py", line 466, in _call_chain
    result = func(*args)
  File "/usr/lib/python3.13/urllib/request.py", line 1348, in http_open
    return self.do_open(http.client.HTTPConnection, req)
           ~~~~~~~~~~~~^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
  File "/usr/lib/python3.13/urllib/request.py", line 1322, in do_open
    raise URLError(err)
urllib.error.URLError: <urlopen error [Errno 111] Connection refused>
```

