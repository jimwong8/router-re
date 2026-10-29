package config

import (
	"os"
	"path/filepath"
	"testing"
)

// 目录不可写时必须在启动阶段就报错。
// 曾经的现场：配置能正常读，跑到"生成卡密"才 500 save failed，
// 排查方向被引向 handler 而不是权限。
func TestNewFileStoreFailsFastOnReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "ro")
	if err := os.Mkdir(sub, 0o500); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root 无视目录权限，跳过")
	}
	if _, err := NewFileStore(filepath.Join(sub, "cfg.json")); err == nil {
		t.Fatal("只读目录应返回错误")
	} else if !contains(err.Error(), "不可写") {
		t.Fatalf("错误信息应指明目录不可写，got %v", err)
	}
}

// 正常可写目录不应被误杀。
func TestNewFileStoreOKOnWritableDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg.json")
	fs, err := NewFileStore(p)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := fs.Set("a", "b", "c"); err != nil {
		t.Fatalf("set: %v", err)
	}
	fs2, err := NewFileStore(p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if v, _ := fs2.Get("a", "b"); v != "c" {
		t.Fatalf("持久化丢失: %q", v)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
