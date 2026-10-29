# CHANGELOG 2026-10-29

- p99-p124 全批次落地：IPv6/VLAN/DHCP静态绑定/DNS/静态路由/DDNS/VPN客户端/连接数限制/ARP绑定/MAC黑白名单/网址过滤/流量审计/iPerf/WOL/抓包/SNMP/内网穿透/路由器重启/子网换算/FTP开关/任务队列/双机热备/SMS网关/错误码去重/模糊测试。
- 错误码审计去重：10454/10480/10496 二次出现改码为 10498/10499/10500，conflicts=0。
- 文档：remaining-work 已清零表更新至 p99-p124；对比文档无线 AC/Mesh/信道标为已取消。
- 验证：CGO_ENABLED=1 go build ./... rc=0；go test 全包通过；fuzz 168 用例无 500/panic。
