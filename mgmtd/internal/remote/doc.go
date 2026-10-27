// Package remote 提供远程访问（SSH/Web/Telnet）、系统时间与网络唤醒(WOL)的配置与执行层。
//
// 设计取舍（与 internal/traffic 一脉相承）：
//   - 一切外部命令统一走 execx.Runner，可注入 FakeRunner 单测，不依赖真实系统。
//   - **绝不直接改写 sshd_config / web 端口配置**：这些是"切开就锁死自己"的危险操作，
//     因此 ApplyAccess 只负责执行安全无侵入的 systemctl 开关注，端口/监听项仅返回
//     "需要人工处理"的清单，交由运维在控制台上确认。
//   - 禁止任何 nftables `flush ruleset`；本包不含 nftables 代码。
package remote
