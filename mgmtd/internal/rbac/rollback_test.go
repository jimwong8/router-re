package rbac

import (
	"os"
	"path/filepath"
	"testing"
)

// 用一个"目录"当落盘路径，persist 必然失败（无法把临时文件 rename 成目录）。
func badStorePath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "conf")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// 落盘失败时 Create 必须回滚内存：否则 API 报 500 但账号已在内存生效，
// 用户能登录能授权，直到重启才消失——状态与报错不一致。
func TestCreateRollsBackOnPersistFailure(t *testing.T) {
	m := NewManagerWithFile("admin", "admin123", badStorePath(t))
	if err := m.Create(User{Username: "bob", PasswordHash: "x", Role: RoleViewer, Enabled: true}); err == nil {
		t.Fatal("期望落盘失败，但 Create 返回 nil")
	}
	if _, ok := m.Get("bob"); ok {
		t.Fatal("落盘失败后 bob 仍在内存——未回滚")
	}
}

// Delete 落盘失败必须把用户放回去，不能"报错但人已被删"。
func TestDeleteRollsBackOnPersistFailure(t *testing.T) {
	m := NewManagerWithFile("admin", "admin123", filepath.Join(t.TempDir(), "users.json"))
	if err := m.Create(User{Username: "carol", PasswordHash: "x", Role: RoleViewer, Enabled: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 把落盘路径换成必然失败的目录
	m.file = badStorePath(t)
	if err := m.Delete("carol"); err == nil {
		t.Fatal("期望落盘失败，但 Delete 返回 nil")
	}
	if _, ok := m.Get("carol"); !ok {
		t.Fatal("落盘失败后 carol 被删了——未回滚")
	}
}

// SetPassword 落盘失败也不能让新密码在内存里生效。
func TestChangePasswordRollsBackOnPersistFailure(t *testing.T) {
	m := NewManagerWithFile("admin", "admin123", filepath.Join(t.TempDir(), "users.json"))
	before, _ := m.Get("admin")
	m.file = badStorePath(t)
	if err := m.ChangePassword("admin", "BrandNewPass123"); err == nil {
		t.Fatal("期望落盘失败，但 SetPassword 返回 nil")
	}
	after, _ := m.Get("admin")
	if after.PasswordHash != before.PasswordHash {
		t.Fatal("落盘失败后密码哈希已变——未回滚")
	}
}
