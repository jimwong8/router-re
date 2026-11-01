package tailscale

import (
	"strings"
	"testing"
)

func TestBuildUpArgsFull(t *testing.T) {
	c := Config{
		Enabled: true, AuthKey: "tskey-auth-kABCDEF123456-abcdefghijklmnop",
		Hostname: "router-fcg", AdvertiseRoutes: []string{"192.168.1.0/24", " 10.0.0.0/24 "},
		AdvertiseExitNode: true, AcceptRoutes: true, AcceptDNS: false, SSH: true,
		LoginServer: "https://hs.example.com", ForceReauth: true,
	}
	got := BuildUpArgs(c)
	want := []string{
		"up",
		"--authkey=tskey-auth-kABCDEF123456-abcdefghijklmnop",
		"--hostname=router-fcg",
		"--advertise-routes=192.168.1.0/24,10.0.0.0/24",
		"--advertise-exit-node=true",
		"--accept-routes=true",
		"--accept-dns=false",
		"--ssh=true",
		"--login-server=https://hs.example.com",
		"--force-reauth",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("参数不符\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildUpArgsWithoutAuthKey(t *testing.T) {
	got := BuildUpArgs(Config{Enabled: true, AcceptDNS: true})
	for _, a := range got {
		if strings.HasPrefix(a, "--authkey=") {
			t.Fatalf("没有 authkey 时不应传 --authkey（否则会把已登录节点踢成需要重新授权）：%v", got)
		}
	}
	if got[0] != "up" {
		t.Fatalf("首个参数应是 up：%v", got)
	}
}

func TestMaskArgsHidesKey(t *testing.T) {
	got := MaskArgs([]string{"up", "--authkey=tskey-auth-xxx"})
	if got[1] != "--authkey=tskey-********" {
		t.Fatalf("authkey 未打码：%v", got)
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	base := Config{Enabled: true, AuthKey: "tskey-auth-kABCDEF123456-abcdefghijklmnop"}
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"主机名以连字符开头", func(c *Config) { c.Hostname = "-bad" }},
		{"主机名含下划线", func(c *Config) { c.Hostname = "bad_host" }},
		{"通告网段非 CIDR", func(c *Config) { c.AdvertiseRoutes = []string{"192.168.1.1"} }},
		{"通告网段是 IPv6", func(c *Config) { c.AdvertiseRoutes = []string{"fd00::/64"} }},
		{"控制面地址无协议", func(c *Config) { c.LoginServer = "hs.example.com" }},
		{"auth key 换行注入", func(c *Config) { c.AuthKey = "tskey-auth-aaaaaaaaaaaa\n--advertise-exit-node" }},
		{"auth key 过短", func(c *Config) { c.AuthKey = "tskey-short" }},
	}
	for _, tc := range cases {
		c := base
		tc.mut(&c)
		if err := Validate(c); err == nil {
			t.Fatalf("%s：应当被拒绝", tc.name)
		}
	}
}

func TestValidateAcceptsNormal(t *testing.T) {
	c := Config{
		Enabled: true, Hostname: "router-fcg", AdvertiseRoutes: []string{"192.168.1.0/24"},
		AcceptDNS: true, LoginServer: "https://hs.example.com",
	}
	if err := Validate(c); err != nil {
		t.Fatalf("合法配置被拒：%v", err)
	}
	// 未启用时允许保存草稿（含不完整 key 也不校验）
	if err := Validate(Config{Enabled: false}); err != nil {
		t.Fatalf("草稿被拒：%v", err)
	}
}

// 真实输出（本机 1.94.2，保留两个对端以缩短样例）
const realStatusJSON = `{
  "BackendState": "Running",
  "Version": "1.94.2-t0a29cf18b-g3f044c9f6",
  "AuthURL": "",
  "Health": ["Tailscale can't reach the configured DNS servers. Internet connectivity may be affected."],
  "MagicDNSSuffix": "taila1fc66.ts.net",
  "CurrentTailnet": {"Name": "jimwong8.github", "MagicDNSEnabled": true},
  "Self": {
    "ID": "nMAGVrVXxC21CNTRL", "HostName": "jimwong-standardpc",
    "DNSName": "jimwong-openclaw.taila1fc66.ts.net.",
    "OS": "linux", "Online": true, "ExitNode": false, "Relay": "sin",
    "TailscaleIPs": ["100.111.28.64", "fd7a:115c:a1e0::ea35:1c40"]
  },
  "Peer": {
    "nUCxG7FmY321CNTRL": {
      "ID": "nUCxG7FmY321CNTRL", "HostName": "hk87", "DNSName": "hk87.taila1fc66.ts.net.",
      "OS": "linux", "Online": true, "ExitNode": false, "Relay": "hkg",
      "TailscaleIPs": ["100.84.81.101"], "RxBytes": 0, "TxBytes": 0,
      "LastSeen": "0001-01-01T00:00:00Z"
    },
    "nZZZoffline321CNTRL": {
      "ID": "nZZZoffline321CNTRL", "HostName": "oldbox", "DNSName": "oldbox.taila1fc66.ts.net.",
      "OS": "windows", "Online": false, "ExitNode": false, "Relay": "",
      "TailscaleIPs": ["100.99.1.2"], "RxBytes": 10, "TxBytes": 20,
      "LastSeen": "2026-10-01T00:00:00Z"
    }
  }
}`

func TestParseStatusRealOutput(t *testing.T) {
	st, err := ParseStatus([]byte(realStatusJSON))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if st.BackendState != "Running" || st.Version == "" {
		t.Fatalf("基本字段错误：%+v", st)
	}
	if st.Tailnet != "jimwong8.github" || !st.MagicDNSEnabled {
		t.Fatalf("tailnet 字段错误：%+v", st)
	}
	if st.Self.Hostname != "jimwong-standardpc" || !st.Self.IsSelf || len(st.Self.IPs) != 2 {
		t.Fatalf("Self 解析错误：%+v", st.Self)
	}
	if st.PeerCount != 2 || st.OnlineCount != 1 {
		t.Fatalf("节点统计错误：count=%d online=%d", st.PeerCount, st.OnlineCount)
	}
	// 在线优先
	if !st.Peers[0].Online || st.Peers[0].Hostname != "hk87" {
		t.Fatalf("排序错误，在线节点应排前面：%+v", st.Peers)
	}
	if st.Peers[0].DNSName != "hk87.taila1fc66.ts.net" {
		t.Fatalf("DNSName 末尾的点未去掉：%q", st.Peers[0].DNSName)
	}
	if len(st.Health) != 1 {
		t.Fatalf("health 丢失：%+v", st.Health)
	}
}

func TestParseStatusNeedsLogin(t *testing.T) {
	st, err := ParseStatus([]byte(`{"BackendState":"NeedsLogin","AuthURL":"https://login.tailscale.com/a/abc123","Peer":null}`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if st.BackendState != "NeedsLogin" || st.AuthURL == "" {
		t.Fatalf("待授权状态解析错误：%+v", st)
	}
	if st.Peers == nil || len(st.Peers) != 0 {
		t.Fatalf("Peer 为 null 时应得到空切片而不是 nil：%+v", st.Peers)
	}
}

func TestParseStatusGarbage(t *testing.T) {
	if _, err := ParseStatus([]byte("tailscaled not running")); err == nil {
		t.Fatal("非 JSON 应当报错")
	}
}
