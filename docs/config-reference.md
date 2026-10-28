# 配置参考

## conntrack 字节统计（acct）

- `GET /api/v1/conntrack/acct` → `s.auth(s.handleGetConntrackAcct)`
- `POST /api/v1/conntrack/acct` → `s.auth(s.auditWrap("conntrack.acct.set", s.handleSetConntrackAcct))`

认证：Bearer，POST 有审计 wrap `auditWrap`。

## 终端列表（terminals）

- `GET /api/v1/terminals` → `s.auth(s.handleListTerminals)`

认证：Bearer，无 audit wrap（只读查询）。

## 固件确认（firmware/confirm）

- `POST /api/v1/firmware/confirm` → `s.auth(s.handleFirmwareConfirm)`

认证：Bearer，无 audit wrap（确认操作直接挂 auth）。
