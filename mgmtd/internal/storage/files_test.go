package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newMgr(t *testing.T) (*FileManager, string) {
	t.Helper()
	root := t.TempDir()
	return NewFileManager(root), root
}

func TestListRootEmpty(t *testing.T) {
	m, _ := newMgr(t)
	got, err := m.List("/")
	if err != nil {
		t.Fatalf("列根目录失败: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("空目录应返回 0 项，实际 %d", len(got))
	}
}

func TestMkdirCreateRenameRemove(t *testing.T) {
	m, root := newMgr(t)
	if err := m.Mkdir("/", "photos"); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "photos")); err != nil {
		t.Fatalf("目录未落盘: %v", err)
	}
	if err := m.CreateFile("/photos", "a.txt"); err != nil {
		t.Fatalf("建文件失败: %v", err)
	}
	if err := m.Rename("/photos", "a.txt", "b.txt"); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "photos", "b.txt")); err != nil {
		t.Fatalf("改名后文件不存在: %v", err)
	}
	list, err := m.List("/photos")
	if err != nil || len(list) != 1 || list[0].Name != "b.txt" {
		t.Fatalf("列表不符: %v %+v", err, list)
	}
	if err := m.Remove("/photos/b.txt"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := m.Remove("/photos"); err != nil {
		t.Fatalf("删除目录失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "photos")); !os.IsNotExist(err) {
		t.Fatalf("目录应已删除: %v", err)
	}
}

func TestListSortsDirsFirst(t *testing.T) {
	m, _ := newMgr(t)
	_ = m.Mkdir("/", "zdir")
	_ = m.CreateFile("/", "afile")
	list, err := m.List("/")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("应有 2 项，实际 %d", len(list))
	}
	if !list[0].IsDir || list[0].Name != "zdir" {
		t.Fatalf("目录应排在最前: %+v", list)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	m, _ := newMgr(t)
	for _, p := range []string{
		"../../../etc/passwd",
		"/../../etc/passwd",
		"a/../../../../etc/shadow",
		"..",
	} {
		if _, err := m.List(p); err == nil {
			t.Fatalf("穿越路径 %q 应被拒绝", p)
		}
	}
	// 绝对路径会被当成根内相对路径，而不是真的去读 /etc/passwd
	if _, err := m.List("/etc"); !os.IsNotExist(err) && err == nil {
		t.Fatalf("/etc 应映射为根内的 etc（不存在），而不是系统 /etc")
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	m, root := newMgr(t)
	// 在根目录里放一个指向 /etc 的符号链接
	if err := os.Symlink("/etc", filepath.Join(root, "escape")); err != nil {
		t.Skipf("无法创建符号链接: %v", err)
	}
	if _, err := m.List("/escape"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("指向外部的符号链接应被拒绝，实际 err=%v", err)
	}
	// 经由符号链接新建文件同样要挡住
	if err := m.CreateFile("/escape", "pwn.txt"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("经符号链接写入应被拒绝，实际 err=%v", err)
	}
	if _, err := os.Stat("/etc/pwn.txt"); err == nil {
		t.Fatal("系统 /etc 下不该出现被写入的文件")
	}
}

func TestSymlinkInsideAllowed(t *testing.T) {
	m, root := newMgr(t)
	_ = m.Mkdir("/", "real")
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skipf("无法创建符号链接: %v", err)
	}
	// 指向根内部的符号链接是合法的
	if _, err := m.List("/link"); err != nil {
		t.Fatalf("指向根内的符号链接应允许: %v", err)
	}
}

func TestRemoveRootRejected(t *testing.T) {
	m, root := newMgr(t)
	if err := m.Remove("/"); !errors.Is(err, ErrRootAccess) {
		t.Fatalf("删除根目录应被拒绝，实际 err=%v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("根目录不应被删除: %v", err)
	}
}

func TestInvalidNames(t *testing.T) {
	m, _ := newMgr(t)
	_ = m.Mkdir("/", "d")
	bad := []string{"", ".", "..", "a/b", "a\\b", "x\x00y", "line\nbreak", strings.Repeat("a", 300)}
	for _, n := range bad {
		if err := m.Mkdir("/d", n); err == nil {
			t.Fatalf("非法名字 %q 应被拒绝", n)
		}
		if err := m.CreateFile("/d", n); err == nil {
			t.Fatalf("非法名字 %q 应被拒绝（文件）", n)
		}
	}
}

func TestDuplicatesRejected(t *testing.T) {
	m, _ := newMgr(t)
	_ = m.Mkdir("/", "one")
	if err := m.Mkdir("/", "one"); err == nil {
		t.Fatal("重复目录应报错")
	}
	_ = m.CreateFile("/", "f.txt")
	if err := m.CreateFile("/", "f.txt"); err == nil {
		t.Fatal("重复文件应报错")
	}
}

func TestMove(t *testing.T) {
	m, root := newMgr(t)
	_ = m.Mkdir("/", "src")
	_ = m.Mkdir("/", "dst")
	_ = m.CreateFile("/src", "x.txt")
	if err := m.Move("/src", "x.txt", "/dst"); err != nil {
		t.Fatalf("移动失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "x.txt")); err != nil {
		t.Fatalf("移动后文件不在目标目录: %v", err)
	}
	// 目标已有同名
	_ = m.CreateFile("/src", "x.txt")
	if err := m.Move("/src", "x.txt", "/dst"); err == nil {
		t.Fatal("目标重名应报错")
	}
	// 目录移入自身
	_ = m.Mkdir("/src", "inner")
	if err := m.Move("/", "src", "/src/inner"); err == nil {
		t.Fatal("把目录移进自身应报错")
	}
}

func TestStat(t *testing.T) {
	m, _ := newMgr(t)
	_ = m.CreateFile("/", "s.txt")
	e, err := m.Stat("/s.txt")
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "s.txt" || e.IsDir {
		t.Fatalf("Stat 结果不符: %+v", e)
	}
	if _, err := m.Stat("/nope"); err == nil {
		t.Fatal("不存在的路径应报错")
	}
}
