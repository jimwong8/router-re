package config

import "testing"

// Export → Restore 往返必须幂等。若键在往返中自我叠加
// （alerts.alerts/rules → alerts.alerts/rules.alerts.alerts/rules），
// 每次恢复备份都会让配置文件膨胀一倍。
func TestExportRestoreRoundTripIsIdempotent(t *testing.T) {
	fs, err := NewFileStore(t.TempDir() + "/c.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Set("alerts", "alerts/rules", `[{"a":1}]`); err != nil {
		t.Fatal(err)
	}
	if err := fs.Set("wechat", "config", `{"appid":"wx"}`); err != nil {
		t.Fatal(err)
	}
	first := fs.Export()
	if len(first) != 2 {
		t.Fatalf("期望 2 个键，got %d: %v", len(first), first)
	}
	// 模拟 restore：按 Export 的键拆回 (section, option)
	for ns, raw := range first {
		sec, opt := splitKey(ns)
		if err := fs.Set(sec, opt, raw); err != nil {
			t.Fatal(err)
		}
	}
	second := fs.Export()
	if len(second) != len(first) {
		t.Fatalf("往返后键数变了: %d → %d\nbefore=%v\nafter=%v", len(first), len(second), first, second)
	}
	for k := range first {
		if _, ok := second[k]; !ok {
			t.Fatalf("键在往返中变成别的名字: %q 不见了，现在是 %v", k, second)
		}
	}
}

// 与 p10.go 中恢复逻辑一致：按第一个 '.' 拆分。
func splitKey(ns string) (string, string) {
	for i := 0; i < len(ns); i++ {
		if ns[i] == '.' {
			o := ns[i+1:]
			if o == "" {
				o = "config"
			}
			return ns[:i], o
		}
	}
	return ns, "config"
}
