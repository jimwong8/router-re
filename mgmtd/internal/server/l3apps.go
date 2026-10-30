package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// 跨三层应用识别。
//
// 场景：终端在别的网段（另一个 VLAN、另一台三层交换机下面），本机看不到
// 它的二层流量，但要按应用去限速/管控。
//
// 这里做两件事：
//  1. 登记那些"隔着三层"的网段与它们的网关（本机靠路由把流量引过来）
//  2. 校验配置本身是否可行——网段写错、网关不在网段内、和本机直连网段
//     撞车，这些都会让策略静默失效，必须在保存时就拒绝
//
// 不做的事也写明：真正的应用识别要靠流经本机的流量，配置只是一个前提。

const maxL3Subnets = 32

// L3Subnet 是一个跨三层网段。
type L3Subnet struct {
	CIDR    string `json:"cidr"`
	Gateway string `json:"gateway"`
	Comment string `json:"comment,omitempty"`
	Enabled bool   `json:"enabled"`
}

// L3Config 是跨三层应用的完整配置。
type L3Config struct {
	Enabled bool       `json:"enabled"`
	Subnets []L3Subnet `json:"subnets"`
	// SniffMode 说明流量是怎么被引到本机的：
	// route = 本机有回程路由（推荐）；mirror = 上游做了端口镜像；
	// snmp = 只读拓扑信息，不做识别
	SniffMode string `json:"sniff_mode"`
}

// validateL3 校验配置。返回的错误要能直接展示给用户。
func validateL3(c L3Config, localNets []*net.IPNet) error {
	switch c.SniffMode {
	case "", "route", "mirror", "snmp":
	default:
		return fmt.Errorf("不支持的流量引取方式 %q（只接受 route / mirror / snmp）", c.SniffMode)
	}
	if c.Enabled && len(c.Subnets) == 0 {
		return fmt.Errorf("启用跨三层应用至少要登记一个网段")
	}
	if len(c.Subnets) > maxL3Subnets {
		return fmt.Errorf("最多登记 %d 个网段", maxL3Subnets)
	}
	seen := map[string]bool{}
	for i, sn := range c.Subnets {
		_, n, err := net.ParseCIDR(strings.TrimSpace(sn.CIDR))
		if err != nil {
			return fmt.Errorf("第 %d 个网段 %q 不是合法 CIDR（形如 192.168.10.0/24）", i+1, sn.CIDR)
		}
		key := n.String()
		if seen[key] {
			return fmt.Errorf("网段 %s 重复登记", key)
		}
		seen[key] = true

		if sn.Gateway != "" {
			gw := net.ParseIP(strings.TrimSpace(sn.Gateway))
			if gw == nil {
				return fmt.Errorf("第 %d 个网段的网关 %q 不是合法 IP", i+1, sn.Gateway)
			}
			if !n.Contains(gw) {
				return fmt.Errorf("网关 %s 不在网段 %s 内", sn.Gateway, key)
			}
		}
		for _, ln := range localNets {
			if ln.Contains(n.IP) {
				return fmt.Errorf("网段 %s 与本机直连网段 %s 重叠——直连的网段不需要登记为跨三层", key, ln.String())
			}
		}
	}
	return nil
}

// localNets 返回本机所有接口的 IPv4 网段（用于重叠检测）。
func localNets() []*net.IPNet {
	var out []*net.IPNet
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			out = append(out, ipnet)
		}
	}
	return out
}

func (s *Server) handleGetL3Apps(w http.ResponseWriter, r *http.Request) {
	cfg := L3Config{Enabled: false, Subnets: []L3Subnet{}, SniffMode: "route"}
	if raw, _ := s.store.Get("l3", "apps"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Subnets == nil {
		cfg.Subnets = []L3Subnet{}
	}
	var locals []string
	for _, n := range localNets() {
		locals = append(locals, n.String())
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":      cfg,
		"local_nets":  locals,
		"max_subnets": maxL3Subnets,
		"note": "跨三层识别的前提是这些网段的流量能到达本机（回程路由或上游镜像）。" +
			"只登记网段不会自动产生流量——没配路由时这里配得再对，应用识别也不会有数据。",
	})
}

func (s *Server) handleSetL3Apps(w http.ResponseWriter, r *http.Request) {
	var cfg L3Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(11700, "json decode fail", err.Error()))
		return
	}
	if err := validateL3(cfg, localNets()); err != nil {
		writeJSON(w, 422, errBody(11701, "l3 config rejected", err.Error()))
		return
	}
	if cfg.Subnets == nil {
		cfg.Subnets = []L3Subnet{}
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("l3", "apps", string(b))
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "saved": true})
}
