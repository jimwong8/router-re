package filesvc

import (
	"strings"
	"testing"
)

func TestValidRoot(t *testing.T) {
	ok := []string{StorageRoot, "/etc/disk_user/photos", "/etc/disk_user/a/b"}
	for _, r := range ok {
		if err := validRoot(r); err != nil {
			t.Fatalf("%q 应通过: %v", r, err)
		}
	}
	bad := []string{
		"", "/etc", "/etc/", "/", "etc/disk_user", "/etc/disk_user/../..",
		"/etc/disk_userX", "/root", "/etc/disk_user/../../etc",
	}
	for _, r := range bad {
		if err := validRoot(r); err == nil {
			t.Fatalf("%q 必须被拒绝", r)
		}
	}
}

func TestValidPort(t *testing.T) {
	for _, p := range []int{1, 21, 2121, 8088, 65535} {
		if err := validPort(p); err != nil {
			t.Fatalf("端口 %d 应通过: %v", p, err)
		}
	}
	for _, p := range []int{0, -1, 65536, 99999, 9090, 22, 53} {
		if err := validPort(p); err == nil {
			t.Fatalf("端口 %d 必须被拒绝", p)
		}
	}
}

func TestValidUser(t *testing.T) {
	for _, u := range []string{"admin", "ftpuser", "a_b-c.d"} {
		if err := validUser(u); err != nil {
			t.Fatalf("%q 应通过: %v", u, err)
		}
	}
	for _, u := range []string{"", "1admin", "a/b", "a b", "a;b", strings.Repeat("x", 33)} {
		if err := validUser(u); err == nil {
			t.Fatalf("%q 必须被拒绝", u)
		}
	}
}

func TestValidateFTP(t *testing.T) {
	base := FTPConfig{Enabled: true, Port: 21, Root: StorageRoot, MaxConn: 10}
	anon := base
	anon.Anonymous = true
	if err := ValidateFTP(anon); err != nil {
		t.Fatalf("匿名模式不需要密码: %v", err)
	}

	// 非匿名 + 启用 + 无密码 → 拒
	noPw := base
	noPw.User = "ftpuser"
	if err := ValidateFTP(noPw); err == nil {
		t.Fatal("启用非匿名 FTP 但没有密码，必须拒绝")
	}
	withPw := noPw
	withPw.Password = "x"
	if err := ValidateFTP(withPw); err != nil {
		t.Fatalf("有密码应通过: %v", err)
	}

	// 关闭状态下不要求密码，便于先保存配置
	off := noPw
	off.Enabled = false
	if err := ValidateFTP(off); err != nil {
		t.Fatalf("未启用时允许空密码: %v", err)
	}

	badRoot := withPw
	badRoot.Root = "/etc"
	if err := ValidateFTP(badRoot); err == nil {
		t.Fatal("根目录越界必须拒绝")
	}

	badPort := withPw
	badPort.Port = 9090
	if err := ValidateFTP(badPort); err == nil {
		t.Fatal("管理口必须拒绝")
	}

	badConn := withPw
	badConn.MaxConn = 5000
	if err := ValidateFTP(badConn); err == nil {
		t.Fatal("超范围连接数必须拒绝")
	}
}

func TestValidateSamba(t *testing.T) {
	c := SambaConfig{Enabled: true, Workgroup: "WORKGROUP", ShareName: "disk", Root: StorageRoot}
	ro := c
	ro.ReadOnly = true
	if err := ValidateSamba(ro); err != nil {
		t.Fatalf("只读共享不需要用户: %v", err)
	}
	// 可写但无用户/密码
	if err := ValidateSamba(c); err == nil {
		t.Fatal("可写共享缺少用户必须拒绝")
	}
	c.User = "smbuser"
	c.Password = "p"
	if err := ValidateSamba(c); err != nil {
		t.Fatalf("完整配置应通过: %v", err)
	}
	c.ShareName = "a b"
	if err := ValidateSamba(c); err == nil {
		t.Fatal("共享名含空格必须拒绝")
	}
	c.ShareName = "disk"
	c.Workgroup = strings.Repeat("W", 16)
	if err := ValidateSamba(c); err == nil {
		t.Fatal("工作组超 15 字符必须拒绝")
	}
}

func TestValidateHTTP(t *testing.T) {
	c := HTTPConfig{Enabled: true, Port: 8088, Root: StorageRoot, Listing: true}
	if err := ValidateHTTP(c); err != nil {
		t.Fatalf("默认配置应通过: %v", err)
	}
	c.Port = 22
	if err := ValidateHTTP(c); err == nil {
		t.Fatal("SSH 端口必须拒绝")
	}
	c.Port = 8088
	c.Root = "/var/www"
	if err := ValidateHTTP(c); err == nil {
		t.Fatal("根目录越界必须拒绝")
	}
}

func TestRenderFTP(t *testing.T) {
	c := FTPConfig{Enabled: true, Port: 2121, Root: StorageRoot + "/media", User: "ftpuser", Password: "secret", MaxConn: 20, SpeedLimit: 512}
	out := RenderFTP(c)
	for _, want := range []string{
		"listen=YES", "listen_port=2121", "local_root=/etc/disk_user/media",
		"max_clients=20", "local_max_rate=524288", "chroot_local_user=YES",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("渲染结果缺少 %q\n%s", want, out)
		}
	}
	// 密码绝不能出现在配置文件里
	if strings.Contains(out, "secret") {
		t.Fatal("密码不能写进 vsftpd.conf")
	}
	if strings.Contains(out, "listen=NO") {
		t.Fatal("启用状态不应渲染 listen=NO")
	}
}

func TestRenderSamba(t *testing.T) {
	c := SambaConfig{Enabled: true, Workgroup: "HOME", ShareName: "disk", Root: StorageRoot, ReadOnly: false, User: "smbuser"}
	out := RenderSamba(c)
	for _, want := range []string{"workgroup = HOME", "[disk]", "path = /etc/disk_user", "read only = no", "valid users = smbuser"} {
		if !strings.Contains(out, want) {
			t.Fatalf("渲染结果缺少 %q\n%s", want, out)
		}
	}
	ro := c
	ro.ReadOnly = true
	ro.User = ""
	out2 := RenderSamba(ro)
	if !strings.Contains(out2, "read only = yes") {
		t.Fatalf("只读共享渲染错误\n%s", out2)
	}
	if strings.Contains(out2, "valid users") {
		t.Fatal("只读共享不应渲染 valid users")
	}
}

func TestRenderHTTP(t *testing.T) {
	c := HTTPConfig{Enabled: true, Port: 8088, Root: StorageRoot, Listing: false}
	out := RenderHTTP(c)
	for _, want := range []string{"server.port = 8088", `server.document-root = "/etc/disk_user"`, `dir-listing.activate = "disable"`, "follow-symlink"} {
		if !strings.Contains(out, want) {
			t.Fatalf("渲染结果缺少 %q\n%s", want, out)
		}
	}
	c.Listing = true
	if !strings.Contains(RenderHTTP(c), `dir-listing.activate = "enable"`) {
		t.Fatal("开启列目录时渲染错误")
	}
}

func TestConfPaths(t *testing.T) {
	if ConfPath(KindFTP) != "/etc/vsftpd.conf" {
		t.Fatal("FTP 配置路径错误")
	}
	if ConfPath(KindSamba) != "/etc/samba/smb.conf" {
		t.Fatal("Samba 配置路径错误")
	}
	if BinaryName(KindSamba) != "smbd" || BinaryName(KindFTP) != "vsftpd" {
		t.Fatal("二进制名错误")
	}
}
