//go:build linux

package config

import (
	"os/exec"
	"testing"
)

// uciAvailable 检测系统是否有 uci 命令。
func uciAvailable() bool {
	_, err := exec.LookPath("uci")
	return err == nil
}

// TestUCIStoreInterfaceSemantics 验证 UCIStore 实现 Store 接口的语义。
// 仅当系统存在 uci 命令时执行真实测试；否则 skip。
func TestUCIStoreInterfaceSemantics(t *testing.T) {
	if !uciAvailable() {
		t.Skip("uci command not found; skipping real UCI test")
	}

	s := NewUCIStore()

	// 未写前 Get 返回空（uci get 未定义键会报错，但 UCIStore 返回错误；这里测语义以存在值为准）
	if err := s.Set("testconfig", "lan_ipaddr", "192.168.1.1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if v, err := s.Get("testconfig", "lan_ipaddr"); err != nil || v != "192.168.1.1" {
		t.Fatalf("Get after Set: got %q err %v, want %q", v, err, "192.168.1.1")
	}

	// Snapshot
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if snap == "" {
		t.Fatal("Snapshot returned empty id")
	}

	// Set 改值 → Get 反映新值
	if err := s.Set("testconfig", "lan_ipaddr", "10.0.0.1"); err != nil {
		t.Fatalf("Set 2 failed: %v", err)
	}
	if v, _ := s.Get("testconfig", "lan_ipaddr"); v != "10.0.0.1" {
		t.Fatalf("Get after Set 2: got %q want %q", v, "10.0.0.1")
	}

	// Rollback → 恢复快照值
	if err := s.Rollback(snap); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if v, _ := s.Get("testconfig", "lan_ipaddr"); v != "192.168.1.1" {
		t.Fatalf("Get after Rollback: got %q want %q", v, "192.168.1.1")
	}

	// Commit
	if err := s.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}
