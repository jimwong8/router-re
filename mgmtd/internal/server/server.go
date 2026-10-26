package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/auth"
	"github.com/jimwong/router-re/mgmtd/internal/config"
	"github.com/jimwong/router-re/mgmtd/internal/dhcp"
	"github.com/jimwong/router-re/mgmtd/internal/firewall"
	"github.com/jimwong/router-re/mgmtd/internal/nat"
	"github.com/jimwong/router-re/mgmtd/internal/qos"
	"github.com/jimwong/router-re/mgmtd/internal/system"
	"github.com/jimwong/router-re/mgmtd/internal/wan"
	"github.com/jimwong/router-re/mgmtd/internal/vpn"
)

type Config struct {
	Addr      string
	SecretKey []byte
}

type Server struct {
	cfg   Config
	mux   *http.ServeMux
	store config.Store
	tm    *auth.TokenManager
}

func New(cfg Config) *Server {
	s := &Server{
		cfg:   cfg,
		mux:   http.NewServeMux(),
		store: config.NewMockStore(),
		tm:    auth.NewTokenManager(cfg.SecretKey, 15*time.Minute),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/v1/system/info", s.auth(s.handleSystemInfo))
	s.mux.HandleFunc("GET /api/v1/qos/limits", s.auth(s.handleGetQoSLimits))
	s.mux.HandleFunc("POST /api/v1/qos/limits", s.auth(s.handleSetQoSLimits))
	s.mux.HandleFunc("GET /api/v1/interfaces", s.auth(s.handleInterfaces))
	s.mux.HandleFunc("GET /api/v1/firewall/rules", s.auth(s.handleGetFirewallRules))
	s.mux.HandleFunc("POST /api/v1/firewall/rules", s.auth(s.handleSetFirewallRules))
	s.mux.HandleFunc("GET /api/v1/nat/rules", s.auth(s.handleGetNATRules))
	s.mux.HandleFunc("POST /api/v1/nat/rules", s.auth(s.handleSetNATRules))
	s.mux.HandleFunc("GET /api/v1/wan/links", s.auth(s.handleGetWANLinks))
	s.mux.HandleFunc("POST /api/v1/wan/links", s.auth(s.handleSetWANLinks))
	s.mux.HandleFunc("GET /api/v1/dhcp/config", s.auth(s.handleGetDHCPConfig))
	s.mux.HandleFunc("POST /api/v1/dhcp/config", s.auth(s.handleSetDHCPConfig))
	s.mux.HandleFunc("POST /api/v1/vpn/wireguard/keypair", s.auth(s.handleWireGuardKeypair))
	s.mux.HandleFunc("POST /api/v1/vpn/wireguard/config", s.auth(s.handleWireGuardConfig))
}

// handleLogin authenticates a fixed dev account and issues a JWT session token.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// Dev default account; production reads from store/DB.
	if req.Username != "admin" || req.Password != "admin123" {
		writeJSON(w, 401, errBody(10001, "invalid credentials", ""))
		return
	}
	tok, err := s.tm.Issue(req.Username)
	if err != nil {
		writeJSON(w, 500, errBody(10000, "token issue fail", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{"token": tok})
}

// handleSystemInfo returns real host system information.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := system.GetInfo()
	if info.Hostname == "" {
		info.Hostname = "router"
	}
	writeJSON(w, 200, info)
}

// handleInterfaces returns the real list of network interfaces.
func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"interfaces": system.GetInterfaces(),
	})
}

// dhcpConfigFromStore 从 config.Store 读出一个 DHCPConfig。
func (s *Server) dhcpConfigFromStore() dhcp.DHCPConfig {
	get := func(opt string) string {
		v, _ := s.store.Get("dhcp", opt)
		return v
	}
	cfg := dhcp.DHCPConfig{
		Interface:  get("interface"),
		RangeStart: get("range_start"),
		RangeEnd:   get("range_end"),
		Netmask:    get("netmask"),
		Gateway:    get("gateway"),
		LeaseTime:  get("lease_time"),
	}
	// DNS 存为逗号分隔（store 只存字符串）
	if dns, _ := s.store.Get("dhcp", "dns"); dns != "" {
		cfg.DNS = strings.Split(dns, ",")
	}
	return cfg
}

// handleGetDHCPConfig returns the stored DHCP config plus its rendered dnsmasq text.
func (s *Server) handleGetDHCPConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.dhcpConfigFromStore()
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"dnsmasq": dhcp.RenderDnsmasq(cfg),
	})
}

// handleSetDHCPConfig stores the submitted DHCP config and returns rendered dnsmasq text.
func (s *Server) handleSetDHCPConfig(w http.ResponseWriter, r *http.Request) {
	var cfg dhcp.DHCPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	set := func(opt, val string) {
		if val != "" {
			s.store.Set("dhcp", opt, val)
		}
	}
	set("interface", cfg.Interface)
	set("range_start", cfg.RangeStart)
	set("range_end", cfg.RangeEnd)
	set("netmask", cfg.Netmask)
	set("gateway", cfg.Gateway)
	set("lease_time", cfg.LeaseTime)
	if len(cfg.DNS) > 0 {
		s.store.Set("dhcp", "dns", strings.Join(cfg.DNS, ","))
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"dnsmasq": dhcp.RenderDnsmasq(cfg),
	})
}

// handleGetFirewallRules returns the current firewall rules as rendered nftables.
func (s *Server) handleGetFirewallRules(w http.ResponseWriter, r *http.Request) {
	var rules []firewall.Rule
	for i := 0; ; i++ {
		chain, err := s.store.Get("firewall", itoa(i)+".chain")
		if err != nil || chain == "" {
			break
		}
		proto, _ := s.store.Get("firewall", itoa(i)+".proto")
		dport, _ := s.store.Get("firewall", itoa(i)+".dport")
		action, _ := s.store.Get("firewall", itoa(i)+".action")
		src, _ := s.store.Get("firewall", itoa(i)+".src")
		dst, _ := s.store.Get("firewall", itoa(i)+".dst")
		rules = append(rules, firewall.Rule{
			Chain: chain, Src: src, Dst: dst, Proto: proto, DPort: dport, Action: action,
		})
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": rules,
		"nft":   firewall.RenderNftables(rules),
	})
}

// handleSetFirewallRules stores the submitted rules in the store and applies
// them to the live nftables via Validate + Apply.
func (s *Server) handleSetFirewallRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []firewall.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// Validate syntax before storing
	if err := firewall.Validate(req.Rules); err != nil {
		writeJSON(w, 422, errBody(10010, "firewall validation failed", err.Error()))
		return
	}
	// Apply to live nftables
	if err := firewall.Apply(req.Rules); err != nil {
		writeJSON(w, 500, errBody(10011, "firewall apply failed", err.Error()))
		return
	}
	// Persist to store
	for i, rule := range req.Rules {
		key := itoa(i)
		s.store.Set("firewall", key+".chain", rule.Chain)
		s.store.Set("firewall", key+".src", rule.Src)
		s.store.Set("firewall", key+".dst", rule.Dst)
		s.store.Set("firewall", key+".proto", rule.Proto)
		s.store.Set("firewall", key+".dport", rule.DPort)
		s.store.Set("firewall", key+".action", rule.Action)
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": req.Rules,
		"nft":   firewall.RenderNftables(req.Rules),
	})
}

// handleGetNATRules returns the persisted NAT rules and their rendered nftables.
func (s *Server) handleGetNATRules(w http.ResponseWriter, r *http.Request) {
	var rules []nat.NATRule
	for i := 0; ; i++ {
		typ, err := s.store.Get("nat", itoa(i)+".type")
		if err != nil || typ == "" {
			break
		}
		iface, _ := s.store.Get("nat", itoa(i)+".interface")
		proto, _ := s.store.Get("nat", itoa(i)+".proto")
		srcport, _ := s.store.Get("nat", itoa(i)+".srcport")
		dstaddr, _ := s.store.Get("nat", itoa(i)+".dstaddr")
		dstport, _ := s.store.Get("nat", itoa(i)+".dstport")
		toport, _ := s.store.Get("nat", itoa(i)+".toport")
		rules = append(rules, nat.NATRule{
			Type: typ, Interface: iface, Proto: proto,
			SrcPort: srcport, DstAddr: dstaddr, DstPort: dstport, ToPort: toport,
		})
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": rules,
		"nft":   nat.RenderNftables(rules),
	})
}

// handleSetNATRules persists the submitted NAT rules in the store and returns
// the rendered nftables text.
func (s *Server) handleSetNATRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []nat.NATRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	for i, rule := range req.Rules {
		key := itoa(i)
		s.store.Set("nat", key+".type", rule.Type)
		s.store.Set("nat", key+".interface", rule.Interface)
		s.store.Set("nat", key+".proto", rule.Proto)
		s.store.Set("nat", key+".srcport", rule.SrcPort)
		s.store.Set("nat", key+".dstaddr", rule.DstAddr)
		s.store.Set("nat", key+".dstport", rule.DstPort)
		s.store.Set("nat", key+".toport", rule.ToPort)
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": req.Rules,
		"nft":   nat.RenderNftables(req.Rules),
	})
}

// handleGetWANLinks returns the persisted WAN links and their policy-routing
// commands.
func (s *Server) handleGetWANLinks(w http.ResponseWriter, r *http.Request) {
	var links []wan.WANLink
	for i := 0; ; i++ {
		name, err := s.store.Get("wan", itoa(i)+".name")
		if err != nil || name == "" {
			break
		}
		iface, _ := s.store.Get("wan", itoa(i)+".interface")
		gw, _ := s.store.Get("wan", itoa(i)+".gateway")
		weight, _ := s.store.Get("wan", itoa(i)+".weight")
		table, _ := s.store.Get("wan", itoa(i)+".table")
		ht, _ := s.store.Get("wan", itoa(i)+".health_target")
		wl := wan.WANLink{Name: name, Interface: iface, Gateway: gw, HealthTarget: ht}
		if weight != "" {
			fmt.Sscan(weight, &wl.Weight)
		}
		if table != "" {
			fmt.Sscan(table, &wl.Table)
		}
		links = append(links, wl)
	}
	writeJSON(w, 200, map[string]interface{}{
		"links":    links,
		"commands": wan.RenderPolicyRouting(links),
	})
}

// handleSetWANLinks persists the submitted WAN links and returns their
// policy-routing commands.
func (s *Server) handleSetWANLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Links []wan.WANLink `json:"links"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	for i, l := range req.Links {
		key := itoa(i)
		s.store.Set("wan", key+".name", l.Name)
		s.store.Set("wan", key+".interface", l.Interface)
		s.store.Set("wan", key+".gateway", l.Gateway)
		s.store.Set("wan", key+".weight", itoa(l.Weight))
		s.store.Set("wan", key+".table", itoa(l.Table))
		s.store.Set("wan", key+".health_target", l.HealthTarget)
	}
	writeJSON(w, 200, map[string]interface{}{
		"links":    req.Links,
		"commands": wan.RenderPolicyRouting(req.Links),
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleWireGuardKeypair 生成一对 WireGuard 密钥并返回。
// 私钥仅在响应中返回（一次），不写入日志。
func (s *Server) handleWireGuardKeypair(w http.ResponseWriter, r *http.Request) {
	priv, pub, err := vpn.GenerateKeypair()
	if err != nil {
		writeJSON(w, 500, errBody(10000, "wg keypair generate fail", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{
		"private_key": priv,
		"public_key":  pub,
	})
}

// handleWireGuardConfig 保存 WireGuard 配置并返回渲染的 wg0.conf 文本。
// 私钥写入 store 但绝不写入响应日志/输出（渲染文本含 PrivateKey 行是 wg0.conf 正常内容，
// 此处仅返回给调用方；错误路径不回显私钥）。
func (s *Server) handleWireGuardConfig(w http.ResponseWriter, r *http.Request) {
	var cfg vpn.WireGuardConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// 持久化到 config.Store（section=wg），私钥单独一个键
	if cfg.Interface != "" {
		s.store.Set("wg", "interface", cfg.Interface)
	}
	if cfg.Address != "" {
		s.store.Set("wg", "address", cfg.Address)
	}
	if cfg.ListenPort != 0 {
		s.store.Set("wg", "listen_port", itoa(cfg.ListenPort))
	}
	if cfg.PrivateKey != "" {
		s.store.Set("wg", "private_key", cfg.PrivateKey)
	}
	// 返回渲染文本（含 PrivateKey 是 wg0.conf 的正常内容，仅本次响应返回）
	writeJSON(w, 200, map[string]string{
		"config":  vpn.RenderConfig(cfg),
		"message": "wireguard config saved",
	})
}


// handleGetQoSLimits returns the current QoS limits and their tc/nft commands.
func (s *Server) handleGetQoSLimits(w http.ResponseWriter, r *http.Request) {
	var limits []qos.RateLimit
	for i := 0; ; i++ {
		iface, err := s.store.Get("qos", itoa(i)+".interface")
		if err != nil || iface == "" {
			break
		}
		rateStr, _ := s.store.Get("qos", itoa(i)+".rate")
		burstStr, _ := s.store.Get("qos", itoa(i)+".burst")
		dirStr, _ := s.store.Get("qos", itoa(i)+".direction")
		priorityStr, _ := s.store.Get("qos", itoa(i)+".priority")
		var rateMbps, burstKB, priority int
		fmt.Sscanf(rateStr, "%d", &rateMbps)
		fmt.Sscanf(burstStr, "%d", &burstKB)
		fmt.Sscanf(priorityStr, "%d", &priority)
		limits = append(limits, qos.RateLimit{
			Interface: iface, RateMbps: rateMbps, BurstKB: burstKB,
			Direction: dirStr, Priority: priority,
		})
	}
	writeJSON(w, 200, map[string]interface{}{
		"limits": limits,
		"tc":     qos.RenderTcCommands(limits),
		"nft":    qos.RenderNftLimit(limits),
	})
}

// handleSetQoSLimits validates, applies, and stores submitted QoS rules.
func (s *Server) handleSetQoSLimits(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Limits []qos.RateLimit `json:"limits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	for _, lim := range req.Limits {
		if lim.Interface == "" {
			writeJSON(w, 422, errBody(10020, "invalid qos rule", "interface required"))
			return
		}
	}
	if err := qos.Validate(req.Limits); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "permission") || strings.Contains(strings.ToLower(msg), "operation not permitted") {
			msg = "nft requires root privileges"
		}
		writeJSON(w, 422, errBody(10021, "qos validation failed", msg))
		return
	}
	if err := qos.Apply(req.Limits); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "permission") || strings.Contains(strings.ToLower(msg), "operation not permitted") {
			msg = "nft apply requires root privileges"
		}
		writeJSON(w, 500, errBody(10022, "qos apply failed", msg))
		return
	}
	for i, lim := range req.Limits {
		key := itoa(i)
		s.store.Set("qos", key+".interface", lim.Interface)
		s.store.Set("qos", key+".rate", fmt.Sprintf("%d", lim.RateMbps))
		s.store.Set("qos", key+".burst", fmt.Sprintf("%d", lim.BurstKB))
		s.store.Set("qos", key+".direction", lim.Direction)
		s.store.Set("qos", key+".priority", fmt.Sprintf("%d", lim.Priority))
	}
	writeJSON(w, 200, map[string]interface{}{
		"limits": req.Limits,
		"tc":     qos.RenderTcCommands(req.Limits),
		"nft":    qos.RenderNftLimit(req.Limits),
	})
}

func (s *Server) Run() error {
	return http.ListenAndServe(s.cfg.Addr, s.mux)
}

// auth is a Bearer-token middleware guarding all endpoints except health/login.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(h, "Bearer ")
		if tok == "" {
			writeJSON(w, 401, errBody(10003, "unauthorized", "missing token"))
			return
		}
		if _, err := s.tm.Verify(tok); err != nil {
			writeJSON(w, 401, errBody(10003, "unauthorized", "invalid token"))
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func errBody(code int, msg, detail string) map[string]string {
	return map[string]string{"code": itoa(code), "message": msg, "detail": detail}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

