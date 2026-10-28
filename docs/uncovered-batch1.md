# p89-uncovered-batch1: 未覆盖端点测试（批1）

生成时间: 2026-10-29 01:38:25

目标: mgmtd @ http://127.0.0.1:9090（root 运行，admin 登录取 Bearer）。全部为真实 HTTP 调用。

## 汇总: 8 PASS / 1 FAIL / 0 SKIP（共 9 步）

覆盖端点: accounts/import、accounts/{u}/bind-mac、accounts/{u}/reset-usage、
users/{u}/password、alerts/test、logs/sources、logs/syslog(+syslog/test)。

## 分步结果

### accounts/import

- **状态**: FAIL
- **详情**: created=0, total=5, errors=['line 1: account "p89test-acct1" already exists', 'line 2: account "p89test-acct2" already exists', 'line 5: need at least 3 fields'] (期望 created=2, total=5, 1条错误行)

### accounts/bind-mac

- **状态**: PASS
- **详情**: bound=True, account.bound_mac='aa:bb:cc:dd:ee:01' (期望 bound=true, mac已写入并小写化)

### accounts/reset-usage

- **状态**: PASS
- **详情**: reset=True, username='p89test-acct1', used_mb=0 (期望 reset=true)

### users/password(不存在用户->409)

- **状态**: PASS
- **详情**: HTTP 409, code=10220, detail='user "p89test-nonexistent" not found' (期望 409 user not found)

### users/password(弱密码->422)

- **状态**: PASS
- **详情**: HTTP 422: {"code":"10219","detail":"password must contain at least one letter and one digit","message":"weak password"}
 (期望 422 weak password)

### alerts/test

- **状态**: PASS
- **详情**: matched=[], count=0 (纯 dry-run，无外发告警，HTTP 200)

### logs/sources

- **状态**: PASS
- **详情**: sources 长度=30, count=30 (HTTP 200)

### logs/syslog GET

- **状态**: PASS
- **详情**: target={'name': '', 'address': '', 'protocol': '', 'format': '', 'facility': '', 'enabled': False}, rsyslog='@:' (HTTP 200，单条目标，无分页参数)

### logs/syslog/test

- **状态**: PASS
- **详情**: HTTP 200, ok=True, detail='udp test message sent: ' (真实 syslog 发送尝试)
