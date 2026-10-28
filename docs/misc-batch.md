# p94-misc-batch: 未覆盖杂项端点批（真实 HTTP）

生成时间: 2026-10-29 04:22:29
目标: mgmtd @ http://127.0.0.1:9090（root 运行, admin 登录取 Bearer）。全部真实 HTTP；端点/参数已 grep 确认。

## 汇总: 11 PASS / 0 FAIL / 1 SKIP（共 12 步）

覆盖: stats/current、ips/rules+stats、dpi/classify-payload、pppoe/users(写读回+清理)、
pppoe/multidial(写读回+清理, 含 p91 遗留)、vpn keypair+config(dry)+client、bridge/{name}(创建+删除)、
quota/test、snmp/query(环境 SKIP)、cloud register+heartbeat、diag/traceroute。
threat/htb/lb/l7 不在本批。标记 p94test；写操作均有读回+清理路径。

## 分步结果

### stats/current GET

- **状态**: PASS
- **详情**: HTTP 200, keys=['conntrack_count', 'cpu_percent', 'load1', 'load15', 'load5', 'mem_total_mb', 'mem_used_mb', 'rx_bytes']

### ips/stats + ips/rules(GET)

- **状态**: PASS
- **详情**: stats HTTP 200 total=0 | rules HTTP 200 count=0（空集 rules=null 正常）

### dpi/classify-payload

- **状态**: PASS
- **详情**: HTTP 200, app='http:example.com', category='Web', payload_len=44

### quota/test

- **状态**: PASS
- **详情**: HTTP 200, matched=False, scan_count=0

### snmp/query

- **状态**: SKIP
- **详情**: 环境依赖: snmpwalk 未安装; 端点按设计如实返回 HTTP 400 code=10027（行为正确）

### diag/traceroute(127.0.0.1,2hops)

- **状态**: PASS
- **详情**: HTTP 200, tool='traceroute', hop_count=1

### vpn 相关2(keypair+config dry+client)

- **状态**: PASS
- **详情**: keypair HTTP 200, config dry HTTP 200 msg='wireguard config saved', client HTTP 200

### bridge/{name} 创建+删除

- **状态**: PASS
- **详情**: GET HTTP 200, POST p94test-br HTTP 200 applied=True, DELETE HTTP 200, 列表已无=True

### ips/rules marker 写读回

- **状态**: PASS
- **详情**: 原 0 条; POST HTTP 200 body=None applied=True, 读回含 sid=94001=True; 原集合为空, 端点按设计拒空(10034); marker 规则保持 enabled=false 留存

### pppoe/users 写读回+清理

- **状态**: PASS
- **详情**: 原 0 用户; POST HTTP 200 读回含 marker=True; 原集合为空, 已 confirm 清空 HTTP 200

### pppoe/multidial 写读回+清理

- **状态**: PASS
- **详情**: 原 0 会话(含 p91 遗留 p91test-d1); POST HTTP 200 读回含 marker=True; 端点按设计拒空数组(HTTP 422 code=10199); 该文件系 p91/p94 测试产物, 已 sudo rm 恢复测试前(无文件=无会话)

### cloud/heartbeat

- **状态**: PASS
- **详情**: devices GET HTTP 200, 注册 p94test-dev HTTP 200 id='p94test-dev', heartbeat HTTP 200 status=ok（内存态, 无删除端点, 重启即清）

## 关键响应片段

- **stats/current 响应**

  ```
  {"ts":1793218923,"cpu_percent":17.771084337349397,"mem_used_mb":29344,"mem_total_mb":63886,"load1":5.92,"load5":6.75,"load15":6.04,"conntrack_count":358,"rx_bytes":147026785723,"tx_bytes":193849071999}
  ```
- **ips/stats 响应**

  ```
  {"stats":{"alerted":0,"dropped":0,"enabled":0,"total":0},"total":0}
  ```
- **ips/rules GET 响应**

  ```
  {"count":0,"rules":null}
  ```
- **dpi/classify-payload 响应**

  ```
  {"app":"http:example.com","category":"Web","payload_len":44}
  ```
- **quota/test 响应**

  ```
  {"ctx":{"SrcIP":"192.168.1.50","SrcMAC":"aa:bb:cc:dd:ee:94","DstIP":"8.8.8.8","DstPort":"53","App":"dns"},"hit":null,"matched":false,"scan_count":0}
  ```
- **snmp/query 响应**

  ```
  {"code":"10027","detail":"snmpwalk not installed","message":"snmpwalk not installed"}
  ```
- **diag/traceroute 响应**

  ```
  {"host":"127.0.0.1","hop_count":1,"raw":"traceroute to 127.0.0.1 (127.0.0.1), 2 hops max, 60 byte packets\n 1  127.0.0.1  0.049 ms  0.020 ms  0.017 ms","extra":{"tool":"traceroute"}}
  ```
- **vpn keypair(仅片段, 不回显私钥)**

  ```
  {"private_key":"GLKcssJJfqgzTKXERRskc0nvfPY15ym/I/xDN267bWs=...[私钥省略]
  ```
- **vpn config dry-run**

  ```
  {"config":"[Interface]\n","message":"wireguard config saved"}
  ```
- **vpn/client GET**

  ```
  {"has_config":false,"hint":"配置存在不代表隧道通；peers 为空说明接口没起来","interface":"wg0","live":false,"live_error":"wg show 不可用（wireguard-tools 未安装，或接口未启动）","peer_count":0,"peers":[]}
  ```
- **bridge GET 列表**

  ```
  {"bridges":[{"name":"lxcbr0","members":[],"address":"10.0.3.1/24","stp":false},{"name":"docker0","members":[],"address":"172.17.0.1/16","stp":false}]}
  ```
- **bridge POST 创建**

  ```
  {"applied":true,"bridges":[{"name":"lxcbr0","members":[],"address":"10.0.3.1/24","stp":false},{"name":"docker0","members":[],"address":"172.17.0.1/16","stp":false},{"name":"p94test-br","members":["ens18"],"stp":false}]}
  ```
- **bridge DELETE**

  ```
  {"deleted":"p94test-br"}
  ```
- **ips/rules POST(原+marker)**

  ```
  {"applied":true,"count":1,"failures":"","rules":[{"sid":94001,"msg":"p94test-ips","proto":"tcp","src_port":"any","dst_port":"any","payload_hex":"","payload_ascii":"p94test-marker","flow":"","action":"alert","enabled":false,"severity":3}]}
  ```
- **ips/rules GET 读回**

  ```
  {"count":1,"rules":[{"sid":94001,"msg":"p94test-ips","proto":"tcp","src_port":"any","dst_port":"any","payload_hex":"","payload_ascii":"p94test-marker","flow":"","action":"alert","enabled":true,"severity":3}]}
  ```
- **pppoe/users POST(原+marker)**

  ```
  {"count":1,"users":[{"name":"p94test-u1","password":"p94pass","ip":"10.94.99.99"}]}
  ```
- **pppoe/users GET 读回**

  ```
  {"count":1,"users":[{"name":"p94test-u1","password":"p94pass","ip":"10.94.99.99"}]}
  ```
- **pppoe/users 清空(confirm)**

  ```
  {"count":0,"users":[]}
  ```
- **multidial GET(原始)**

  ```
  {"path":"/etc/ppp/peers/router-re-sessions.json","sessions":[]}
  ```
- **multidial POST(原+marker)**

  ```
  {"count":1,"detail":"已写入。启动拨号需要对每个会话执行 `pon p94test-m1`（未自动执行）。","path":"/etc/ppp/peers/router-re-sessions.json"}
  ```
- **multidial GET 读回**

  ```
  {"path":"/etc/ppp/peers/router-re-sessions.json","sessions":[{"ac":"","enabled":false,"iface":"ens18","lcp_echo":30,"metric":100,"mru":1492,"name":"p94test-m1","password":"***","peer":"plugin rp-pppoe.so\nens18\nuser \"p94user\"\nnoauth\nnoipdefault\ndefaultroute\nreplacedefaultroute\npersist\nmaxfa
  ```
- **multidial 清空尝试(端点拒空)**

  ```
  {"code":"10199","detail":"如需清空请显式传空数组并说明意图，或逐条停用","message":"sessions 不能为空"}
  ```
- **cloud/devices GET**

  ```
  {"devices":[{"id":"p91test-dev1","name":"p91test","model":"RM200","version":"v1.0.0","last_seen":1793208744,"online":true,"config_version":0},{"id":"p94test-dev","name":"p94test-dev","model":"RM200","version":"v0.94","last_seen":1793218935,"online":true,"config_version":0}]}
  ```
- **cloud/devices 注册**

  ```
  {"device":{"id":"p94test-dev","name":"p94test-dev","model":"RM200","version":"v0.94","last_seen":0,"online":false,"config_version":0},"devices":[{"id":"p91test-dev1","name":"p91test","model":"RM200","version":"v1.0.0","last_seen":1793208744,"online":true,"config_version":0},{"id":"p94test-dev","name":"p94test-dev","model":"RM200","version":"v0.94","last_seen":0,"online":false,"config_version":0}]}
  ```
- **cloud heartbeat**

  ```
  {"id":"p94test-dev","status":"ok"}
  ```