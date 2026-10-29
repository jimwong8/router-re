package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/config"
)

// TestBrokenConfigStoreRefusesToStart 锁死一个「静默丢配置」的启动路径。
//
// newConfigStore 原来在 FileStore 打不开时 log 一行就退回 MockStore：
// 服务照常启动、所有配置写入返回 200、运行期间没有任何异常迹象，
// 重启后配置全部消失。运维只有在重启之后才会发现，而且会怀疑是别的改动。
//
// 现在：配了 MGM_CONFIG_FILE 却打不开 → InitError() 非空，main 拒绝启动。
// 想要「明知会丢也要跑」必须显式 MGM_ALLOW_EPHEMERAL_STORE=1。
func TestBrokenConfigStoreRefusesToStart(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(bad, []byte("{这不是 JSON"), 0o600); err != nil {
		t.Fatalf("写坏配置: %v", err)
	}

	t.Setenv("MGM_CONFIG_FILE", bad)
	t.Setenv("MGM_ALLOW_EPHEMERAL_STORE", "")

	s := New(Config{SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	if err := s.InitError(); err == nil {
		t.Fatal("配置文件是坏的，服务却认为可以启动——它会用内存存储跑，" +
			"所有配置变更返回成功、重启后全部消失，运行期间没有任何迹象")
	}

	// 正常文件不能误报
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`{"a.b":"1"}`), 0o600); err != nil {
		t.Fatalf("写正常配置: %v", err)
	}
	t.Setenv("MGM_CONFIG_FILE", good)
	s2 := New(Config{SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	if err := s2.InitError(); err != nil {
		t.Fatalf("正常配置文件被误判为不可用: %v", err)
	}

	// 显式放行仍然可以退化（这就是修复前无条件发生的行为）
	t.Setenv("MGM_CONFIG_FILE", bad)
	t.Setenv("MGM_ALLOW_EPHEMERAL_STORE", "1")
	s3 := New(Config{SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	if err := s3.InitError(); err != nil {
		t.Fatalf("显式设了 MGM_ALLOW_EPHEMERAL_STORE=1 仍拒绝启动: %v", err)
	}
	if _, ok := s3.store.(*config.FileStore); ok {
		t.Fatal("退化后应该是内存存储，却拿到了 FileStore")
	}
}
