package filesvc

import (
	"strings"
	"testing"
)

// validRoot 的全部意义是"把共享目录关在 StorageRoot 里"，但它原来只查
// 绝对路径和 ".."，不查控制字符。filepath.Clean 原样保留换行，于是
//   /etc/disk_user/x\n   path = /etc
// 通过校验，渲染进 smb.conf 就是第二条 path —— samba 以最后一条为准，
// 共享目录直接变成 /etc。
func TestValidRootRejectsControlChars(t *testing.T) {
	payloads := []string{
		StorageRoot + "/x\n   path = /etc",
		StorageRoot + "/x\r\npath = /etc",
		StorageRoot + "/x\x00",
		StorageRoot + "/x\v",
		StorageRoot + "/x\"\nserver.document-root = \"/\"",
		StorageRoot + "/x'; rm -rf /",
		StorageRoot + "/x\\y",
	}
	for _, p := range payloads {
		if err := validRoot(p); err == nil {
			t.Errorf("Root=%q 未被拒绝", p)
		}
	}
}

func TestValidRootAcceptsLegit(t *testing.T) {
	for _, p := range []string{StorageRoot, StorageRoot + "/docs", StorageRoot + "/a/b-c_d.1"} {
		if err := validRoot(p); err != nil {
			t.Errorf("合法路径 %q 被误拒: %v", p, err)
		}
	}
}

// Workgroup 原来只查长度 1-15。"W\n[etc]\npath=/" 只有 14 字符，
// 能在 smb.conf 里开出一个指向 / 的新共享段。
func TestValidateSambaRejectsWorkgroupInjection(t *testing.T) {
	payloads := []string{
		"W\n[etc]\npath=/",
		"WORK\r\n[tmp]",
		"WG\x00",
		"WG;x",
		"WG x",
	}
	for _, wg := range payloads {
		c := SambaConfig{ShareName: "docs", Root: StorageRoot + "/docs", Workgroup: wg, ReadOnly: true}
		if err := ValidateSamba(c); err == nil {
			t.Errorf("Workgroup=%q 未被拒绝", wg)
		}
	}
}

func TestRenderSambaNoInjection(t *testing.T) {
	c := SambaConfig{
		ShareName: "docs",
		Root:      StorageRoot + "/x\n   path = /etc",
		Workgroup: "W\n[etc]\npath=/",
		ReadOnly:  true,
	}
	out := RenderSamba(c)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "path = /etc" {
			t.Fatalf("Root 注入出了独立的 path 行:\n%s", out)
		}
		if strings.TrimSpace(line) == "[etc]" {
			t.Fatalf("Workgroup 注入出了新的共享段:\n%s", out)
		}
	}
}

func TestRenderFTPNoInjection(t *testing.T) {
	c := FTPConfig{
		Enabled: true, Port: 21, MaxConn: 10, User: "svc", Password: "x",
		Root: StorageRoot + "/x\nlocal_root=/",
	}
	out := RenderFTP(c)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "local_root=/" {
			t.Fatalf("Root 注入出了独立的 local_root 行:\n%s", out)
		}
	}
}

func TestRenderHTTPNoInjection(t *testing.T) {
	c := HTTPConfig{Port: 8080, Root: StorageRoot + "/x\"\nserver.document-root = \"/\""}
	out := RenderHTTP(c)
	// 注入若成功会多出一条以 server.document-root 开头的独立配置行
	var n int
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "server.document-root") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("应只有 1 条 document-root 行，实际 %d:\n%s", n, out)
	}
}

func TestRenderSambaAcceptsLegit(t *testing.T) {
	c := SambaConfig{ShareName: "docs", Root: StorageRoot + "/docs", Workgroup: "WORKGROUP", ReadOnly: true}
	if err := ValidateSamba(c); err != nil {
		t.Fatalf("合法配置被误拒: %v", err)
	}
	out := RenderSamba(c)
	for _, want := range []string{"workgroup = WORKGROUP", "[docs]", "path = " + StorageRoot + "/docs"} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}
