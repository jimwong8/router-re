# 真实执行 POST 端点测试模式（Real-Apply Endpoint Testing Pattern）

适用场景：本项目中任何 POST 端点真实执行系统操作（`Apply` 写文件、`ApplyClientLimits` 下发 nft/tc、`Apply` 重启 pppoe-server）的测试。

## 核心规则

真实执行端点在无环境/无权限的开发/CI 主机上会返回 500（`permission denied`、`no such file` 等）。测试不能假设总是 200，否则会产生假阳性（修改未落地但测试看起来正常）。

## 测试双路径模式

每个涉及真实应用的端点测试必须同时覆盖两种路径（同一测试函数内，用 `if` 分支控制，不拆分为两个测试函数）：

1. **真实成功路径**（可选，通过注入临时目录或 FakeRunner）：
   - 注入 `Applier` 的 `ConfPath` 到 `t.TempDir()` 或使用 `FakeRunner`（断言命令序列：`nft delete` 在 `nft -f -` 前，`tc` 命令含 `htb`/`fw`）。
   - 断言状态码 200、`applied:true`（或对应的成功标志）、响应体含渲染规则文本（`table inet ...`、`meta mark set`、`htb rate`、`fw classid`）。

2. **真实环境无权限路径**（默认，不修改真实 `Applier`）：
   - 保持真实 `Applier`（不注入临时目录），让真实系统操作发生。
   - 当本机无目标软件（dnsmasq/pppoe-server 未安装、无 `/etc/dnsmasq.d` 写入权限、nft 无 root 权限）时，真实 `Apply` 正确返回错误（500）。
   - 测试断言接受两种可能状态码：`if rr.Code == 500 { assert error body contains expected error msg; skip body/render assertions; continue/skip rest } else { assert 200 + applied:true + body contains render text }`。
   - **关键：内容断言（渲染文本检查）必须在状态判断之后执行**，且当状态为 500 时跳过内容检查。否则无环境时内容断言假失败（文件未写入，渲染规则为空或缺失）。

## 验证完整流程（每次修改后执行）

修改涉及真实执行端点的代码后，执行完整防护流程（来自 SKILL.md 增强规则）：

1. `assert old in content`（替换前断言匹配成功，防止 python `content.replace` 静默失败）。
2. 替换后 `grep -n -A8 'func (s \*Server) handleX' file` 确认新函数体在正确位置（不是被贴到错位处的重复行）。
3. `go build ./...`（确认无重复 import/redeclared/undefined 错误）。
4. `ss -tlnp | grep <port>` 核对新起进程 PID 等于刚编译的二进制（防止旧进程静默接管请求）。
5. `curl -H "Authorization: Bearer $TOKEN" ...` 真实端到端验证（POST 真实执行行为是否按预期：成功时 `applied:true`、失败时明确错误信息而非 panic/假 200）。

## 相关 SKILL.md 规则

本参考文件配合 SKILL.md 中以下规则使用：
- `python content.replace` 静默失败规则（第 52 行增强版，含完整防护流程描述）。
- `MockStore.Get` 永远返回 `(val, nil)` 规则（键存在判断用值非空、清旧键加硬上限）。
- 并行编辑 `server.go` 时的精确替换与真实状态验证流程。
- 真实执行 POST 端点的测试双路径模式（本文件）。

## 适用端点列表（本项目中）

- `POST /api/v1/dhcp/config`（`Apply` 写 `/etc/dnsmasq.d/` + reload dnsmasq）
- `POST /api/v1/pppoe/config`（`Apply` 写 `/etc/ppp/pppoe-server-options` + `chap-secrets` + 重启）
- `POST /api/v1/qos/client-limits`（`ApplyClientLimits` 下发 nft mark + tc htb 限速）
- `POST /api/v1/dpi/rules`（`Apply` 下发 nft 规则，实际 `ApplyMarks` 使用）
