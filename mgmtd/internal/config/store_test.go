package config

import "testing"

// TestSnapshotRollback 验证基本快照/回滚语义：
// Set → Snapshot → 修改 → Rollback → 恢复到快照值。
func TestSnapshotRollback(t *testing.T) {
	s := NewMockStore()
	s.Set("network", "lan_ipaddr", "192.168.1.1")
	snap, _ := s.Snapshot()
	s.Set("network", "lan_ipaddr", "10.0.0.1")
	if v, _ := s.Get("network", "lan_ipaddr"); v != "10.0.0.1" {
		t.Fatalf("set failed: %s", v)
	}
	s.Rollback(snap)
	if v, _ := s.Get("network", "lan_ipaddr"); v != "192.168.1.1" {
		t.Fatalf("rollback failed: %s", v)
	}
}

// TestRollbackNoSnapshot 验证回滚一个不存在的快照标识符不会破坏当前数据。
func TestRollbackNoSnapshot(t *testing.T) {
	s := NewMockStore()
	s.Set("network", "lan_ipaddr", "1.1.1.1")
	// 不存在的快照
	if err := s.Rollback("nonexistent"); err != nil {
		t.Fatalf("rollback nonexistent should not error: %v", err)
	}
	if v, _ := s.Get("network", "lan_ipaddr"); v != "1.1.1.1" {
		t.Fatalf("data corrupted by rollback of nonexistent snap: %s", v)
	}
}

// TestMultipleSnapshots 验证多次快照互不干扰，可回滚到任意中间状态。
func TestMultipleSnapshots(t *testing.T) {
	s := NewMockStore()
	for i := 0; i < 3; i++ {
		if err := s.Set("network", "lan_ipaddr", string(rune('0'+i))); err != nil {
			t.Fatalf("set %d failed: %v", i, err)
		}
		if _, err := s.Snapshot(); err != nil {
			t.Fatalf("snapshot %d failed: %v", i, err)
		}
	}
	s.Set("network", "lan_ipaddr", "final")
	// 回滚到第 2 个快照（值为 '2'）
	// 取第 2 次快照的 ID：seq 是 3
	s.Rollback("3")
	if v, _ := s.Get("network", "lan_ipaddr"); v != "2" {
		t.Fatalf("rollback to snap 3 failed, got %q want %q", v, "2")
	}
}

// TestSnapshotIsIsolated 验证快照是数据的副本，后续修改不影响已保存的快照。
func TestSnapshotIsIsolated(t *testing.T) {
	s := NewMockStore()
	s.Set("network", "lan_ipaddr", "192.168.1.1")
	snap, _ := s.Snapshot()
	// 快照后再写，快照本身不应变
	s.Set("network", "lan_ipaddr", "10.0.0.1")
	s.Set("network", "lan_netmask", "255.255.255.0")
	v, _ := s.Get("network", "lan_ipaddr")
	if v != "10.0.0.1" {
		t.Fatalf("set after snapshot failed: %s", v)
	}
	s.Rollback(snap)
	// 回滚后快照值恢复，且快照中不存在的 key 消失
	if v, _ := s.Get("network", "lan_ipaddr"); v != "192.168.1.1" {
		t.Fatalf("rollback to 192.168.1.1 failed: %s", v)
	}
	if v, _ := s.Get("network", "lan_netmask"); v != "" {
		t.Fatalf("rollback should drop post-snapshot keys, got %q", v)
	}
}

// TestRollbackRestoresAllKeys 验证回滚完整恢复整份配置（多键）。
func TestRollbackRestoresAllKeys(t *testing.T) {
	s := NewMockStore()
	s.Set("network", "lan_ipaddr", "192.168.1.1")
	s.Set("network", "lan_netmask", "255.255.255.0")
	s.Set("wireless", "ssid", "MyRouter")
	snap, _ := s.Snapshot()
	// 改 + 增
	s.Set("network", "lan_ipaddr", "10.0.0.1")
	s.Set("wireless", "password", "secret")
	s.Rollback(snap)
	checks := map[string]string{
		"network.lan_ipaddr":  "192.168.1.1",
		"network.lan_netmask": "255.255.255.0",
		"wireless.ssid":       "MyRouter",
		"wireless.password":   "",
	}
	for k, want := range checks {
		var got string
		switch k {
		case "network.lan_ipaddr":
			got, _ = s.Get("network", "lan_ipaddr")
		case "network.lan_netmask":
			got, _ = s.Get("network", "lan_netmask")
		case "wireless.ssid":
			got, _ = s.Get("wireless", "ssid")
		case "wireless.password":
			got, _ = s.Get("wireless", "password")
		}
		if got != want {
			t.Fatalf("key %s: got %q want %q", k, got, want)
		}
	}
}

// TestCommitNoop 验证 Commit 可调用且不影响数据。
func TestCommitNoop(t *testing.T) {
	s := NewMockStore()
	s.Set("network", "lan_ipaddr", "192.168.1.1")
	if err := s.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}
	if v, _ := s.Get("network", "lan_ipaddr"); v != "192.168.1.1" {
		t.Fatalf("commit corrupted data: %s", v)
	}
}
