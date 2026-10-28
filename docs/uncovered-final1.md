# p95 未覆盖端点批测试报告（ddns/records、system/timezones、users/password）

> 生成: 2026-10-29 04:19:13  BASE=http://127.0.0.1:9090

**汇总**: 10 步 | PASS 10 / FAIL 0

| # | 端点 | 步骤 | 结果 | 说明 |
|---|------|------|------|------|
| 1 | 登录 | 登录 admin | PASS | Bearer token 获取成功 |
| 2 | ddns/records | ddns/records 创建(POST全量替换) | PASS | count=1 |
| 3 | ddns/records | ddns/records 读回拿 id | PASS | id=p95test-record masked_token=p95t*******ret1 |
| 4 | ddns/records | ddns/records 更新 | PASS | last_ip=203.0.113.50 |
| 5 | ddns/records | ddns/records 删除{id} | PASS | HTTP 200 deleted=True, 列表中已无该 id |
| 6 | system/timezones | system/timezones | PASS | HTTP 200 count=601 含 Asia/Shanghai |
| 7 | users | users 建临时账号 | PASS | HTTP 201 user=p95test |
| 8 | users/{u}/password | users/{u}/password 改密 | PASS | HTTP 200 changed=p95test |
| 9 | 改密生效验证 | 改密生效验证 | PASS | 旧密401, 新密200 |
| 10 | users | users 删除账号清理 | PASS | DELETE HTTP 200 deleted=p95test, 删除后登录=401 |

## 清理状态
- ddns p95test-record 残留: 无
- users p95test 账号残留: 无

## 1. 登录 admin

- 结果: **PASS** — Bearer token 获取成功

```
token:<redacted>
```

## 2. ddns/records 创建(POST全量替换)

- 结果: **PASS** — count=1

```
{"count":1,"records":[{"id":"p95test-record","provider":"duckdns","domain":"p95test.example.com","subdomain":"","token":"p95t*******ret1","enabled":true,"last_ip":"","last_ok":0,"last_error":""}]}

```

## 3. ddns/records 读回拿 id

- 结果: **PASS** — id=p95test-record masked_token=p95t*******ret1

```
{"id": "p95test-record", "provider": "duckdns", "domain": "p95test.example.com", "subdomain": "", "token": "p95t*******ret1", "enabled": true, "last_ip": "", "last_ok": 0, "last_error": ""}
```

## 4. ddns/records 更新

- 结果: **PASS** — last_ip=203.0.113.50

```
{"count":1,"records":[{"id":"p95test-record","provider":"duckdns","domain":"p95test.example.com","subdomain":"","token":"p95t*******ret1","enabled":true,"last_ip":"203.0.113.50","last_ok":0,"last_error":""}]}

```

## 5. ddns/records 删除{id}

- 结果: **PASS** — HTTP 200 deleted=True, 列表中已无该 id

```
{"deleted":true}

```

## 6. system/timezones

- 结果: **PASS** — HTTP 200 count=601 含 Asia/Shanghai

```
{"count":601,"timezones":["Africa/Abidjan","Africa/Accra","Africa/Addis_Ababa","Africa/Algiers","Africa/Asmara","Africa/Asmera","Africa/Bamako","Africa/Bangui","Africa/Banjul","Africa/Bissau","Africa/Blantyre","Africa/Brazzaville","Afric...
```

## 7. users 建临时账号

- 结果: **PASS** — HTTP 201 user=p95test

```
{"user":{"username":"p95test","role":"viewer","enabled":true,"created_at":1793218752,"last_login":0}}

```

## 8. users/{u}/password 改密

- 结果: **PASS** — HTTP 200 changed=p95test

```
{"changed":"p95test"}

```

## 9. 改密生效验证

- 结果: **PASS** — 旧密401, 新密200

```
old=401 new=200
```

## 10. users 删除账号清理

- 结果: **PASS** — DELETE HTTP 200 deleted=p95test, 删除后登录=401

```
{"deleted":"p95test"}

```
