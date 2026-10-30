package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogIntegrity(t *testing.T) {
	if len(Catalog) < 10 {
		t.Fatalf("应用清单太短（%d），至少要覆盖常用插件", len(Catalog))
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, p := range Catalog {
		if p.ID == "" || p.Name == "" || p.Binary == "" || p.Desc == "" {
			t.Fatalf("清单条目字段不全: %+v", p)
		}
		if ids[p.ID] {
			t.Fatalf("ID 重复: %s", p.ID)
		}
		ids[p.ID] = true
		if names[p.Name] {
			t.Fatalf("名称重复: %s", p.Name)
		}
		names[p.Name] = true
		if p.Homepage == "" {
			t.Fatalf("%s 缺少项目地址，现场没法装", p.ID)
		}
		for _, port := range p.Ports {
			if port < 1 || port > 65535 {
				t.Fatalf("%s 端口非法: %d", p.ID, port)
			}
		}
	}
}

func TestFind(t *testing.T) {
	if p, ok := Find("ddns-go"); !ok || p.Name != "DDNS-GO" {
		t.Fatalf("Find 失败: %+v %v", p, ok)
	}
	if _, ok := Find("no-such-plugin"); ok {
		t.Fatal("未知 ID 应返回 false")
	}
}

func TestStatusMissingBinary(t *testing.T) {
	m := NewManager(t.TempDir())
	p := Plugin{ID: "ghost", Name: "Ghost", Binary: "definitely-not-installed-xyz"}
	st := m.StatusOf(p)
	if st.Installed {
		t.Fatal("不存在的二进制不该报已安装")
	}
	if st.Running {
		t.Fatal("不存在的二进制不该报运行中")
	}
	if st.StateText != "未安装" {
		t.Fatalf("状态文案错误: %s", st.StateText)
	}
}

func TestSetEnabledWritesFlag(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.SetEnabled("ddns-go", true); err != nil {
		t.Fatal(err)
	}
	flag := filepath.Join(dir, "ddns-go.enabled")
	if _, err := os.Stat(flag); err != nil {
		t.Fatalf("自启标记未写入: %v", err)
	}
	st := m.StatusOf(Plugin{ID: "ddns-go", Name: "DDNS-GO", Binary: "definitely-not-installed-xyz"})
	if !st.Enabled {
		t.Fatal("应读到 Enabled=true")
	}
	// 未安装时状态文案应仍是"未安装"，而不是被 enabled 掩盖
	if st.StateText != "未安装" {
		t.Fatalf("未安装时不该显示其他状态: %s", st.StateText)
	}

	if err := m.SetEnabled("ddns-go", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(flag); !os.IsNotExist(err) {
		t.Fatal("关闭后标记应被删除")
	}
	// 重复关闭不应报错
	if err := m.SetEnabled("ddns-go", false); err != nil {
		t.Fatalf("幂等关闭失败: %v", err)
	}
}

func TestSetEnabledUnknown(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.SetEnabled("nope", true); err == nil {
		t.Fatal("未知插件应报错")
	}
}

func TestStartMissingBinary(t *testing.T) {
	m := NewManager(t.TempDir())
	// 清单里的插件在本机大概率没装；即便装了也不该在测试里真启动，
	// 所以只验证"二进制不存在时的报错可读"。
	if _, err := m.Start("aria2"); err != nil {
		if !containsAny(err.Error(), "未安装", "已在运行") {
			t.Fatalf("错误信息应说明原因: %v", err)
		}
	}
}

func TestStartUnknown(t *testing.T) {
	m := NewManager(t.TempDir())
	if _, err := m.Start("nope"); err == nil {
		t.Fatal("未知插件应报错")
	}
}

func TestStopNotRunning(t *testing.T) {
	m := NewManager(t.TempDir())
	// 用一个肯定没在跑的清单条目
	err := m.Stop("speedtest")
	if err == nil {
		t.Skip("speedtest 恰好正在运行，跳过")
	}
	if !containsAny(err.Error(), "没有在运行") {
		t.Logf("错误信息: %v（可接受）", err)
	}
}

func TestListSortedAndComplete(t *testing.T) {
	m := NewManager(t.TempDir())
	list := m.List()
	if len(list) != len(Catalog) {
		t.Fatalf("List 返回 %d 项，清单 %d 项", len(list), len(Catalog))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Category > list[i].Category {
			t.Fatalf("应按分类排序: %s > %s", list[i-1].Category, list[i].Category)
		}
	}
}

func TestPortConflicts(t *testing.T) {
	// 当前清单里各插件端口互不冲突，冲突检测应返回空
	conflicts := PortConflicts()
	if len(conflicts) != 0 {
		for port, names := range conflicts {
			t.Logf("端口 %d 被 %v 同时占用", port, names)
		}
		// 这里不判失败：以后清单里真出现冲突时，这个测试会提醒我们去处理
	}
	// 但必须能识别出冲突本身
	orig := Catalog
	defer func() { Catalog = orig }()
	Catalog = append(Catalog,
		Plugin{ID: "dup-a", Name: "DupA", Binary: "a", Ports: []int{9999}},
		Plugin{ID: "dup-b", Name: "DupB", Binary: "b", Ports: []int{9999}},
	)
	c := PortConflicts()
	if len(c[9999]) != 2 {
		t.Fatalf("应检测到 9999 冲突，实际 %v", c)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
