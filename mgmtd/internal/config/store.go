// Package config 提供路由器配置存储的抽象接口。
//
// Store 接口抽象了底层 UCI (OpenWrt Unified Configuration Interface) 配置系统，
// 并提供事务语义：Snapshot 保存当前状态，Rollback 恢复到某个快照，Commit 固化。
package config

// Store 是配置存储的抽象接口。
//
// Get/Set 读写单个 (section, option) 配置项。
// Snapshot 返回当前配置的一个快照标识符，可用于后续 Rollback。
// Rollback 将配置恢复到指定快照标识符对应的状态。
// Commit 固化当前状态（在持久化实现中生效）。
type Store interface {
	Get(section, option string) (string, error)
	Set(section, option, value string) error
	// Unset 删除单个 (section, option)。
	//
	// 为什么需要它：Set 只能写值，写空串和"键不存在"在下层实现里无法区分
	// （FileStore 的 Get 对缺失键返回 ""），于是"按索引持久化的列表"没法
	// 截断——见 server.handleSetFirewallRules 的幽灵规则 bug。
	Unset(section, option string) error
	Snapshot() (string, error)
	Rollback(snap string) error
	Commit() error
}
