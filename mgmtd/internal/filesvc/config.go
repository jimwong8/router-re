// Package filesvc 管理 FTP / Samba / HTTP 三个文件服务的配置与运行状态。
//
// 三者的共同点：都是把 /etc/disk_user 暴露出去的入口，配置里任何一个
// 路径写错，要么共享不到东西，要么把系统目录共享出去。所以这一层的
// 核心职责不是"把配置写到磁盘"，而是**在写之前把危险配置拒掉**：
// 根目录必须在存储根内，用户名不能带路径分隔符，端口不能撞已成服务的端口。
//
// 配置以文本渲染（vsftpd.conf / smb.conf 片段 / lighttpd.conf 片段），
// 渲染结果可以直接断言，不需要真的把服务装起来才能验证正确性。
package filesvc

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Kind 是三选一的服务类型。
type Kind string

const (
	KindFTP   Kind = "ftp"
	KindSamba Kind = "samba"
	KindHTTP  Kind = "http"
)

// StorageRoot 是所有共享根目录必须落在其内的前缀（与 storage 包一致）。
const StorageRoot = "/etc/disk_user"

// FTPConfig 对应 iKuai 的 FTP 服务页。
type FTPConfig struct {
	Enabled    bool   `json:"enabled"`
	Port       int    `json:"port"`
	Root       string `json:"root"`
	Anonymous  bool   `json:"anonymous"`
	User       string `json:"user"`
	Password   string `json:"password,omitempty"`
	MaxConn    int    `json:"max_conn"`
	SpeedLimit int    `json:"speed_limit"` // KB/s，0 = 不限
}

// SambaConfig 对应 SMB 共享页。
type SambaConfig struct {
	Enabled   bool   `json:"enabled"`
	Workgroup string `json:"workgroup"`
	ShareName string `json:"share_name"`
	Root      string `json:"root"`
	ReadOnly  bool   `json:"read_only"`
	User      string `json:"user"`
	Password  string `json:"password,omitempty"`
}

// HTTPConfig 对应 HTTP 文件服务页。
type HTTPConfig struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Root    string `json:"root"`
	Listing bool   `json:"listing"` // 是否列目录
}

// Defaults 返回三方服务的默认配置（默认关闭，端口用标准端口）。
func Defaults(kind Kind) interface{} {
	switch kind {
	case KindFTP:
		return FTPConfig{Enabled: false, Port: 21, Root: StorageRoot, MaxConn: 10}
	case KindSamba:
		return SambaConfig{Enabled: false, Workgroup: "WORKGROUP", ShareName: "disk", Root: StorageRoot}
	default:
		return HTTPConfig{Enabled: false, Port: 8088, Root: StorageRoot, Listing: true}
	}
}

// validPort 判断端口可用性。1-65535，且避开管理端口（9090 是本机 mgmtd，
// 22 是 SSH）——把文件服务配到管理端口上等于自己把自己踢下线。
func validPort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("端口必须在 1-65535 之间")
	}
	switch p {
	case 9090:
		return fmt.Errorf("端口 %d 是管理口，不能用于文件服务", p)
	case 22:
		return fmt.Errorf("端口 22 是 SSH，不能用于文件服务")
	case 53, 67, 68:
		return fmt.Errorf("端口 %d 是 DNS/DHCP 保留端口", p)
	}
	return nil
}

// validRoot 校验共享根目录。
//
// 这里不做 EvalSymlinks（配置文件渲染阶段拿不到运行时状态），
// 但要求路径必须是绝对路径、必须在 StorageRoot 内、不含 .. ——
// 真正的符号链接逃逸由 storage.FileManager 在运行时挡。
func validRoot(root string) error {
	if root == "" {
		return fmt.Errorf("共享目录不能为空")
	}
	if !strings.HasPrefix(root, "/") {
		return fmt.Errorf("共享目录必须是绝对路径")
	}
	for _, seg := range strings.Split(root, "/") {
		if seg == ".." {
			return fmt.Errorf("共享目录不能包含 ..")
		}
	}
	clean := filepath.Clean(root)
	if clean != StorageRoot && !strings.HasPrefix(clean, StorageRoot+"/") {
		return fmt.Errorf("共享目录必须位于 %s 之内（当前 %s）", StorageRoot, clean)
	}
	return nil
}

// validUser 校验用户名：Unix 用户名的保守子集，且不允许数字开头。
func validUser(name string) error {
	if name == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if len(name) > 32 {
		return fmt.Errorf("用户名最多 32 字节")
	}
	if name[0] >= '0' && name[0] <= '9' {
		return fmt.Errorf("用户名不能以数字开头")
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok {
			return fmt.Errorf("用户名只能包含字母、数字、下划线、短横线和点")
		}
	}
	return nil
}

// ValidateFTP 校验 FTP 配置。
func ValidateFTP(c FTPConfig) error {
	if err := validPort(c.Port); err != nil {
		return err
	}
	if err := validRoot(c.Root); err != nil {
		return err
	}
	if !c.Anonymous {
		if err := validUser(c.User); err != nil {
			return err
		}
		if c.Enabled && c.Password == "" {
			return fmt.Errorf("非匿名模式下必须设置密码")
		}
	}
	if c.MaxConn < 0 || c.MaxConn > 1000 {
		return fmt.Errorf("最大连接数必须在 0-1000 之间（0 = 不限）")
	}
	if c.SpeedLimit < 0 || c.SpeedLimit > 1048576 {
		return fmt.Errorf("限速必须在 0-1048576 KB/s 之间（0 = 不限）")
	}
	return nil
}

// ValidateSamba 校验 SMB 配置。
func ValidateSamba(c SambaConfig) error {
	if err := validRoot(c.Root); err != nil {
		return err
	}
	if c.Workgroup == "" || len(c.Workgroup) > 15 {
		return fmt.Errorf("工作组名必须 1-15 个字符")
	}
	for _, r := range c.ShareName {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok {
			return fmt.Errorf("共享名只能包含字母、数字、下划线、短横线和点")
		}
	}
	if c.ShareName == "" {
		return fmt.Errorf("共享名不能为空")
	}
	if !c.ReadOnly {
		if err := validUser(c.User); err != nil {
			return err
		}
		if c.Enabled && c.Password == "" {
			return fmt.Errorf("可写共享必须设置密码")
		}
	}
	return nil
}

// ValidateHTTP 校验 HTTP 文件服务配置。
func ValidateHTTP(c HTTPConfig) error {
	if err := validPort(c.Port); err != nil {
		return err
	}
	return validRoot(c.Root)
}

// RenderFTP 渲染 vsftpd.conf。密码不写进配置（vsftpd 用系统用户 + PAM）。
func RenderFTP(c FTPConfig) string {
	var b strings.Builder
	b.WriteString("# 由 mgmtd 生成，请勿手工编辑\n")
	if c.Enabled {
		b.WriteString("listen=YES\n")
	} else {
		b.WriteString("listen=NO\n")
	}
	fmt.Fprintf(&b, "listen_port=%d\n", c.Port)
	if c.Anonymous {
		b.WriteString("anonymous_enable=YES\n")
	} else {
		b.WriteString("anonymous_enable=NO\nlocal_enable=YES\n")
		fmt.Fprintf(&b, "local_root=%s\n", c.Root)
	}
	if !c.Anonymous && c.Root != "" {
		fmt.Fprintf(&b, "anon_root=%s\n", c.Root)
	}
	if c.MaxConn > 0 {
		fmt.Fprintf(&b, "max_clients=%d\n", c.MaxConn)
	}
	if c.SpeedLimit > 0 {
		fmt.Fprintf(&b, "local_max_rate=%d\n", c.SpeedLimit*1024)
	}
	b.WriteString("write_enable=YES\n")
	b.WriteString("chroot_local_user=YES\n")
	return b.String()
}

// RenderSamba 渲染 smb.conf 的一个共享段。
func RenderSamba(c SambaConfig) string {
	var b strings.Builder
	b.WriteString("# 由 mgmtd 生成，请勿手工编辑\n[global]\n")
	fmt.Fprintf(&b, "   workgroup = %s\n", c.Workgroup)
	b.WriteString("   server string = router-re file service\n")
	b.WriteString("   security = user\n   map to guest = Bad User\n\n")
	fmt.Fprintf(&b, "[%s]\n", c.ShareName)
	fmt.Fprintf(&b, "   path = %s\n", c.Root)
	fmt.Fprintf(&b, "   read only = %s\n", yesNo(c.ReadOnly))
	b.WriteString("   browseable = yes\n   create mask = 0644\n   directory mask = 0755\n")
	if c.User != "" && !c.ReadOnly {
		fmt.Fprintf(&b, "   valid users = %s\n", c.User)
	}
	return b.String()
}

// RenderHTTP 渲染一个 lighttpd 风格的静态文件服务片段。
func RenderHTTP(c HTTPConfig) string {
	var b strings.Builder
	b.WriteString("# 由 mgmtd 生成，请勿手工编辑\n")
	fmt.Fprintf(&b, "server.port = %d\n", c.Port)
	fmt.Fprintf(&b, "server.document-root = \"%s\"\n", c.Root)
	b.WriteString("server.modules += ( \"mod_dirlisting\", \"mod_staticfile\" )\n")
	fmt.Fprintf(&b, "dir-listing.activate = \"%s\"\n", enabled(c.Listing))
	b.WriteString("server.follow-symlink = \"disable\"\n")
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func enabled(v bool) string {
	if v {
		return "enable"
	}
	return "disable"
}

// ConfPath 返回各服务的配置文件路径。
func ConfPath(kind Kind) string {
	switch kind {
	case KindFTP:
		return "/etc/vsftpd.conf"
	case KindSamba:
		return "/etc/samba/smb.conf"
	default:
		return "/etc/lighttpd/lighttpd.conf"
	}
}

// BinaryName 返回该服务对应的可执行文件名，用来判断是否安装。
func BinaryName(kind Kind) string {
	switch kind {
	case KindFTP:
		return "vsftpd"
	case KindSamba:
		return "smbd"
	default:
		return "lighttpd"
	}
}
