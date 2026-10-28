# API 输入模糊测试报告

> 生成: 2026-10-28 22:17:38

- 端点数: 10, 畸形类型: 8, 总组数: 80
- 安全 (4xx 无栈无XSS): 68
- 接受 (2xx，无严格校验): 12
- 危险 (500/栈/XSS/连接失败): 0
- HTTP 500: 0, HTTP 4xx: 68, HTTP 2xx: 12, 连接失败: 0
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

80 组畸形输入无 500、无 Go 栈泄露、无 XSS 反射；68 组返回 4xx，12 组被 2xx 接受（qos/firewall/nat 缺严格校验，建议后续加固）。
