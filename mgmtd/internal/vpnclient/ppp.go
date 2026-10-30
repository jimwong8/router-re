package vpnclient

import (
	"fmt"
	"regexp"
	"strings"
)

// PPTP / L2TP 拨号客户端。
//
// 两者最终都落到 pppd，差别只在外层：
//   PPTP  -> pty "pptp <server> --nolaunchpppd" + options.pptp
//   L2TP  -> xl2tpd 建 L2TP 隧道后启动 pppd + options.l2tpd.client
//
// 这里生成的是配置文件文本，不直接执行命令——方便单测，也让"生成"和
// "应用"两件事分开。
//
// 凭据一律写 /etc/ppp/chap-secrets，不写进 peers 文件：peers 文件常被
// 随手 cat 出来贴到工单里，chap-secrets 有 600 权限。

// PPPSpec 是一条 PPP 类拨号连接。
type PPPSpec struct {
	Name     string `json:"name"`   // 连接名，同时用作 peers 文件名
	Kind     string `json:"kind"`   // pptp / l2tp
	Server   string `json:"server"` // 对端地址
	Username string `json:"username"`
	Password string `json:"password"`
	// RouteAll 为 true 时下发 defaultroute 并替换默认路由；
	// false 时只加一条到对端网段的路由（split tunnel）。
	RouteAll bool   `json:"route_all"`
	MTU      int    `json:"mtu"`
	Comment  string `json:"comment"`
	Enabled  bool   `json:"enabled"`
}

// pppNameRe 限制连接名：它要成为文件名和 ipparam，不能带路径分隔符。
var pppNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)

// hostRe 校验服务器地址（域名或 IPv4）。故意不接受 IPv6 字面量：
// pppd/ptp 的地址参数在这里按空格分隔，冒号形式容易被解析歧义。
var hostRe = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)

// ValidatePPP 校验一条 PPP 拨号配置。
func ValidatePPP(s PPPSpec) error {
	if s.Kind != "pptp" && s.Kind != "l2tp" {
		return fmt.Errorf("类型只能是 pptp 或 l2tp")
	}
	if !pppNameRe.MatchString(s.Name) {
		return fmt.Errorf("连接名只能用字母、数字、点、下划线、短横线，以字母或数字开头，最长 32")
	}
	if s.Server == "" {
		return fmt.Errorf("服务器地址不能为空")
	}
	if !hostRe.MatchString(s.Server) || strings.Contains(s.Server, "..") {
		return fmt.Errorf("服务器地址 %q 不是合法的域名或 IPv4", s.Server)
	}
	if s.Username == "" {
		return fmt.Errorf("用户名不能为空")
	}
	// 用户名/密码会进 chap-secrets，格式是 "client server secret IP"，
	// 空格和换行会改变字段数量，等于改写配置文件结构。
	if err := noFieldBreak(s.Username, "用户名"); err != nil {
		return err
	}
	if s.Password == "" {
		return fmt.Errorf("密码不能为空")
	}
	if err := noFieldBreak(s.Password, "密码"); err != nil {
		return err
	}
	if len(s.Password) > 128 {
		return fmt.Errorf("密码过长（最多 128 字符）")
	}
	if s.MTU != 0 && (s.MTU < 576 || s.MTU > 1500) {
		return fmt.Errorf("MTU 必须在 576-1500 之间")
	}
	if err := noFieldBreak(s.Comment, "备注"); err != nil {
		return err
	}
	return nil
}

// noFieldBreak 拒绝会破坏配置文件字段结构的字符。
//
// 这是配置注入的入口：pppd 的 chap-secrets 是空白分行的，"user pass\n* * *"
// 会让后面的内容变成一条通配凭据，等于给任何对端一个合法账号。
func noFieldBreak(v, field string) error {
	if strings.ContainsAny(v, "\n\r\t") {
		return fmt.Errorf("%s不能包含换行或制表符", field)
	}
	// chap-secrets 用空白分隔字段，且不支持引号转义：含空格会让
	// "user pass" 变成两个字段，后面的 * 被当成 IP，认证直接失败。
	if strings.Contains(v, " ") {
		return fmt.Errorf("%s不能包含空格", field)
	}
	if strings.ContainsAny(v, "\"\\") {
		return fmt.Errorf("%s不能包含引号或反斜杠", field)
	}
	return nil
}

// ChapSecretLine 生成 chap-secrets 里的一行。
func ChapSecretLine(s PPPSpec) (string, error) {
	if err := ValidatePPP(s); err != nil {
		return "", err
	}
	remote := "*"
	if s.Kind == "pptp" {
		remote = "PPTP"
	} else {
		remote = "L2TP"
	}
	// 星号 IP 表示由服务端分配地址
	return fmt.Sprintf("%s\t%s\t%s\t*", s.Username, remote, s.Password), nil
}

// RenderPeers 生成 /etc/ppp/peers/<name> 的内容（不含密码）。
func RenderPeers(s PPPSpec) (string, error) {
	if err := ValidatePPP(s); err != nil {
		return "", err
	}
	var b strings.Builder
	if s.Kind == "pptp" {
		fmt.Fprintf(&b, "pty \"pptp %s --nolaunchpppd\"\n", s.Server)
		fmt.Fprintf(&b, "name %s\n", s.Username)
		b.WriteString("remotename PPTP\n")
		b.WriteString("file /etc/ppp/options.pptp\n")
	} else {
		// L2TP 的隧道由 xl2tpd 建，pppd 只连本地 l2tp 接口
		fmt.Fprintf(&b, "name %s\n", s.Username)
		b.WriteString("remotename L2TP\n")
		b.WriteString("file /etc/ppp/options.l2tpd.client\n")
	}
	fmt.Fprintf(&b, "ipparam %s\n", s.Name)
	b.WriteString("persist\n")
	b.WriteString("maxfail 0\n")
	b.WriteString("holdoff 10\n")
	if s.MTU > 0 {
		fmt.Fprintf(&b, "mtu %d\nmru %d\n", s.MTU, s.MTU)
	}
	if s.RouteAll {
		b.WriteString("defaultroute\nreplacedefaultroute\n")
	}
	b.WriteString("nopcomp\nnoaccomp\n")
	return b.String(), nil
}

// RenderXl2tpd 生成 L2TP 的 lac 段。只对 l2tp 有意义。
func RenderXl2tpd(s PPPSpec) (string, error) {
	if s.Kind != "l2tp" {
		return "", fmt.Errorf("只有 l2tp 需要 xl2tpd 配置")
	}
	if err := ValidatePPP(s); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[lac %s]\n", s.Name)
	fmt.Fprintf(&b, "lns = %s\n", s.Server)
	fmt.Fprintf(&b, "name = %s\n", s.Username)
	b.WriteString("ppp debug = no\n")
	b.WriteString("pppoptfile = /etc/ppp/peers/")
	b.WriteString(s.Name)
	b.WriteString("\n")
	b.WriteString("length bit = yes\n")
	// max redials = 0 表示不限次数；路由器上掉线要能自己重连
	b.WriteString("redial = yes\nredial timeout = 10\nmax redials = 0\n")
	return b.String(), nil
}

// ServiceName 返回 systemd 单元名。
func ServiceName(s PPPSpec) string {
	if s.Kind == "pptp" {
		return "rr-pptp-" + s.Name
	}
	return "rr-l2tp-" + s.Name
}

// SplitTunnelNote 说明 RouteAll=false 时的实际行为，供界面展示。
//
// 这里刻意把话说死：不做 split tunnel 路由计算的情况下，"只走对端网段"
// 需要用户自己填路由，系统不会猜对端推什么网段。
func SplitTunnelNote() string {
	return "未开启「全局走 VPN」时，系统只建立隧道与接口，不会自动添加网段路由——" +
		"对端推送的网段取决于其配置，需要你在静态路由里显式指定要走隧道的目标网段。"
}
