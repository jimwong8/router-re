package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Node 是一个 Tailscale 节点（本机或对端）。
type Node struct {
	ID        string   `json:"id"`
	Hostname  string   `json:"hostname"`
	DNSName   string   `json:"dns_name"`
	OS        string   `json:"os"`
	IPs       []string `json:"ips"`
	Online    bool     `json:"online"`
	Relay     string   `json:"relay"`
	ExitNode  bool     `json:"exit_node"`
	RxBytes   int64    `json:"rx_bytes"`
	TxBytes   int64    `json:"tx_bytes"`
	LastSeen  string   `json:"last_seen"`
	IsSelf    bool     `json:"is_self"`
}

// Status 是 `tailscale status --json` 整理后的结果。
type Status struct {
	Reachable       bool     `json:"reachable"` // CLI 能连上 tailscaled
	BackendState    string   `json:"backend_state"`
	Version         string   `json:"version"`
	Tailnet         string   `json:"tailnet"`
	MagicDNSSuffix  string   `json:"magic_dns_suffix"`
	MagicDNSEnabled bool     `json:"magic_dns_enabled"`
	Self            Node     `json:"self"`
	Peers           []Node   `json:"peers"`
	PeerCount       int      `json:"peer_count"`
	OnlineCount     int      `json:"online_count"`
	Health          []string `json:"health"`
	AuthURL         string   `json:"auth_url"`
	Socket          string   `json:"socket"`
	SocketExists    bool     `json:"socket_exists"`
	BinaryExists    bool     `json:"binary_exists"`
	Error           string   `json:"error"`
}

// 原始 JSON 结构（只声明用得到的字段）
type rawTailnet struct {
	Name            string `json:"Name"`
	MagicDNSEnabled bool   `json:"MagicDNSEnabled"`
}

type rawStatus struct {
	BackendState   string                     `json:"BackendState"`
	Version        string                     `json:"Version"`
	MagicDNSSuffix string                     `json:"MagicDNSSuffix"`
	AuthURL        string                     `json:"AuthURL"`
	Health         []string                   `json:"Health"`
	TailscaleIPs   []string                   `json:"TailscaleIPs"`
	CurrentTailnet *rawTailnet                `json:"CurrentTailnet"`
	Self           *rawNode                   `json:"Self"`
	Peer           map[string]json.RawMessage `json:"Peer"`
}

type rawNode struct {
	ID           string   `json:"ID"`
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	OS           string   `json:"OS"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
	Relay        string   `json:"Relay"`
	ExitNode     bool     `json:"ExitNode"`
	RxBytes      int64    `json:"RxBytes"`
	TxBytes      int64    `json:"TxBytes"`
	LastSeen     string   `json:"LastSeen"`
}

func (r rawNode) toNode(self bool) Node {
	return Node{
		ID: r.ID, Hostname: r.HostName, DNSName: strings.TrimSuffix(r.DNSName, "."),
		OS: r.OS, IPs: r.TailscaleIPs, Online: r.Online, Relay: r.Relay,
		ExitNode: r.ExitNode, RxBytes: r.RxBytes, TxBytes: r.TxBytes,
		LastSeen: r.LastSeen, IsSelf: self,
	}
}

// ParseStatus 解析 `tailscale status --json` 的输出。
func ParseStatus(data []byte) (Status, error) {
	var raw rawStatus
	if err := json.Unmarshal(data, &raw); err != nil {
		return Status{}, fmt.Errorf("解析 tailscale status 失败：%w", err)
	}
	st := Status{
		Reachable:    true,
		BackendState: raw.BackendState,
		Version:      raw.Version,
		MagicDNSSuffix: raw.MagicDNSSuffix,
		Health:       raw.Health,
		AuthURL:      raw.AuthURL,
		Peers:        make([]Node, 0, len(raw.Peer)),
	}
	if st.Health == nil {
		st.Health = []string{}
	}
	if raw.CurrentTailnet != nil {
		st.Tailnet = raw.CurrentTailnet.Name
		st.MagicDNSEnabled = raw.CurrentTailnet.MagicDNSEnabled
	}
	if raw.Self != nil {
		st.Self = raw.Self.toNode(true)
		if len(st.Self.IPs) == 0 {
			st.Self.IPs = raw.TailscaleIPs
		}
	}
	for _, v := range raw.Peer {
		var rn rawNode
		if err := json.Unmarshal(v, &rn); err != nil {
			continue
		}
		st.Peers = append(st.Peers, rn.toNode(false))
	}
	// 在线优先，其次按主机名，保证同一份数据每次展示顺序一致
	sort.SliceStable(st.Peers, func(i, j int) bool {
		if st.Peers[i].Online != st.Peers[j].Online {
			return st.Peers[i].Online
		}
		return strings.ToLower(st.Peers[i].Hostname) < strings.ToLower(st.Peers[j].Hostname)
	})
	st.PeerCount = len(st.Peers)
	for _, p := range st.Peers {
		if p.Online {
			st.OnlineCount++
		}
	}
	return st, nil
}

// run 执行 tailscale 子命令并带回 stdout/stderr。
func run(timeout time.Duration, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	full := append([]string{"--socket=" + SocketPath()}, args...)
	cmd := exec.CommandContext(ctx, BinaryPath(), full...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.String(), errb.String(), fmt.Errorf("tailscale %s 超时（%s）", strings.Join(args, " "), timeout)
	}
	return out.String(), errb.String(), err
}

// FetchStatus 拉取真实状态。CLI 或守护进程不可用时返回带 Error 的结果而不是报错，
// 这样界面能显示"守护进程没起来"，而不是一个空白的错误页。
func FetchStatus() Status {
	st := Status{
		Socket:       SocketPath(),
		SocketExists: SocketExists(),
		BinaryExists: BinaryAvailable(),
		Peers:        []Node{},
		Health:       []string{},
	}
	if !st.BinaryExists {
		st.Error = "找不到 tailscale 命令"
		return st
	}
	if !st.SocketExists {
		st.Error = "tailscaled 没有运行（socket 不存在：" + SocketPath() + "）"
		return st
	}
	out, errOut, err := run(8*time.Second, "status", "--json")
	if err != nil && strings.TrimSpace(out) == "" {
		st.Error = strings.TrimSpace(errOut)
		if st.Error == "" {
			st.Error = err.Error()
		}
		return st
	}
	parsed, perr := ParseStatus([]byte(out))
	if perr != nil {
		st.Error = perr.Error()
		return st
	}
	parsed.Socket = st.Socket
	parsed.SocketExists = true
	parsed.BinaryExists = true
	return parsed
}

// Apply 执行 up/down，并用真实 BackendState 判定是否成功。
//
// `tailscale up` 退出码 0 不代表成功：节点可能停在 NeedsLogin（需要点 AuthURL 授权）。
// 所以这里回读状态，只有 BackendState == Running 才算生效。
func Apply(c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	if !BinaryAvailable() {
		return fmt.Errorf("找不到 tailscale 命令")
	}
	if !SocketExists() {
		return fmt.Errorf("tailscaled 没有运行：%s 不存在", SocketPath())
	}
	if !c.Enabled {
		out, errOut, err := run(30*time.Second, "down")
		msg := strings.TrimSpace(out + errOut)
		if err != nil && !strings.Contains(strings.ToLower(msg), "already") {
			return fmt.Errorf("关闭 Tailscale 失败：%s", msg)
		}
		st := FetchStatus()
		if st.BackendState == "Running" {
			return fmt.Errorf("已执行 down，但状态仍是 Running")
		}
		return nil
	}

	out, errOut, err := run(90*time.Second, BuildUpArgs(c)...)
	combined := strings.TrimSpace(out + errOut)
	st := FetchStatus()
	if st.BackendState == "Running" {
		return nil
	}
	switch st.BackendState {
	case "NeedsLogin":
		if st.AuthURL != "" {
			return fmt.Errorf("节点需要授权：请打开 %s 完成登录后重试", st.AuthURL)
		}
		// 没有 AuthURL 一般是 key 无效/过期，把 CLI 原话带回去，否则用户只知道"需要授权"
		if combined != "" {
			return fmt.Errorf("节点需要授权（未返回授权链接，多半是 auth key 无效或已过期）：%s", firstLine(combined))
		}
		return fmt.Errorf("节点需要授权：auth key 无效或已过期，请重新生成")
	case "Stopped":
		if err != nil {
			return fmt.Errorf("tailscale up 失败：%s", combined)
		}
		return fmt.Errorf("tailscale up 返回成功但节点处于 Stopped：%s", combined)
	}
	if err != nil {
		return fmt.Errorf("tailscale up 失败（状态 %s）：%s", st.BackendState, combined)
	}
	return fmt.Errorf("tailscale up 后状态仍为 %s：%s", st.BackendState, combined)
}

// Down 是 Apply(disabled) 的简写。
func Down() error {
	c := Default()
	c.Enabled = false
	return Apply(c)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// firstLine 取多行输出的第一行非空内容，用于把 CLI 的报错塞进一句话里。
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
