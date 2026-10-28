# p96 未覆盖端点批测试报告（vlan、wol/targets）

> 生成: 2026-10-29 04:25:53  BASE=http://127.0.0.1:9090

**汇总**: 8 步 | PASS 8 / FAIL 0

| # | 端点 | 步骤 | 结果 | 说明 |
|---|------|------|------|------|
| 1 | 登录 | 登录 admin | PASS | Bearer token 获取成功 |
| 2 | vlan | vlan POST 建 p96test | PASS | applied=True in_returned_list |
| 3 | vlan | vlan GET 列表确认 | PASS | name=p96test parent=ens18 vid=100 |
| 4 | vlan | vlan DELETE 清理 | PASS | HTTP 200 deleted=p96test 列表中已无 |
| 5 | wol | wol GET 列表 | PASS | HTTP 200 count=0 |
| 6 | wol | wol POST 建 p96test | PASS | count=1 mac=00:11:22:33:44:55 |
| 7 | wol | wol GET 确认 | PASS | name=p96test mac=00:11:22:33:44:55 iface=ens18 |
| 8 | wol | wol 清理（POST 空目标） | PASS | count=0 列表中已无 p96test |

## 清理状态
- vlan `p96test` 残留: 已删除/干净
- wol `p96test` 残留: 已删除/干净
- 全部清理干净: YES

## 1. 登录 admin

- 结果: **PASS** — Bearer token 获取成功

```
token:<redacted>
```

## 2. vlan POST 建 p96test

- 结果: **PASS** — applied=True in_returned_list

```
{"applied":true,"vlans":[{"parent":"ens18","vid":100,"name":"p96test"}]}

```

## 3. vlan GET 列表确认

- 结果: **PASS** — name=p96test parent=ens18 vid=100

```
{"parent": "ens18", "vid": 100, "name": "p96test"}
```

## 4. vlan DELETE 清理

- 结果: **PASS** — HTTP 200 deleted=p96test 列表中已无

```
{"deleted":"p96test"}

```

## 5. wol GET 列表

- 结果: **PASS** — HTTP 200 count=0

```
{"count":0,"targets":[]}

```

## 6. wol POST 建 p96test

- 结果: **PASS** — count=1 mac=00:11:22:33:44:55

```
{"count":1,"targets":[{"name":"p96test","mac":"00:11:22:33:44:55","iface":"ens18","broadcast":""}]}

```

## 7. wol GET 确认

- 结果: **PASS** — name=p96test mac=00:11:22:33:44:55 iface=ens18

```
{"name": "p96test", "mac": "00:11:22:33:44:55", "iface": "ens18", "broadcast": ""}
```

## 8. wol 清理（POST 空目标）

- 结果: **PASS** — count=0 列表中已无 p96test

```
{"count":0,"targets":[]}

```
