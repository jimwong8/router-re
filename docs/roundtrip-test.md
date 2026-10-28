# 写端点 Round-trip 测试报告

> 生成: 2026-10-28 23:33:30  BASE=http://127.0.0.1:9090

**汇总**: 测了 5 个写端点 / PASS 5 / FAIL 0

| # | 写端点 | 读端点 | 写码 | 读码 | 结果 |
|---|---|---|---|---|---|
| 1 | `/api/v1/voucher/generate` | `/api/v1/voucher/list` | 200 | 200 | PASS |
| 2 | `/api/v1/portal/template` | `/api/v1/portal/templates` | 200 | 200 | PASS |
| 3 | `/api/v1/linkcheck/targets` | `/api/v1/linkcheck/targets` | 200 | 200 | PASS |
| 4 | `/api/v1/ddns/records` | `/api/v1/ddns/records` | 200 | 200 | PASS |
| 5 | `/api/v1/policy/timeschedule` | `/api/v1/policy/timeschedule` | 200 | 200 | PASS |

## 1. voucher/generate→list

- 写入值（完整，不截断——截断会让审查者无法核对读回命中）:

```json
{
  "count": 1,
  "code_len": 10,
  "minutes": 30,
  "traffic_mb": 0,
  "device_limit": 1,
  "expire_days": 0,
  "note": "roundtrip-test"
}
```

- 写接口返回（用于确认写成功）:

```json
{
  "generated": 1,
  "note": "卡密明文只返回这一次，请立即保存；之后查询只能看到打码版本",
  "vouchers": [
    {
      "code": "TVZYQKKLGH",
      "password": "TCTDJ9KRN3",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793201610,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test"
    }
  ]
}
```

- **落盘确认 verified = True**（True 表示写进去的标识能在对应读接口里找到）

- 读回值（完整）:

```json
{
  "count": 33,
  "unused": 18,
  "used": 15,
  "vouchers": [
    {
      "code": "R4VTY4YKNE",
      "password": "CH******D2",
      "minutes": 1440,
      "traffic_mb": 10240,
      "device_limit": 2,
      "created_at": 1793113374,
      "expires_at": 1795705374,
      "used_at": 1793113374,
      "used": true,
      "note": "测试批次",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "ULYU6GAQSH",
      "password": "42******SU",
      "minutes": 1440,
      "traffic_mb": 10240,
      "device_limit": 2,
      "created_at": 1793113374,
      "expires_at": 1795705374,
      "used_at": 0,
      "used": false,
      "note": "测试批次",
      "remaining_min": 1440,
      "expired": false
    },
    {
      "code": "LTUSMJL49D",
      "password": "E8******HL",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793188376,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "3DS45H5YPH",
      "password": "XQ******AS",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793188399,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "82MBYGEZR8",
      "password": "2M******KE",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793189737,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "NRPQABLRGW",
      "password": "PJ******G4",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793190036,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "NDT77R4THB",
      "password": "8V******N5",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793190774,
      "expires_at": 0,
      "used_at": 1793190774,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "89KXTM8BY9",
      "password": "9X******6T",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191645,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "NYGRESBDBT",
      "password": "QW******9G",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191646,
      "expires_at": 0,
      "used_at": 1793191646,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "FFNZMPB2ZM",
      "password": "ZR******K7",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191718,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "Z4PJKCPQN4",
      "password": "3R******7C",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191718,
      "expires_at": 0,
      "used_at": 1793191719,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "E2G53VL6YF",
      "password": "AN******UQ",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191869,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "H83K6VDQJ4",
      "password": "W4******BZ",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191870,
      "expires_at": 0,
      "used_at": 1793191870,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "3DZDZKVBBU",
      "password": "F6******H7",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191944,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "NLNNYGBRXJ",
      "password": "YK******PR",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793191944,
      "expires_at": 0,
      "used_at": 1793191944,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "VU8PUR3GNF",
      "password": "DQ******KY",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192018,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "AZUSNT2H8L",
      "password": "QL******QX",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192018,
      "expires_at": 0,
      "used_at": 1793192018,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "5BY2C826SD",
      "password": "6R******82",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192497,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "GTGMGK5HFQ",
      "password": "TB******4D",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192652,
      "expires_at": 0,
      "used_at": 1793192652,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "YLKJA8PP5V",
      "password": "R6******DU",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192791,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "XQS2GSGMCF",
      "password": "LM******WT",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192807,
      "expires_at": 0,
      "used_at": 1793192807,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "T7C2WJWWRB",
      "password": "FG******WJ",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793192913,
      "expires_at": 0,
      "used_at": 1793192913,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "PZRLFJSV5H",
      "password": "AP******2B",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793193024,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "JCBZ84SCUQ",
      "password": "U2******B2",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793193029,
      "expires_at": 0,
      "used_at": 1793193029,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "8XH74NMPAP",
      "password": "JY******B5",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793194210,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "EUWDBHE6HK",
      "password": "C2******T2",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793194215,
      "expires_at": 0,
      "used_at": 1793194215,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "NKGQNP3CZK",
      "password": "LA******JF",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793195288,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "KVHC2JXZ8F",
      "password": "UM******NE",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793195293,
      "expires_at": 0,
      "used_at": 1793195293,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "SYTMVY3CXP",
      "password": "RP******JF",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793195842,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "GNFSG7JMZ9",
      "password": "SS******YJ",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793195847,
      "expires_at": 0,
      "used_at": 1793195847,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "QTJZE3PCWB",
      "password": "W4******H9",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793197146,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    },
    {
      "code": "FKXKJGP32D",
      "password": "2Q******8U",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793197151,
      "expires_at": 0,
      "used_at": 1793197151,
      "used": true,
      "note": "p47 roundtrip",
      "remaining_min": 0,
      "expired": false
    },
    {
      "code": "TVZYQKKLGH",
      "password": "TC******N3",
      "minutes": 30,
      "traffic_mb": 0,
      "device_limit": 1,
      "created_at": 1793201610,
      "expires_at": 0,
      "used_at": 0,
      "used": false,
      "note": "roundtrip-test",
      "remaining_min": 30,
      "expired": false
    }
  ]
}
```


## 2. portal/template→templates

- 写入值（完整，不截断——截断会让审查者无法核对读回命中）:

```json
{
  "template": {
    "id": "password",
    "name": "密码认证",
    "kind": "password",
    "title": "roundtrip-portal-title",
    "logo_url": "",
    "bg_color": "#1a2b3c",
    "text_color": "#ffffff",
    "notice": "请输入管理员分配的密码完成认证",
    "redirect_url": ""
  }
}
```

- 写接口返回（用于确认写成功）:

```json
{
  "saved": true,
  "template": {
    "id": "password",
    "name": "密码认证",
    "kind": "password",
    "title": "roundtrip-portal-title",
    "logo_url": "",
    "bg_color": "#1a2b3c",
    "text_color": "#ffffff",
    "notice": "请输入管理员分配的密码完成认证",
    "redirect_url": ""
  }
}
```

- **落盘确认 verified = True**（True 表示写进去的标识能在对应读接口里找到）

- 读回值（完整）:

```json
{
  "builtin": [
    {
      "id": "password",
      "name": "密码认证",
      "kind": "password",
      "title": "欢迎使用本网络",
      "logo_url": "",
      "bg_color": "#1a2b3c",
      "text_color": "#ffffff",
      "notice": "请输入管理员分配的密码完成认证",
      "redirect_url": ""
    },
    {
      "id": "voucher",
      "name": "卡密认证",
      "kind": "voucher",
      "title": "卡密认证",
      "logo_url": "",
      "bg_color": "#2e8b57",
      "text_color": "#ffffff",
      "notice": "请输入购买的卡密（券码）完成认证",
      "redirect_url": ""
    },
    {
      "id": "sms",
      "name": "短信认证",
      "kind": "sms",
      "title": "手机短信认证",
      "logo_url": "",
      "bg_color": "#0a84ff",
      "text_color": "#ffffff",
      "notice": "请输入手机号并获取验证码完成认证",
      "redirect_url": ""
    },
    {
      "id": "wechat",
      "name": "微信认证",
      "kind": "wechat",
      "title": "微信扫码认证",
      "logo_url": "",
      "bg_color": "#07c160",
      "text_color": "#ffffff",
      "notice": "请使用微信扫码关注后完成认证",
      "redirect_url": ""
    }
  ],
  "count": 4,
  "current": {
    "id": "password",
    "name": "密码认证",
    "kind": "password",
    "title": "roundtrip-portal-title",
    "logo_url": "",
    "bg_color": "#1a2b3c",
    "text_color": "#ffffff",
    "notice": "请输入管理员分配的密码完成认证",
    "redirect_url": ""
  }
}
```


## 3. linkcheck/targets→targets

- 写入值（完整，不截断——截断会让审查者无法核对读回命中）:

```json
{
  "targets": [
    {
      "id": "rt-probe-1",
      "name": "roundtrip",
      "iface": "eth0",
      "kind": "icmp",
      "host": "127.0.0.1",
      "interval": 30,
      "timeout": 5,
      "fail_threshold": 3,
      "recover_threshold": 2,
      "enabled": true
    }
  ]
}
```

- 写接口返回（用于确认写成功）:

```json
{
  "count": 1,
  "targets": [
    {
      "id": "rt-probe-1",
      "name": "roundtrip",
      "iface": "eth0",
      "kind": "icmp",
      "host": "127.0.0.1",
      "url": "",
      "interval": 30,
      "timeout": 5,
      "fail_threshold": 3,
      "recover_threshold": 2,
      "enabled": true
    }
  ]
}
```

- **落盘确认 verified = True**（True 表示写进去的标识能在对应读接口里找到）

- 读回值（完整）:

```json
{
  "count": 1,
  "states": {},
  "targets": [
    {
      "id": "rt-probe-1",
      "name": "roundtrip",
      "iface": "eth0",
      "kind": "icmp",
      "host": "127.0.0.1",
      "url": "",
      "interval": 30,
      "timeout": 5,
      "fail_threshold": 3,
      "recover_threshold": 2,
      "enabled": true
    }
  ]
}
```


## 4. ddns/records→records

- 写入值（完整，不截断——截断会让审查者无法核对读回命中）:

```json
{
  "records": [
    {
      "id": "rt-ddns-1",
      "provider": "cloudflare",
      "domain": "example.com",
      "subdomain": "rt",
      "token": "roundtriptoken1234",
      "enabled": true
    }
  ],
  "confirm": true
}
```

- 写接口返回（用于确认写成功）:

```json
{
  "count": 1,
  "records": [
    {
      "id": "rt-ddns-1",
      "provider": "cloudflare",
      "domain": "example.com",
      "subdomain": "rt",
      "token": "roun**********1234",
      "enabled": true,
      "last_ip": "",
      "last_ok": 0,
      "last_error": ""
    }
  ]
}
```

- **落盘确认 verified = True**（True 表示写进去的标识能在对应读接口里找到）

- 读回值（完整）:

```json
{
  "count": 1,
  "providers": [
    {
      "key": "cloudflare",
      "name": "Cloudflare",
      "needs_token": true,
      "needs_user": false,
      "endpoint": "https://api.cloudflare.com/client/v4/zones/{zone}/dns_records/{id}"
    },
    {
      "key": "aliyun",
      "name": "Aliyun",
      "needs_token": true,
      "needs_user": false,
      "endpoint": "https://alidns.aliyuncs.com/"
    },
    {
      "key": "duckdns",
      "name": "DuckDNS",
      "needs_token": true,
      "needs_user": false,
      "endpoint": "https://www.duckdns.org/update?domains={domain}&token={token}&ip={ip}"
    },
    {
      "key": "noip",
      "name": "No-IP",
      "needs_token": false,
      "needs_user": true,
      "endpoint": "https://dynupdate.no-ip.com/nic/update?hostname={host}&myip={ip}"
    },
    {
      "key": "custom",
      "name": "Custom (generic HTTP)",
      "needs_token": false,
      "needs_user": false,
      "endpoint": "{endpoint}"
    }
  ],
  "records": [
    {
      "id": "rt-ddns-1",
      "provider": "cloudflare",
      "domain": "example.com",
      "subdomain": "rt",
      "token": "roun**********1234",
      "enabled": true,
      "last_ip": "",
      "last_ok": 0,
      "last_error": ""
    }
  ]
}
```


## 5. policy/timeschedule→timeschedule

- 写入值（完整，不截断——截断会让审查者无法核对读回命中）:

```json
{
  "rules": [
    {
      "id": "rt-rule-1",
      "name": "roundtrip",
      "days": [
        1,
        2,
        3,
        4,
        5
      ],
      "start_min": 60,
      "end_min": 120,
      "targets": [
        "10.0.0.0/24"
      ],
      "action": "ratelimit",
      "rate_kbps": 1000,
      "enabled": true
    }
  ],
  "confirm": true
}
```

- 写接口返回（用于确认写成功）:

```json
{
  "applied": true,
  "count": 1,
  "nft": "table inet timesched {\n\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n\t\t# no active rules\n\t}\n}\n",
  "output": ""
}
```

- **落盘确认 verified = True**（True 表示写进去的标识能在对应读接口里找到）

- 读回值（完整）:

```json
{
  "count": 1,
  "now": "2026-10-28 23:33:30 Wed",
  "rules": [
    {
      "id": "rt-rule-1",
      "name": "roundtrip",
      "days": [
        1,
        2,
        3,
        4,
        5
      ],
      "start_min": 60,
      "end_min": 120,
      "targets": [
        "10.0.0.0/24"
      ],
      "action": "ratelimit",
      "rate_kbps": 1000,
      "enabled": true,
      "active_now": false
    }
  ],
  "timezone": "Asia/Shanghai"
}
```


