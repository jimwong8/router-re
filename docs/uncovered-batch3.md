# p91-uncovered-batch3: 未覆盖端点测试批3（真实 HTTP）

生成时间: 2026-10-29 01:32:24
目标: mgmtd @ http://127.0.0.1:9090（root 运行，admin 登录取 Bearer）。全部为真实 HTTP 调用。

## 汇总: 8 PASS / 5 FAIL / 0 SKIP（共 13 步）

覆盖端点: stats/current、ips/rules+stats、l7/apply+test+stats、htb/apply+cleanup、
lb/apply+check、dpi/classify-payload、pppoe/multidial+users、vpn 相关2 (keypair/config/client)、
bridge/{name}、quota/test、snmp/query、cloud/heartbeat、threat 四件套 (stats/block/apply/import，fetch 外联 SKIP)。

## 分步结果

### stats/current

- **状态**: PASS
- **详情**: HTTP 200, body keys=['ts', 'cpu_percent', 'mem_used_mb', 'mem_total_mb', 'load1', 'load5', 'load15', 'conntrack_count', 'rx_bytes', 'tx_bytes']

### ips/rules + ips/stats

- **状态**: FAIL
- **详情**: stats HTTP 200 ok=True, POST rules HTTP 422 ok=False, stats再读 HTTP 200

### l7/apply+test+stats

- **状态**: PASS
- **详情**: stats HTTP 200, apply HTTP 200 applied=False, test HTTP 200 blocked=False

### htb/apply+cleanup

- **状态**: PASS
- **详情**: apply dry HTTP 200 applied=False, apply 确认 HTTP 200 applied=True, cleanup dry HTTP 200 applied=False, cleanup 确认 HTTP 200

### lb/apply+check

- **状态**: FAIL
- **详情**: check HTTP 405 checked_at=None, apply HTTP 200 applied=True

### dpi/classify-payload

- **状态**: PASS
- **详情**: HTTP 200, app='http:example.com', category='Web', payload_len=37

### pppoe/multidial+users

- **状态**: FAIL
- **详情**: multidial POST HTTP 200 读回 有 | users POST HTTP 200 读回 有 | 清理 multidial HTTP 422 users HTTP 200

### vpn 相关2

- **状态**: FAIL
- **详情**: keypair HTTP 200 有私钥=True, config dry HTTP 200 applied=None, client GET HTTP 200

### bridge/{name}

- **状态**: PASS
- **详情**: GET HTTP 200, POST 创建 p91test-br HTTP 200 applied=True, DELETE HTTP 200

### quota/test

- **状态**: PASS
- **详情**: HTTP 200, matched=False, scan_count=0

### snmp/query

- **状态**: FAIL
- **详情**: HTTP 400, body keys=['code', 'detail', 'message']

### cloud/heartbeat

- **状态**: PASS
- **详情**: devices GET HTTP 200, register HTTP 200, heartbeat p91test-dev1 HTTP 200 status=ok

### threat 四件套

- **状态**: PASS
- **详情**: stats HTTP 200, block 203.0.113.99 HTTP 200 列表有=有, apply HTTP 200, unblock HTTP 200, import HTTP 200 added=2, fetch SKIP(外联)

## 关键响应片段

- **stats/current**

  ```
  {"ts":1793208723,"cpu_percent":46.012269938650306,"mem_used_mb":31058,"mem_total_mb":63886,"load1":5.37,"load5":5.72,"load15":5.35,"conntrack_count":468,"rx_bytes":146526097583,"tx_bytes":188507053512}

  ```

- **ips/stats**

  ```
  {"stats":{"alerted":0,"dropped":0,"enabled":0,"total":0},"total":0}

  ```

- **ips/rules POST**

  ```
  {"code":"10033","detail":"sid=90001: payload_ascii/payload_hex cannot both be empty","message":"invalid rule"}

  ```

- **ips/stats 写后**

  ```
  {"stats":{"alerted":0,"dropped":0,"enabled":0,"total":0},"total":0}

  ```

- **l7/stats**

  ```
  {"by_action":{},"by_type":{},"enabled":0,"total":0}

  ```

- **l7/apply**

  ```
  {"applied":false,"detail":"配置已写入 /etc/dnsmasq.d/router-re-l7.conf，但 dnsmasq 未重启：systemctl: /usr/bin/systemctl restart dnsmasq: Failed to restart dnsmasq.service: Unit dnsmasq.service not found.；init.d: /etc/init.d/dnsmasq restart: fork/exec /etc/init.d/dnsmasq: no such file or directory","dnsmasq":""}

  ```

- **l7/test**

  ```
  {"blocked":false,"by":"","hit":null}

  ```

- **htb/apply dry-run**

  ```
  {"applied":false,"detail":"dry-run：未下发。要用同一份 config 真正应用，请带 confirm=true。这会重置该接口现有的 qdisc。","dry_run":true,"script":"# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+add 幂等）\ntc qdisc del dev ens18 root 2\u003e/dev/null || true\n# 建立根 HTB qdisc，默认类 1:10\ntc qdisc add dev ens18 root handle 1: htb default 10\n# 根类：总带宽\ntc class add dev ens18 parent 1: classid 1:1 htb rate 100mbit\n"}

  ```

- **htb/apply confirm**

  ```
  {"applied":true,"detail":"applied 0 classes on ens18 (root 100mbit)","dry_run":false,"script":"# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+add 幂等）\ntc qdisc del dev ens18 root 2\u003e/dev/null || true\n# 建立根 HTB qdisc，默认类 1:10\ntc qdisc add dev ens18 root handle 1: htb default 10\n# 根类：总带宽\ntc class add dev ens18 parent 1: classid 1:1 htb rate 100mbit\n"}

  ```

- **htb/cleanup dry-run**

  ```
  {"applied":false,"detail":"dry-run：未清理。要真正删除该接口的根 qdisc，请带 confirm=true。","dry_run":true}

  ```

- **htb/cleanup confirm**

  ```
  {"applied":true,"iface":"ens18"}

  ```

- **lb/check**

  ```
  Method Not Allowed

  ```

- **lb/apply**

  ```
  {"applied":true,"config":{"mode":"","members":null,"health":{"target":"","interval_sec":0,"timeout_sec":0,"fail_threshold":0,"rise_threshold":0}},"detail":"mark table + ip rules applied","iprules":"","marks":{},"rules":"0:\tfrom all lookup local\n4000:\tfrom all to 10.250.0.0/16 lookup main\n5040:\tfrom all fwmark 0x1000/0xff0000 lookup 52\n5045:\tnot from all fwmark 0x80000/0xff0000 lookup main\n
  ```

- **dpi/classify-payload**

  ```
  {"app":"http:example.com","category":"Web","payload_len":37}

  ```

- **pppoe/multidial POST**

  ```
  {"count":1,"detail":"已写入。启动拨号需要对每个会话执行 `pon p91test-d1`（未自动执行）。","path":"/etc/ppp/peers/router-re-sessions.json"}

  ```

- **pppoe/multidial GET**

  ```
  {"path":"/etc/ppp/peers/router-re-sessions.json","sessions":[{"ac":"","enabled":false,"iface":"ens18","lcp_echo":30,"metric":100,"mru":1492,"name":"p91test-d1","password":"***","peer":"plugin rp-pppoe.so\nens18\nuser \"testuser\"\nnoauth\nnoipdefault\ndefaultroute\nreplacedefaultroute\npersist\nmaxfail 0\nmtu 1492\nmru 1492\nlcp-echo-interval 30\nlcp-echo-failure 3\n","service":"","username":"test
  ```

- **pppoe/users POST**

  ```
  {"count":1,"users":[{"name":"p91test-u1","password":"userpass","ip":"10.0.0.50"}]}

  ```

- **pppoe/users GET**

  ```
  {"count":1,"users":[{"name":"p91test-u1","password":"userpass","ip":"10.0.0.50"}]}

  ```

- **vpn/wireguard/keypair**

  ```
  {"private_key":"MNhLrFRsT2u/iu3qJ/qAN1sVuN/bH16jOyUwr7rfSUI=","public_key":"cscoSK0poF+YismH5imMIfYOLSv8tUgRlAefz/XY8W8="}

  ```

- **vpn/wireguard/config dry-run**

  ```
  {"config":"[Interface]\n","message":"wireguard config saved"}

  ```

- **vpn/client GET**

  ```
  {"has_config":false,"hint":"配置存在不代表隧道通；peers 为空说明接口没起来","interface":"wg0","live":false,"live_error":"wg show 不可用（wireguard-tools 未安装，或接口未启动）","peer_count":0,"peers":[]}

  ```

- **bridge GET**

  ```
  {"bridges":[{"name":"lxcbr0","members":[],"address":"10.0.3.1/24","stp":false},{"name":"docker0","members":[],"address":"172.17.0.1/16","stp":false}]}

  ```

- **bridge POST**

  ```
  {"applied":true,"bridges":[{"name":"lxcbr0","members":[],"address":"10.0.3.1/24","stp":false},{"name":"docker0","members":[],"address":"172.17.0.1/16","stp":false},{"name":"p91test-br","members":["ens18"],"stp":false}]}

  ```

- **bridge DELETE**

  ```
  {"deleted":"p91test-br"}

  ```

- **quota/test**

  ```
  {"ctx":{"SrcIP":"10.0.0.1","SrcMAC":"aa:bb:cc:dd:ee:ff","DstIP":"1.2.3.4","DstPort":"443","App":"https"},"hit":null,"matched":false,"scan_count":0}

  ```

- **snmp/query**

  ```
  {"code":"10027","detail":"snmpwalk not installed","message":"snmpwalk not installed"}

  ```

- **cloud/devices GET**

  ```
  {"devices":[]}

  ```

- **cloud/devices POST register**

  ```
  {"device":{"id":"p91test-dev1","name":"p91test","model":"RM200","version":"v1.0.0","last_seen":0,"online":false,"config_version":0},"devices":[{"id":"p91test-dev1","name":"p91test","model":"RM200","version":"v1.0.0","last_seen":0,"online":false,"config_version":0}]}

  ```

- **cloud/heartbeat**

  ```
  {"id":"p91test-dev1","status":"ok"}

  ```

- **threat/stats**

  ```
  {"active":0,"expired":0,"permanent":0,"total":0}

  ```

- **threat/block**

  ```
  {"added":true,"entry":{"ip":"203.0.113.99","reason":"p91test","source":"manual","created_at":1793208744,"expires_at":1793212344,"hits":0}}

  ```

- **threat/list**

  ```
  {"count":1,"entries":[{"ip":"203.0.113.99","reason":"p91test","source":"manual","created_at":1793208744,"expires_at":1793212344,"hits":0}],"expired_count":0,"now":1793208744}

  ```

- **threat/apply**

  ```
  {"applied":true,"count":1,"script":"table inet router_threat {\n  set blocked {\n    type ipv4_addr\n    flags interval,timeout\n    elements = { 203.0.113.99/32 }\n  }\n  set blocked6 {\n    type ipv6_addr\n    flags interval,timeout\n  }\n  chain forward {\n    type filter hook forward priority filter; policy accept;\n    ip saddr @blocked drop\n    ip6 saddr @blocked6 drop\n  }\n}\n","v4_count"
  ```

- **threat/unblock**

  ```
  {"ip":"203.0.113.99","removed":true}

  ```

- **threat/import**

  ```
  {"added":2,"parse_note":"","skipped":0}

  ```
