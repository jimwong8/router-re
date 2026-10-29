# API 输入模糊测试报告

> 生成: 2026-10-29 20:55:34

- 端点数: 21, 畸形类型: 8, 总组数: 168
- 安全 (4xx 无栈无XSS): 149
- 接受 (2xx，无严格校验): 12
- 危险 (500/栈/XSS/连接失败): 3
- HTTP 500: 0, HTTP 4xx: 149, HTTP 2xx: 12, 连接失败: 3
- Go 栈泄露: 0, XSS 反射: 0

| # | 端点 | 畸形类型 | 状态 | 安全 | 栈泄露 | XSS | 片段 |
|---|------|----------|------|------|--------|-----|------|
| 1 | /api/v1/portal/authorize | empty_bytes | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 2 | /api/v1/portal/authorize | spaces | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 3 | /api/v1/portal/authorize | huge_1mb | 400 | Y | - | - | {"code":"10048","detail":"invalid MAC address","message":"invalid MAC"}  |
| 4 | /api/v1/portal/authorize | deep_nest | 400 | Y | - | - | {"code":"10007","detail":"invalid character ':' after object key:value pair","me |
| 5 | /api/v1/portal/authorize | num_as_str | 400 | Y | - | - | {"code":"10048","detail":"invalid MAC address","message":"invalid MAC"}  |
| 6 | /api/v1/portal/authorize | unicode_emoji | 400 | Y | - | - | {"code":"10048","detail":"invalid MAC address","message":"invalid MAC"}  |
| 7 | /api/v1/portal/authorize | sql_injection | 400 | Y | - | - | {"code":"10048","detail":"invalid MAC address","message":"invalid MAC"}  |
| 8 | /api/v1/portal/authorize | control_chars | 400 | Y | - | - | {"code":"10007","detail":"invalid character '\\x00' in string literal","message" |
| 9 | /api/v1/voucher/generate | empty_bytes | 400 | Y | - | - | {"code":"10430","detail":"EOF","message":"invalid JSON body"}  |
| 10 | /api/v1/voucher/generate | spaces | 400 | Y | - | - | {"code":"10430","detail":"EOF","message":"invalid JSON body"}  |
| 11 | /api/v1/voucher/generate | huge_1mb | 422 | Y | - | - | {"code":"10431","detail":"一次最多生成 500 张，避免返回体过大和误操作","message":"count must be 1-5 |
| 12 | /api/v1/voucher/generate | deep_nest | 400 | Y | - | - | {"code":"10430","detail":"invalid character ':' after object key:value pair","me |
| 13 | /api/v1/voucher/generate | num_as_str | 400 | Y | - | - | {"code":"10430","detail":"json: cannot unmarshal string into Go struct field .co |
| 14 | /api/v1/voucher/generate | unicode_emoji | 422 | Y | - | - | {"code":"10431","detail":"一次最多生成 500 张，避免返回体过大和误操作","message":"count must be 1-5 |
| 15 | /api/v1/voucher/generate | sql_injection | 422 | Y | - | - | {"code":"10431","detail":"一次最多生成 500 张，避免返回体过大和误操作","message":"count must be 1-5 |
| 16 | /api/v1/voucher/generate | control_chars | 400 | Y | - | - | {"code":"10430","detail":"invalid character '\\x00' in string literal","message" |
| 17 | /api/v1/ddns/records | empty_bytes | 400 | Y | - | - | {"code":"10422","detail":"EOF","message":"invalid JSON body"}  |
| 18 | /api/v1/ddns/records | spaces | 400 | Y | - | - | {"code":"10422","detail":"EOF","message":"invalid JSON body"}  |
| 19 | /api/v1/ddns/records | huge_1mb | 422 | Y | - | - | {"code":"10425","detail":"to clear all ddns records, send {records:[],confirm:tr |
| 20 | /api/v1/ddns/records | deep_nest | 400 | Y | - | - | {"code":"10422","detail":"invalid character ':' after object key:value pair","me |
| 21 | /api/v1/ddns/records | num_as_str | 422 | Y | - | - | {"code":"10425","detail":"to clear all ddns records, send {records:[],confirm:tr |
| 22 | /api/v1/ddns/records | unicode_emoji | 422 | Y | - | - | {"code":"10425","detail":"to clear all ddns records, send {records:[],confirm:tr |
| 23 | /api/v1/ddns/records | sql_injection | 422 | Y | - | - | {"code":"10425","detail":"to clear all ddns records, send {records:[],confirm:tr |
| 24 | /api/v1/ddns/records | control_chars | 400 | Y | - | - | {"code":"10422","detail":"invalid character '\\x00' in string literal","message" |
| 25 | /api/v1/linkcheck/targets | empty_bytes | 400 | Y | - | - | {"code":"10452","detail":"EOF","message":"invalid JSON body"}  |
| 26 | /api/v1/linkcheck/targets | spaces | 400 | Y | - | - | {"code":"10452","detail":"EOF","message":"invalid JSON body"}  |
| 27 | /api/v1/linkcheck/targets | huge_1mb | 422 | Y | - | - | {"code":"10450","detail":"要停用请把 enabled 设为 false，而不是清空列表","message":"targets mus |
| 28 | /api/v1/linkcheck/targets | deep_nest | 400 | Y | - | - | {"code":"10452","detail":"invalid character ':' after object key:value pair","me |
| 29 | /api/v1/linkcheck/targets | num_as_str | 422 | Y | - | - | {"code":"10450","detail":"要停用请把 enabled 设为 false，而不是清空列表","message":"targets mus |
| 30 | /api/v1/linkcheck/targets | unicode_emoji | 422 | Y | - | - | {"code":"10450","detail":"要停用请把 enabled 设为 false，而不是清空列表","message":"targets mus |
| 31 | /api/v1/linkcheck/targets | sql_injection | 422 | Y | - | - | {"code":"10450","detail":"要停用请把 enabled 设为 false，而不是清空列表","message":"targets mus |
| 32 | /api/v1/linkcheck/targets | control_chars | 400 | Y | - | - | {"code":"10452","detail":"invalid character '\\x00' in string literal","message" |
| 33 | /api/v1/qos/limits | empty_bytes | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 34 | /api/v1/qos/limits | spaces | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 35 | /api/v1/qos/limits | huge_1mb | 200 | N | - | - | {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}  |
| 36 | /api/v1/qos/limits | deep_nest | 400 | Y | - | - | {"code":"10007","detail":"invalid character ':' after object key:value pair","me |
| 37 | /api/v1/qos/limits | num_as_str | 200 | N | - | - | {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}  |
| 38 | /api/v1/qos/limits | unicode_emoji | 200 | N | - | - | {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}  |
| 39 | /api/v1/qos/limits | sql_injection | 200 | N | - | - | {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}  |
| 40 | /api/v1/qos/limits | control_chars | 400 | Y | - | - | {"code":"10007","detail":"invalid character '\\x00' in string literal","message" |
| 41 | /api/v1/firewall/rules | empty_bytes | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 42 | /api/v1/firewall/rules | spaces | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 43 | /api/v1/firewall/rules | huge_1mb | 200 | N | - | - | {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit |
| 44 | /api/v1/firewall/rules | deep_nest | 400 | Y | - | - | {"code":"10007","detail":"invalid character ':' after object key:value pair","me |
| 45 | /api/v1/firewall/rules | num_as_str | 200 | N | - | - | {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit |
| 46 | /api/v1/firewall/rules | unicode_emoji | 200 | N | - | - | {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit |
| 47 | /api/v1/firewall/rules | sql_injection | 200 | N | - | - | {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit |
| 48 | /api/v1/firewall/rules | control_chars | 400 | Y | - | - | {"code":"10007","detail":"invalid character '\\x00' in string literal","message" |
| 49 | /api/v1/nat/rules | empty_bytes | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 50 | /api/v1/nat/rules | spaces | 400 | Y | - | - | {"code":"10007","detail":"EOF","message":"json decode fail"}  |
| 51 | /api/v1/nat/rules | huge_1mb | 200 | N | - | - | {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri |
| 52 | /api/v1/nat/rules | deep_nest | 400 | Y | - | - | {"code":"10007","detail":"invalid character ':' after object key:value pair","me |
| 53 | /api/v1/nat/rules | num_as_str | 200 | N | - | - | {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri |
| 54 | /api/v1/nat/rules | unicode_emoji | 200 | N | - | - | {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri |
| 55 | /api/v1/nat/rules | sql_injection | 200 | N | - | - | {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri |
| 56 | /api/v1/nat/rules | control_chars | 400 | Y | - | - | {"code":"10007","detail":"invalid character '\\x00' in string literal","message" |
| 57 | /api/v1/policy/timeschedule | empty_bytes | 400 | Y | - | - | {"code":"10389","detail":"EOF","message":"invalid JSON body"}  |
| 58 | /api/v1/policy/timeschedule | spaces | 400 | Y | - | - | {"code":"10389","detail":"EOF","message":"invalid JSON body"}  |
| 59 | /api/v1/policy/timeschedule | huge_1mb | 422 | Y | - | - | {"code":"10403","detail":"to disable all time-based policies, send one rule with |
| 60 | /api/v1/policy/timeschedule | deep_nest | 400 | Y | - | - | {"code":"10389","detail":"invalid character ':' after object key:value pair","me |
| 61 | /api/v1/policy/timeschedule | num_as_str | 422 | Y | - | - | {"code":"10403","detail":"to disable all time-based policies, send one rule with |
| 62 | /api/v1/policy/timeschedule | unicode_emoji | 422 | Y | - | - | {"code":"10403","detail":"to disable all time-based policies, send one rule with |
| 63 | /api/v1/policy/timeschedule | sql_injection | 422 | Y | - | - | {"code":"10403","detail":"to disable all time-based policies, send one rule with |
| 64 | /api/v1/policy/timeschedule | control_chars | 400 | Y | - | - | {"code":"10389","detail":"invalid character '\\x00' in string literal","message" |
| 65 | /api/v1/firmware/confirm | empty_bytes | 422 | Y | - | - | {"code":"10503","detail":"EOF","message":"invalid body"}  |
| 66 | /api/v1/firmware/confirm | spaces | 422 | Y | - | - | {"code":"10503","detail":"EOF","message":"invalid body"}  |
| 67 | /api/v1/firmware/confirm | huge_1mb | 422 | Y | - | - | {"code":"10504","detail":"path points to the firmware image on the server","mess |
| 68 | /api/v1/firmware/confirm | deep_nest | 422 | Y | - | - | {"code":"10503","detail":"invalid character ':' after object key:value pair","me |
| 69 | /api/v1/firmware/confirm | num_as_str | 422 | Y | - | - | {"code":"10504","detail":"path points to the firmware image on the server","mess |
| 70 | /api/v1/firmware/confirm | unicode_emoji | 422 | Y | - | - | {"code":"10504","detail":"path points to the firmware image on the server","mess |
| 71 | /api/v1/firmware/confirm | sql_injection | 422 | Y | - | - | {"code":"10504","detail":"path points to the firmware image on the server","mess |
| 72 | /api/v1/firmware/confirm | control_chars | 422 | Y | - | - | {"code":"10503","detail":"invalid character '\\x00' in string literal","message" |
| 73 | /api/v1/acl/rules | empty_bytes | 400 | Y | - | - | {"code":"10090","detail":"EOF","message":"invalid body"}  |
| 74 | /api/v1/acl/rules | spaces | 400 | Y | - | - | {"code":"10090","detail":"EOF","message":"invalid body"}  |
| 75 | /api/v1/acl/rules | huge_1mb | 422 | Y | - | - | {"code":"10091","detail":"POST /acl/rules must carry at least one rule","message |
| 76 | /api/v1/acl/rules | deep_nest | 400 | Y | - | - | {"code":"10090","detail":"invalid character ':' after object key:value pair","me |
| 77 | /api/v1/acl/rules | num_as_str | 422 | Y | - | - | {"code":"10091","detail":"POST /acl/rules must carry at least one rule","message |
| 78 | /api/v1/acl/rules | unicode_emoji | 422 | Y | - | - | {"code":"10091","detail":"POST /acl/rules must carry at least one rule","message |
| 79 | /api/v1/acl/rules | sql_injection | 422 | Y | - | - | {"code":"10091","detail":"POST /acl/rules must carry at least one rule","message |
| 80 | /api/v1/acl/rules | control_chars | 400 | Y | - | - | {"code":"10090","detail":"invalid character '\\x00' in string literal","message" |
| 81 | /api/v1/tools/iperf | empty_bytes | 404 | Y | - | - | 404 page not found  |
| 82 | /api/v1/tools/iperf | spaces | 404 | Y | - | - | 404 page not found  |
| 83 | /api/v1/tools/iperf | huge_1mb | 0 | N | - | - | <urlopen error [Errno 104] Connection reset by peer> |
| 84 | /api/v1/tools/iperf | deep_nest | 404 | Y | - | - | 404 page not found  |
| 85 | /api/v1/tools/iperf | num_as_str | 404 | Y | - | - | 404 page not found  |
| 86 | /api/v1/tools/iperf | unicode_emoji | 404 | Y | - | - | 404 page not found  |
| 87 | /api/v1/tools/iperf | sql_injection | 404 | Y | - | - | 404 page not found  |
| 88 | /api/v1/tools/iperf | control_chars | 404 | Y | - | - | 404 page not found  |
| 89 | /api/v1/tools/capture/start | empty_bytes | 400 | Y | - | - | {"code":"10491","detail":"EOF","message":"invalid body"}  |
| 90 | /api/v1/tools/capture/start | spaces | 400 | Y | - | - | {"code":"10491","detail":"EOF","message":"invalid body"}  |
| 91 | /api/v1/tools/capture/start | huge_1mb | 422 | Y | - | - | {"code":"10492","detail":"interface is required","message":"missing interface"}  |
| 92 | /api/v1/tools/capture/start | deep_nest | 400 | Y | - | - | {"code":"10491","detail":"invalid character ':' after object key:value pair","me |
| 93 | /api/v1/tools/capture/start | num_as_str | 422 | Y | - | - | {"code":"10492","detail":"interface is required","message":"missing interface"}  |
| 94 | /api/v1/tools/capture/start | unicode_emoji | 422 | Y | - | - | {"code":"10492","detail":"interface is required","message":"missing interface"}  |
| 95 | /api/v1/tools/capture/start | sql_injection | 422 | Y | - | - | {"code":"10492","detail":"interface is required","message":"missing interface"}  |
| 96 | /api/v1/tools/capture/start | control_chars | 400 | Y | - | - | {"code":"10491","detail":"invalid character '\\x00' in string literal","message" |
| 97 | /api/v1/frp/proxies | empty_bytes | 404 | Y | - | - | 404 page not found  |
| 98 | /api/v1/frp/proxies | spaces | 404 | Y | - | - | 404 page not found  |
| 99 | /api/v1/frp/proxies | huge_1mb | 0 | N | - | - | <urlopen error [Errno 104] Connection reset by peer> |
| 100 | /api/v1/frp/proxies | deep_nest | 404 | Y | - | - | 404 page not found  |
| 101 | /api/v1/frp/proxies | num_as_str | 404 | Y | - | - | 404 page not found  |
| 102 | /api/v1/frp/proxies | unicode_emoji | 404 | Y | - | - | 404 page not found  |
| 103 | /api/v1/frp/proxies | sql_injection | 404 | Y | - | - | 404 page not found  |
| 104 | /api/v1/frp/proxies | control_chars | 404 | Y | - | - | 404 page not found  |
| 105 | /api/v1/tasks | empty_bytes | 400 | Y | - | - | {"code":"10513","detail":"EOF","message":"invalid JSON body"}  |
| 106 | /api/v1/tasks | spaces | 400 | Y | - | - | {"code":"10513","detail":"EOF","message":"invalid JSON body"}  |
| 107 | /api/v1/tasks | huge_1mb | 422 | Y | - | - | {"code":"10514","detail":"task type is required","message":"type required"}  |
| 108 | /api/v1/tasks | deep_nest | 400 | Y | - | - | {"code":"10513","detail":"invalid character ':' after object key:value pair","me |
| 109 | /api/v1/tasks | num_as_str | 422 | Y | - | - | {"code":"10514","detail":"task type is required","message":"type required"}  |
| 110 | /api/v1/tasks | unicode_emoji | 422 | Y | - | - | {"code":"10514","detail":"task type is required","message":"type required"}  |
| 111 | /api/v1/tasks | sql_injection | 422 | Y | - | - | {"code":"10514","detail":"task type is required","message":"type required"}  |
| 112 | /api/v1/tasks | control_chars | 400 | Y | - | - | {"code":"10513","detail":"invalid character '\\x00' in string literal","message" |
| 113 | /api/v1/ha/status | empty_bytes | 405 | Y | - | - | Method Not Allowed  |
| 114 | /api/v1/ha/status | spaces | 405 | Y | - | - | Method Not Allowed  |
| 115 | /api/v1/ha/status | huge_1mb | 0 | N | - | - | <urlopen error [Errno 104] Connection reset by peer> |
| 116 | /api/v1/ha/status | deep_nest | 405 | Y | - | - | Method Not Allowed  |
| 117 | /api/v1/ha/status | num_as_str | 405 | Y | - | - | Method Not Allowed  |
| 118 | /api/v1/ha/status | unicode_emoji | 405 | Y | - | - | Method Not Allowed  |
| 119 | /api/v1/ha/status | sql_injection | 405 | Y | - | - | Method Not Allowed  |
| 120 | /api/v1/ha/status | control_chars | 405 | Y | - | - | Method Not Allowed  |
| 121 | /api/v1/auth/sms/send | empty_bytes | 400 | Y | - | - | {"code":"11003","detail":"EOF","message":"invalid body"}  |
| 122 | /api/v1/auth/sms/send | spaces | 400 | Y | - | - | {"code":"11003","detail":"EOF","message":"invalid body"}  |
| 123 | /api/v1/auth/sms/send | huge_1mb | 422 | Y | - | - | {"code":"11004","detail":"phone is required","message":"missing phone"}  |
| 124 | /api/v1/auth/sms/send | deep_nest | 400 | Y | - | - | {"code":"11003","detail":"invalid character ':' after object key:value pair","me |
| 125 | /api/v1/auth/sms/send | num_as_str | 422 | Y | - | - | {"code":"11004","detail":"phone is required","message":"missing phone"}  |
| 126 | /api/v1/auth/sms/send | unicode_emoji | 422 | Y | - | - | {"code":"11004","detail":"phone is required","message":"missing phone"}  |
| 127 | /api/v1/auth/sms/send | sql_injection | 422 | Y | - | - | {"code":"11004","detail":"phone is required","message":"missing phone"}  |
| 128 | /api/v1/auth/sms/send | control_chars | 400 | Y | - | - | {"code":"11003","detail":"invalid character '\\x00' in string literal","message" |
| 129 | /api/v1/auth/sms/verify | empty_bytes | 400 | Y | - | - | {"code":"11008","detail":"EOF","message":"invalid body"}  |
| 130 | /api/v1/auth/sms/verify | spaces | 400 | Y | - | - | {"code":"11008","detail":"EOF","message":"invalid body"}  |
| 131 | /api/v1/auth/sms/verify | huge_1mb | 422 | Y | - | - | {"code":"11009","detail":"phone and code are required","message":"missing phone  |
| 132 | /api/v1/auth/sms/verify | deep_nest | 400 | Y | - | - | {"code":"11008","detail":"invalid character ':' after object key:value pair","me |
| 133 | /api/v1/auth/sms/verify | num_as_str | 422 | Y | - | - | {"code":"11009","detail":"phone and code are required","message":"missing phone  |
| 134 | /api/v1/auth/sms/verify | unicode_emoji | 422 | Y | - | - | {"code":"11009","detail":"phone and code are required","message":"missing phone  |
| 135 | /api/v1/auth/sms/verify | sql_injection | 422 | Y | - | - | {"code":"11009","detail":"phone and code are required","message":"missing phone  |
| 136 | /api/v1/auth/sms/verify | control_chars | 400 | Y | - | - | {"code":"11008","detail":"invalid character '\\x00' in string literal","message" |
| 137 | /api/v1/auth/wechat/login | empty_bytes | 400 | Y | - | - | {"code":"10497","detail":"EOF","message":"invalid body"}  |
| 138 | /api/v1/auth/wechat/login | spaces | 400 | Y | - | - | {"code":"10497","detail":"EOF","message":"invalid body"}  |
| 139 | /api/v1/auth/wechat/login | huge_1mb | 502 | N | - | - | {"errcode":41008,"errmsg":"missing code, rid: 6ae34246-56c3ecef-292b95ca","error |
| 140 | /api/v1/auth/wechat/login | deep_nest | 400 | Y | - | - | {"code":"10497","detail":"invalid character ':' after object key:value pair","me |
| 141 | /api/v1/auth/wechat/login | num_as_str | 502 | N | - | - | {"errcode":41008,"errmsg":"missing code, rid: 6ae34246-3e69fdb0-481e3817","error |
| 142 | /api/v1/auth/wechat/login | unicode_emoji | 502 | N | - | - | {"errcode":41008,"errmsg":"missing code, rid: 6ae34246-36278385-7c42584d","error |
| 143 | /api/v1/auth/wechat/login | sql_injection | 502 | N | - | - | {"errcode":41008,"errmsg":"missing code, rid: 6ae34246-0b51f4a5-4ad976c5","error |
| 144 | /api/v1/auth/wechat/login | control_chars | 400 | Y | - | - | {"code":"10497","detail":"invalid character '\\x00' in string literal","message" |
| 145 | /api/v1/webfilter/apply | empty_bytes | 400 | Y | - | - | {"code":"10504","detail":"EOF","message":"invalid body"}  |
| 146 | /api/v1/webfilter/apply | spaces | 400 | Y | - | - | {"code":"10504","detail":"EOF","message":"invalid body"}  |
| 147 | /api/v1/webfilter/apply | huge_1mb | 403 | Y | - | - | {"code":"10503","detail":"没有 /etc/dnsmasq.d 的写权限（需要 root 运行或将 MGM_WEBFILTER_CONF |
| 148 | /api/v1/webfilter/apply | deep_nest | 400 | Y | - | - | {"code":"10504","detail":"invalid character ':' after object key:value pair","me |
| 149 | /api/v1/webfilter/apply | num_as_str | 403 | Y | - | - | {"code":"10503","detail":"没有 /etc/dnsmasq.d 的写权限（需要 root 运行或将 MGM_WEBFILTER_CONF |
| 150 | /api/v1/webfilter/apply | unicode_emoji | 403 | Y | - | - | {"code":"10503","detail":"没有 /etc/dnsmasq.d 的写权限（需要 root 运行或将 MGM_WEBFILTER_CONF |
| 151 | /api/v1/webfilter/apply | sql_injection | 403 | Y | - | - | {"code":"10503","detail":"没有 /etc/dnsmasq.d 的写权限（需要 root 运行或将 MGM_WEBFILTER_CONF |
| 152 | /api/v1/webfilter/apply | control_chars | 400 | Y | - | - | {"code":"10504","detail":"invalid character '\\x00' in string literal","message" |
| 153 | /api/v1/acl/mac-filter | empty_bytes | 400 | Y | - | - | {"code":"10505","detail":"EOF","message":"invalid body"}  |
| 154 | /api/v1/acl/mac-filter | spaces | 400 | Y | - | - | {"code":"10505","detail":"EOF","message":"invalid body"}  |
| 155 | /api/v1/acl/mac-filter | huge_1mb | 422 | Y | - | - | {"code":"10506","detail":"invalid mode \"\": must be blacklist/whitelist","messa |
| 156 | /api/v1/acl/mac-filter | deep_nest | 400 | Y | - | - | {"code":"10505","detail":"invalid character ':' after object key:value pair","me |
| 157 | /api/v1/acl/mac-filter | num_as_str | 422 | Y | - | - | {"code":"10506","detail":"invalid mode \"\": must be blacklist/whitelist","messa |
| 158 | /api/v1/acl/mac-filter | unicode_emoji | 422 | Y | - | - | {"code":"10506","detail":"invalid mode \"\": must be blacklist/whitelist","messa |
| 159 | /api/v1/acl/mac-filter | sql_injection | 422 | Y | - | - | {"code":"10506","detail":"invalid mode \"\": must be blacklist/whitelist","messa |
| 160 | /api/v1/acl/mac-filter | control_chars | 400 | Y | - | - | {"code":"10505","detail":"invalid character '\\x00' in string literal","message" |
| 161 | /api/v1/security/mac | empty_bytes | 404 | Y | - | - | 404 page not found  |
| 162 | /api/v1/security/mac | spaces | 404 | Y | - | - | 404 page not found  |
| 163 | /api/v1/security/mac | huge_1mb | 404 | Y | - | - | 404 page not found  |
| 164 | /api/v1/security/mac | deep_nest | 404 | Y | - | - | 404 page not found  |
| 165 | /api/v1/security/mac | num_as_str | 404 | Y | - | - | 404 page not found  |
| 166 | /api/v1/security/mac | unicode_emoji | 404 | Y | - | - | 404 page not found  |
| 167 | /api/v1/security/mac | sql_injection | 404 | Y | - | - | 404 page not found  |
| 168 | /api/v1/security/mac | control_chars | 404 | Y | - | - | 404 page not found  |

## 危险用例详情 (500/栈/XSS/连接失败)

### /api/v1/tools/iperf / huge_1mb (HTTP 0)

```
<urlopen error [Errno 104] Connection reset by peer>
```

### /api/v1/frp/proxies / huge_1mb (HTTP 0)

```
<urlopen error [Errno 104] Connection reset by peer>
```

### /api/v1/ha/status / huge_1mb (HTTP 0)

```
<urlopen error [Errno 104] Connection reset by peer>
```

## 接受但无严格校验的用例 (HTTP 2xx)

- /api/v1/qos/limits / huge_1mb: {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}

- /api/v1/qos/limits / num_as_str: {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}

- /api/v1/qos/limits / unicode_emoji: {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}

- /api/v1/qos/limits / sql_injection: {"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]}

- /api/v1/firewall/rules / huge_1mb: {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit
- /api/v1/firewall/rules / num_as_str: {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit
- /api/v1/firewall/rules / unicode_emoji: {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit
- /api/v1/firewall/rules / sql_injection: {"nft":"table inet router {\n  chain input {\n    type filter hook input priorit
- /api/v1/nat/rules / huge_1mb: {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri
- /api/v1/nat/rules / num_as_str: {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri
- /api/v1/nat/rules / unicode_emoji: {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri
- /api/v1/nat/rules / sql_injection: {"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting pri

## 结论

存在危险用例，需进一步排查。
