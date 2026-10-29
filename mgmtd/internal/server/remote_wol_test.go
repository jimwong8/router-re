package server

import (
	"os"
	"path/filepath"
	"testing"
)

// WOL 目标文件必须跟着配置走。曾经漏了 MGM_CONFIG_FILE：只设它时
// WOL 仍去写 /etc/router-re，普通用户 500 permission denied。
func TestWolTargetsPathFollowsConfigFile(t *testing.T) {
	t.Setenv("MGM_WOL_TARGETS", "")
	t.Setenv("MGM_CONFIG_DIR", "")
	t.Setenv("MGM_CONFIG_FILE", "/home/u/.router-re-dev/config.json")
	got := wolTargetsPath()
	want := filepath.Join("/home/u/.router-re-dev", "wol_targets.json")
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}

	// 显式 MGM_WOL_TARGETS 最高优先级
	t.Setenv("MGM_WOL_TARGETS", "/tmp/explicit.json")
	if got := wolTargetsPath(); got != "/tmp/explicit.json" {
		t.Fatalf("MGM_WOL_TARGETS 应最优先，got %s", got)
	}
}

func TestWolTargetsPathDefault(t *testing.T) {
	t.Setenv("MGM_WOL_TARGETS", "")
	t.Setenv("MGM_CONFIG_DIR", "")
	t.Setenv("MGM_CONFIG_FILE", "")
	if got := wolTargetsPath(); got != filepath.Join("/etc/router-re", "wol_targets.json") {
		t.Fatalf("缺省应回落 /etc/router-re，got %s", got)
	}
	_ = os.Getenv("HOME")
}
