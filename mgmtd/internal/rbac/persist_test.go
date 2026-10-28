package rbac

import (
	"os"
	"path/filepath"
	"testing"
)

// 账号表必须跨重启存活。
//
// 原来 Manager 只有内存 map：建出来的 viewer 账号重启后就登录不上，等于每次
// 重启清空一次多用户配置。这个用例盯的就是「重启后账号还在、密码还是改过的那个」。
func TestUsersSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")

	m1 := NewManagerWithFile("admin", "admin123", path)
	hash, err := HashPassword("Vtest12345!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := m1.Create(User{Username: "vt1", PasswordHash: hash, Role: RoleViewer, Enabled: true}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// 改 admin 密码：重启后不该被环境变量的初始密码盖回去
	if err := m1.ChangePassword("admin", "N3wAdminPass!"); err != nil {
		t.Fatalf("change admin password: %v", err)
	}

	// 「重启」= 用同一个文件重新构造 Manager
	m2 := NewManagerWithFile("admin", "admin123", path)

	if u, ok := m2.Get("vt1"); !ok {
		t.Fatal("重启后 vt1 不见了——账号表没落盘")
	} else if u.Role != RoleViewer {
		t.Errorf("重启后 role = %q，应 %q", u.Role, RoleViewer)
	} else if !VerifyPassword("Vtest12345!", u.PasswordHash) {
		t.Error("重启后 vt1 的密码校验失败")
	}

	if _, ok := m2.Authenticate("admin", "N3wAdminPass!"); !ok {
		t.Error("admin 改过的密码重启后失效——被环境变量初始密码覆盖了")
	}
	if _, ok := m2.Authenticate("admin", "admin123"); ok {
		t.Error("旧密码 admin123 重启后又能登录了")
	}
}

// 删除也要落盘，否则重启后被删的用户会复活（配合角色校验就是重新拿到权限）。
func TestDeletePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	m1 := NewManagerWithFile("admin", "admin123", path)
	hash, _ := HashPassword("Vtest12345!")
	if err := m1.Create(User{Username: "tmp1", PasswordHash: hash, Role: RoleViewer, Enabled: true}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m1.Delete("tmp1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	m2 := NewManagerWithFile("admin", "admin123", path)
	if _, ok := m2.Get("tmp1"); ok {
		t.Fatal("被删的账号重启后复活了")
	}
}

// 文件权限必须是 0600：里面有密码哈希。
func TestUserFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	m := NewManagerWithFile("admin", "admin123", path)
	hash, _ := HashPassword("Vtest12345!")
	if err := m.Create(User{Username: "vt2", PasswordHash: hash, Role: RoleViewer, Enabled: true}); err != nil {
		t.Fatalf("create: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("账号表权限 = %04o，应 0600（里面有密码哈希）", perm)
	}
}

// 文件损坏时不能当空表：否则所有非 admin 账号静默失效，且下次写盘会覆盖现场。
func TestCorruptFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	m := &Manager{users: map[string]User{}}
	if err := m.loadFromFile(path); err == nil {
		t.Fatal("坏文件应返回错误，实际当成了空表")
	}
}
