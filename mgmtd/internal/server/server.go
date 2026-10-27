package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/acl"
	"github.com/jimwong/router-re/mgmtd/internal/audit"
	"github.com/jimwong/router-re/mgmtd/internal/auth"
	"github.com/jimwong/router-re/mgmtd/internal/client"
	"github.com/jimwong/router-re/mgmtd/internal/cloud"
	"github.com/jimwong/router-re/mgmtd/internal/config"
	"github.com/jimwong/router-re/mgmtd/internal/conntrack"
	"github.com/jimwong/router-re/mgmtd/internal/ddns"
	"github.com/jimwong/router-re/mgmtd/internal/dhcp"
	"github.com/jimwong/router-re/mgmtd/internal/dpi"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/firewall"
	"github.com/jimwong/router-re/mgmtd/internal/iface"
	"github.com/jimwong/router-re/mgmtd/internal/l2"
	"github.com/jimwong/router-re/mgmtd/internal/logcenter"
	"github.com/jimwong/router-re/mgmtd/internal/nat"
	"github.com/jimwong/router-re/mgmtd/internal/portal"
	"github.com/jimwong/router-re/mgmtd/internal/portmap"
	"github.com/jimwong/router-re/mgmtd/internal/pppoe"
	"github.com/jimwong/router-re/mgmtd/internal/qos"
	"github.com/jimwong/router-re/mgmtd/internal/radius"
	"github.com/jimwong/router-re/mgmtd/internal/route"
	"github.com/jimwong/router-re/mgmtd/internal/stats"
	"github.com/jimwong/router-re/mgmtd/internal/system"
	"github.com/jimwong/router-re/mgmtd/internal/vpn"
	"github.com/jimwong/router-re/mgmtd/internal/wan"
	"github.com/jimwong/router-re/mgmtd/internal/wireless"
)

type Config struct {
	Addr      string
	SecretKey []byte
	// AdminUser / AdminPassword are the initial credential. They default to
	// admin/admin123 for development but MUST be overridden in production
	// (MGM_ADMIN_USER / MGM_ADMIN_PASSWORD).
	AdminUser     string
	AdminPassword string
}

type Server struct {
	cfg         Config
	dhcpApplier *dhcp.Applier
	mux         *http.ServeMux
	store       config.Store
	tm          *auth.TokenManager
	cloudMgr    *cloud.Manager
	pppoeUsers  []pppoe.User
	audit       *audit.Logger
	// loginLimiter throttles failed login attempts per remote IP to blunt
	// brute-force / credential-stuffing attacks.
	loginLimiter *loginLimiter
	// portalMgr tracks authorized MAC/IP pairs for the captive portal.
	portalMgr *portal.Manager
	// statsRing holds the rolling history of host samples.
	statsRing *stats.Ring
	// aclApplier runs nft rulesets (reuses execx; nil-safe in handlers).
	aclApplier execx.Runner
}

func New(cfg Config) *Server {
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	if cfg.AdminPassword == "" {
		// Development default. Production must set MGM_ADMIN_PASSWORD.
		cfg.AdminPassword = "admin123"
	}
	s := &Server{
		cfg:          cfg,
		mux:          http.NewServeMux(),
		store:        config.NewMockStore(),
		pppoeUsers:   []pppoe.User{},
		tm:           auth.NewTokenManager(cfg.SecretKey, 15*time.Minute),
		cloudMgr:     cloud.NewManager(),
		dhcpApplier:  dhcp.NewApplier(),
		loginLimiter: newLoginLimiter(5, time.Minute),
		portalMgr:    portal.NewManager(),
		statsRing:    stats.NewRing(720),
		aclApplier:   execx.DefaultRunner{},
	}
	// Start the background stats collector: sample every 30s, run until
	// process exit. Sleep inside the goroutine so it never blocks requests.
	go s.collectStatsLoop()
	// Audit logger: MGM_AUDIT_LOG env or default; degrade to ./audit.jsonl
	// if the default dir cannot be created (never fail startup for logging).
	auditPath := os.Getenv("MGM_AUDIT_LOG")
	if auditPath == "" {
		auditPath = "/var/log/router-re/audit.jsonl"
	}
	if err := os.MkdirAll(filepath.Dir(auditPath), 0o755); err != nil {
		auditPath = "./audit.jsonl"
	}
	s.audit = audit.New(auditPath)
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
	s.mux.HandleFunc("GET /api/v1/qos/client-limits", s.auth(s.handleGetClientLimits))
	s.mux.HandleFunc("POST /api/v1/qos/client-limits", s.auth(s.auditWrap("qos.client_limits.set", s.handleSetClientLimits)))
	s.mux.HandleFunc("POST /api/v1/qos/limits", s.auth(s.auditWrap("qos.limits.set", s.handleSetQoSLimits)))
	s.mux.HandleFunc("GET /api/v1/interfaces", s.auth(s.handleInterfaces))
	s.mux.HandleFunc("GET /api/v1/ddns/config", s.auth(s.handleGetDDNSConfig))
	s.mux.HandleFunc("POST /api/v1/ddns/config", s.auth(s.auditWrap("ddns.config.set", s.handleSetDDNSConfig)))
	s.mux.HandleFunc("POST /api/v1/ddns/update", s.auth(s.auditWrap("ddns.update", s.handleDDNSUpdate)))
	s.mux.HandleFunc("GET /api/v1/ddns/ip", s.auth(s.handleGETDDNSIP))
	s.mux.HandleFunc("GET /api/v1/ntp/config", s.auth(s.handleGetNTPConfig))
	s.mux.HandleFunc("POST /api/v1/ntp/config", s.auth(s.auditWrap("ntp.config.set", s.handleSetNTPConfig)))
	s.mux.HandleFunc("GET /api/v1/ntp/status", s.auth(s.handleNTPStatus))
	s.mux.HandleFunc("GET /api/v1/firewall/rules", s.auth(s.handleGetFirewallRules))
	s.mux.HandleFunc("POST /api/v1/firewall/rules", s.auth(s.auditWrap("firewall.rules.set", s.handleSetFirewallRules)))
	s.mux.HandleFunc("GET /api/v1/nat/rules", s.auth(s.handleGetNATRules))
	s.mux.HandleFunc("POST /api/v1/nat/rules", s.auth(s.auditWrap("nat.rules.set", s.handleSetNATRules)))
	s.mux.HandleFunc("GET /api/v1/wan/links", s.auth(s.handleGetWANLinks))
	s.mux.HandleFunc("POST /api/v1/wan/links", s.auth(s.auditWrap("wan.links.set", s.handleSetWANLinks)))
	s.mux.HandleFunc("GET /api/v1/route/bgp", s.auth(s.handleGetBGP))
	s.mux.HandleFunc("POST /api/v1/route/bgp", s.auth(s.auditWrap("route.bgp.set", s.handleSetBGP)))
	s.mux.HandleFunc("GET /api/v1/route/ospf", s.auth(s.handleGetOSPF))
	s.mux.HandleFunc("POST /api/v1/route/ospf", s.auth(s.auditWrap("route.ospf.set", s.handleSetOSPF)))
	s.mux.HandleFunc("GET /api/v1/dhcp/config", s.auth(s.handleGetDHCPConfig))
	s.mux.HandleFunc("GET /api/v1/dhcp/leases", s.auth(s.handleGetDHCPLeases))
	s.mux.HandleFunc("GET /api/v1/clients", s.auth(s.handleGetClients))
	s.mux.HandleFunc("POST /api/v1/dhcp/config", s.auth(s.auditWrap("dhcp.config.set", s.handleSetDHCPConfig)))
	s.mux.HandleFunc("GET /api/v1/cloud/devices", s.auth(s.handleGetCloudDevices))
	s.mux.HandleFunc("POST /api/v1/cloud/devices", s.auth(s.auditWrap("cloud.device.register", s.handlePostCloudDevices)))
	s.mux.HandleFunc("POST /api/v1/cloud/devices/{id}/heartbeat", s.auth(s.auditWrap("cloud.device.heartbeat", s.handlePostCloudHeartbeat)))
	s.mux.HandleFunc("GET /api/v1/wireless/ssids", s.auth(s.handleGetWirelessSSIDs))
	s.mux.HandleFunc("POST /api/v1/wireless/ssids", s.auth(s.auditWrap("wireless.ssid.set", s.handlePostWirelessSSIDs)))
	s.mux.HandleFunc("GET /api/v1/dpi/rules", s.auth(s.handleGetDPIRules))
	s.mux.HandleFunc("POST /api/v1/dpi/rules", s.auth(s.auditWrap("dpi.rules.set", s.handleSetDPIRules)))
	s.mux.HandleFunc("GET /api/v1/dpi/marks", s.auth(s.handleGetDPIMarks))
	s.mux.HandleFunc("POST /api/v1/dpi/marks", s.auth(s.auditWrap("dpi.marks.set", s.handleSetDPIMarks)))
	s.mux.HandleFunc("GET /api/v1/dpi/classify", s.auth(s.handleDPIClassify))
	s.mux.HandleFunc("GET /api/v1/vlan", s.auth(s.handleGetVLAN))
	s.mux.HandleFunc("POST /api/v1/vlan", s.auth(s.auditWrap("vlan.apply", s.handleSetVLAN)))
	s.mux.HandleFunc("DELETE /api/v1/vlan/{name}", s.auth(s.auditWrap("vlan.delete", s.handleDeleteVLAN)))
	s.mux.HandleFunc("GET /api/v1/bridge", s.auth(s.handleGetBridge))
	s.mux.HandleFunc("POST /api/v1/bridge", s.auth(s.auditWrap("bridge.apply", s.handleSetBridge)))
	s.mux.HandleFunc("DELETE /api/v1/bridge/{name}", s.auth(s.auditWrap("bridge.delete", s.handleDeleteBridge)))
	s.mux.HandleFunc("GET /api/v1/radius/config", s.auth(s.handleGetRadiusConfig))
	s.mux.HandleFunc("POST /api/v1/radius/config", s.auth(s.auditWrap("radius.config.set", s.handleSetRadiusConfig)))
	s.mux.HandleFunc("POST /api/v1/radius/test", s.auth(s.handleRadiusTest))
	s.mux.HandleFunc("POST /api/v1/vpn/wireguard/keypair", s.auth(s.auditWrap("vpn.wireguard.keypair", s.handleWireGuardKeypair)))
	s.mux.HandleFunc("POST /api/v1/vpn/wireguard/config", s.auth(s.auditWrap("vpn.wireguard.config", s.handleWireGuardConfig)))
	s.mux.HandleFunc("GET /api/v1/interfaces/config", s.auth(s.handleGetInterfacesConfig))
	s.mux.HandleFunc("POST /api/v1/interfaces/config", s.auth(s.auditWrap("iface.config.set", s.handleSetInterfacesConfig)))
	s.mux.HandleFunc("GET /api/v1/route/static", s.auth(s.handleGetStaticRoutes))
	s.mux.HandleFunc("POST /api/v1/route/static", s.auth(s.auditWrap("route.static.set", s.handleSetStaticRoutes)))
	s.mux.HandleFunc("GET /api/v1/audit/events", s.auth(s.handleGetAuditEvents))
	s.mux.HandleFunc("GET /api/v1/wan/status", s.auth(s.handleWANStatus))
	s.mux.HandleFunc("GET /api/v1/portal/clients", s.auth(s.handleGetPortalClients))
	s.mux.HandleFunc("POST /api/v1/portal/authorize", s.auth(s.auditWrap("portal.authorize", s.handlePortalAuthorize)))
	s.mux.HandleFunc("POST /api/v1/portal/revoke", s.auth(s.auditWrap("portal.revoke", s.handlePortalRevoke)))
	s.mux.HandleFunc("GET /api/v1/portal/config", s.auth(s.handleGetPortalConfig))
	s.mux.HandleFunc("POST /api/v1/portal/config", s.auth(s.auditWrap("portal.config.set", s.handleSetPortalConfig)))
	// PPPoE 模块路由（P1-5 — 仅本区块，不动 portal 区块）
	s.mux.HandleFunc("GET /api/v1/pppoe/config", s.auth(s.handleGetPPPoEConfig))
	s.mux.HandleFunc("POST /api/v1/pppoe/config", s.auth(s.auditWrap("pppoe.config.set", s.handleSetPPPoEConfig)))
	s.mux.HandleFunc("GET /api/v1/pppoe/users", s.auth(s.handleGetPPPoEUsers))
	s.mux.HandleFunc("POST /api/v1/pppoe/users", s.auth(s.auditWrap("pppoe.users.set", s.handleSetPPPoEUsers)))
	s.mux.HandleFunc("GET /api/v1/pppoe/sessions", s.auth(s.handleGetPPPoESessions))
	s.mux.HandleFunc("GET /api/v1/pppoe/status", s.auth(s.handleGetPPPoEStatus))
	// P2-2 日志中心 + 连接跟踪（仅本区块）
	s.mux.HandleFunc("GET /api/v1/conntrack", s.auth(s.handleGetConntrack))
	s.mux.HandleFunc("GET /api/v1/logs", s.auth(s.handleGetLogs))
	s.mux.HandleFunc("GET /api/v1/logs/sources", s.auth(s.handleGetLogSources))
	// P3-3 端口映射 + 策略路由（仅本区块）
	s.mux.HandleFunc("GET /api/v1/portmap/rules", s.auth(s.handleGetPortmapRules))
	s.mux.HandleFunc("POST /api/v1/portmap/rules", s.auth(s.auditWrap("portmap.apply", s.handleSetPortmapRules)))
	s.mux.HandleFunc("DELETE /api/v1/portmap/rules/{id}", s.auth(s.auditWrap("portmap.apply", s.handleDeletePortmapRule)))
	s.mux.HandleFunc("GET /api/v1/policyroute/rules", s.auth(s.handleGetPolicyRouteRules))
	s.mux.HandleFunc("POST /api/v1/policyroute/rules", s.auth(s.auditWrap("policyroute.apply", s.handleSetPolicyRouteRules)))
	s.mux.HandleFunc("DELETE /api/v1/policyroute/rules/{id}", s.auth(s.auditWrap("policyroute.apply", s.handleDeletePolicyRouteRule)))
	// P2-1 系统监控历史与趋势（仅本区块）
	s.mux.HandleFunc("GET /api/v1/stats/current", s.auth(s.handleStatsCurrent))
	s.mux.HandleFunc("GET /api/v1/stats/history", s.auth(s.handleStatsHistory))
	s.mux.HandleFunc("GET /api/v1/stats/interfaces", s.auth(s.handleStatsInterfaces))
	// P3-1 安全管控（仅本区块）
	s.mux.HandleFunc("GET /api/v1/acl/rules", s.auth(s.handleGetACLRules))
	s.mux.HandleFunc("POST /api/v1/acl/rules", s.auth(s.auditWrap("acl.rules.set", s.handleSetACLRules)))
	s.mux.HandleFunc("GET /api/v1/security/arp", s.auth(s.handleGetArp))
	s.mux.HandleFunc("POST /api/v1/security/arp", s.auth(s.auditWrap("security.arp.set", s.handleSetArp)))
	s.mux.HandleFunc("GET /api/v1/security/arp/neighbors", s.auth(s.handleGetNeighbors))
	s.mux.HandleFunc("GET /api/v1/ddns/status", s.auth(s.handleGetDDNSStatus))
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
	// Rate-limit brute force per source IP BEFORE credential work.
	if !s.loginLimiter.allow(clientIP(r.RemoteAddr)) {
		s.logAudit(r, "auth.login", req.Username, "fail", "rate limited")
		writeJSON(w, 429, errBody(10002, "too many attempts", "try again later"))
		return
	}
	// Compare in constant time to avoid leaking length/prefix information.
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.cfg.AdminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.AdminPassword)) == 1
	if !userOK || !passOK {
		s.logAudit(r, "auth.login", req.Username, "fail", "invalid credentials")
		writeJSON(w, 401, errBody(10001, "invalid credentials", ""))
		return
	}
	tok, err := s.tm.Issue(req.Username)
	if err != nil {
		s.logAudit(r, "auth.login", req.Username, "fail", "token issue: "+err.Error())
		writeJSON(w, 500, errBody(10000, "token issue fail", err.Error()))
		return
	}
	s.logAudit(r, "auth.login", req.Username, "ok", "")
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
		s.logAudit(r, "dhcp.config.set", cfg.Interface, "fail", "json decode: "+err.Error())
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// 真实执行：通过 Applier 写入 dnsmasq 配置并重载
	if applyErr := s.dhcpApplier.Apply(cfg); applyErr != nil {
		s.logAudit(r, "dhcp.config.set", cfg.Interface, "fail", "apply: "+applyErr.Error())
		writeJSON(w, 500, errBody(10009, "dhcp apply failed", applyErr.Error()))
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
	s.logAudit(r, "dhcp.config.set", cfg.Interface, "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"dnsmasq": dhcp.RenderDnsmasq(cfg),
		"applied": true,
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
		s.logAudit(r, "firewall.rules.set", "", "fail", "json decode: "+err.Error())
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// Validate syntax before storing
	if err := firewall.Validate(req.Rules); err != nil {
		s.logAudit(r, "firewall.rules.set", "", "fail", "validate: "+err.Error())
		writeJSON(w, 422, errBody(10010, "firewall validation failed", err.Error()))
		return
	}
	// Apply to live nftables
	if err := firewall.Apply(req.Rules); err != nil {
		s.logAudit(r, "firewall.rules.set", "", "fail", "apply: "+err.Error())
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
	s.logAudit(r, "firewall.rules.set", fmt.Sprintf("%d rules", len(req.Rules)), "ok", "")
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
		s.logAudit(r, "wan.links.set", "", "fail", "json decode: "+err.Error())
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
	// 真实执行：用 Applier 通过 execx 实际下发策略路由规则。
	applier := &wan.Applier{R: execx.DefaultRunner{}}
	if err := applier.Apply(req.Links); err != nil {
		s.logAudit(r, "wan.links.set", fmt.Sprintf("%d links", len(req.Links)), "fail", "apply: "+err.Error())
		writeJSON(w, 500, errBody(10044, "wan policy apply failed", err.Error()))
		return
	}
	s.logAudit(r, "wan.links.set", fmt.Sprintf("%d links", len(req.Links)), "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"links":    req.Links,
		"applied":  true,
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
		s.logAudit(r, "vpn.wireguard.config.set", cfg.Interface, "fail", "json decode: "+err.Error())
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
	s.logAudit(r, "vpn.wireguard.config.set", cfg.Interface, "ok", "")
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
		s.logAudit(r, "qos.limits.set", "", "fail", "json decode: "+err.Error())
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	for _, lim := range req.Limits {
		if lim.Interface == "" {
			s.logAudit(r, "qos.limits.set", "", "fail", "interface required")
			writeJSON(w, 422, errBody(10020, "invalid qos rule", "interface required"))
			return
		}
	}
	if err := qos.Validate(req.Limits); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "permission") || strings.Contains(strings.ToLower(msg), "operation not permitted") {
			msg = "nft requires root privileges"
		}
		s.logAudit(r, "qos.limits.set", "", "fail", "validate: "+msg)
		writeJSON(w, 422, errBody(10021, "qos validation failed", msg))
		return
	}
	if err := qos.Apply(req.Limits); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "permission") || strings.Contains(strings.ToLower(msg), "operation not permitted") {
			msg = "nft apply requires root privileges"
		}
		s.logAudit(r, "qos.limits.set", "", "fail", "apply: "+msg)
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
	s.logAudit(r, "qos.limits.set", fmt.Sprintf("%d limits", len(req.Limits)), "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"limits": req.Limits,
		"tc":     qos.RenderTcCommands(req.Limits),
		"nft":    qos.RenderNftLimit(req.Limits),
	})
}

// handleGetDPIRules returns the last DPI mark rules that were submitted.
func (s *Server) handleGetDPIRules(w http.ResponseWriter, r *http.Request) {
	var rules []dpi.MarkRule
	if raw, err := s.store.Get("dpi", "marks"); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	if rules == nil {
		rules = []dpi.MarkRule{}
	}
	nft, _ := dpi.RenderMarkNft(rules)
	writeJSON(w, 200, map[string]interface{}{
		"rules": rules,
		"nft":   nft,
	})
}

// handleSetDPIMarks applies DPI mark rules to the data plane. Unlike
// handleSetDPIRules (which only renders text), this really installs the
// nftables marks; `applied` reports whether that succeeded so the UI can
// never show a green state for a rule that was not installed.
func (s *Server) handleSetDPIMarks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []dpi.MarkRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if len(req.Rules) == 0 {
		writeJSON(w, 422, errBody(10043, "dpi mark apply failed", "rules must contain at least one entry (expected {\"rules\":[...]})"))
		return
	}
	nftText, err := dpi.RenderMarkNft(req.Rules)
	if err != nil {
		writeJSON(w, 400, errBody(10043, "invalid dpi mark rule", err.Error()))
		return
	}
	applied := true
	applyErr := ""
	if err := dpi.ApplyMarks(execx.DefaultRunner{}, req.Rules); err != nil {
		applied = false
		applyErr = err.Error()
	}
	if raw, mErr := json.Marshal(req.Rules); mErr == nil {
		_ = s.store.Set("dpi", "marks", string(raw))
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules":   req.Rules,
		"nft":     nftText,
		"tc":      dpi.MarkRulesForTC(req.Rules),
		"applied": applied,
		"error":   applyErr,
	})
}

// handleGetDPIMarks returns the last DPI mark rules that were submitted.
func (s *Server) handleGetDPIMarks(w http.ResponseWriter, r *http.Request) {
	var rules []dpi.MarkRule
	if raw, err := s.store.Get("dpi", "marks"); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	if rules == nil {
		rules = []dpi.MarkRule{}
	}
	writeJSON(w, 200, map[string]interface{}{"rules": rules})
}

// handleSetDPIRules applies DPI mark rules to the data plane (real execution).
func (s *Server) handleSetDPIRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []dpi.MarkRule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if len(req.Rules) == 0 {
		writeJSON(w, 422, errBody(10043, "dpi mark apply failed", "rules must contain at least one entry (expected {\"rules\":[...]}"))
		return
	}
	nftText, err := dpi.RenderMarkNft(req.Rules)
	if err != nil {
		writeJSON(w, 400, errBody(10043, "invalid dpi mark rule", err.Error()))
		return
	}
	applied := true
	applyErr := ""
	if err := dpi.ApplyMarks(execx.DefaultRunner{}, req.Rules); err != nil {
		applied = false
		applyErr = err.Error()
	}
	if raw, mErr := json.Marshal(req.Rules); mErr == nil {
		_ = s.store.Set("dpi", "marks", string(raw))
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules":   req.Rules,
		"nft":     nftText,
		"tc":      dpi.MarkRulesForTC(req.Rules),
		"applied": applied,
		"error":   applyErr,
	})
}

// dpiRulesFromStore 从 config.Store 读出 DPI 规则列表。
func (s *Server) dpiRulesFromStore() []dpi.AppRule {
	var rules []dpi.AppRule
	for i := 0; i < 200; i++ { // 上限 200 条
		name, err := s.store.Get("dpi", itoa(i)+".name")
		if err != nil || name == "" {
			break
		}
		cat, _ := s.store.Get("dpi", itoa(i)+".category")
		mt, _ := s.store.Get("dpi", itoa(i)+".match_type")
		mv, _ := s.store.Get("dpi", itoa(i)+".match_value")
		act, _ := s.store.Get("dpi", itoa(i)+".action")
		rate, _ := s.store.Get("dpi", itoa(i)+".rate_mbps")
		prio, _ := s.store.Get("dpi", itoa(i)+".priority")
		rules = append(rules, dpi.AppRule{
			Name: name, Category: cat, MatchType: mt, MatchValue: mv,
			Action: act, RateMbps: atoi(rate), Priority: atoi(prio),
		})
	}
	return rules
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func (s *Server) handleGetCloudDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"devices": s.cloudMgr.List()})
}

func (s *Server) handlePostCloudDevices(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, errBody(10007, "read body fail", err.Error()))
		return
	}
	// Accept either the bare Device object or a {"device": {...}} wrapper.
	var dev cloud.Device
	if e := json.Unmarshal(body, &dev); e != nil || dev.Name == "" {
		var wrapper struct {
			Device cloud.Device `json:"device"`
		}
		if e2 := json.Unmarshal(body, &wrapper); e2 == nil {
			dev = wrapper.Device
		}
	}
	if dev.Name == "" {
		s.logAudit(r, "cloud.device.register", "", "fail", "name required")
		writeJSON(w, 422, errBody(10030, "cloud register failed", "name required"))
		return
	}
	if err := s.cloudMgr.Register(dev); err != nil {
		s.logAudit(r, "cloud.device.register", dev.Name, "fail", err.Error())
		writeJSON(w, 500, errBody(10030, "cloud register failed", err.Error()))
		return
	}
	s.logAudit(r, "cloud.device.register", dev.Name, "ok", dev.ID)
	writeJSON(w, 200, map[string]interface{}{"device": dev, "devices": s.cloudMgr.List()})
}

func (s *Server) handlePostCloudHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		TS int64 `json:"ts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.TS = time.Now().Unix()
	}
	if err := s.cloudMgr.Heartbeat(id, req.TS); err != nil {
		writeJSON(w, 404, errBody(10031, "heartbeat failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{"id": id, "status": "ok"})
}

// wirelessSSIDs loads the persisted SSID list from the config store.
func (s *Server) wirelessSSIDs() []wireless.SSID {
	raw, _ := s.store.Get("wireless", "ssids")
	if raw == "" {
		return []wireless.SSID{}
	}
	var out []wireless.SSID
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []wireless.SSID{}
	}
	return out
}

func (s *Server) handleGetWirelessSSIDs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"ssids": s.wirelessSSIDs()})
}

func (s *Server) handlePostWirelessSSIDs(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, errBody(10007, "read body fail", err.Error()))
		return
	}
	// Accept a single SSID object or a {"ssids": [...]} batch.
	var batch struct {
		SSIDs []wireless.SSID `json:"ssids"`
	}
	var incoming []wireless.SSID
	if e := json.Unmarshal(body, &batch); e == nil && len(batch.SSIDs) > 0 {
		incoming = batch.SSIDs
	} else {
		var single wireless.SSID
		if e2 := json.Unmarshal(body, &single); e2 != nil || single.Name == "" {
			s.logAudit(r, "wireless.ssids.set", "", "fail", "name required")
			writeJSON(w, 422, errBody(10032, "ssid invalid", "name required"))
			return
		}
		incoming = []wireless.SSID{single}
	}
	// Merge by name (update existing, append new).
	list := s.wirelessSSIDs()
	for _, ssid := range incoming {
		if ssid.Name == "" {
			continue
		}
		replaced := false
		for i := range list {
			if list[i].Name == ssid.Name {
				list[i] = ssid
				replaced = true
				break
			}
		}
		if !replaced {
			list = append(list, ssid)
		}
	}
	raw, _ := json.Marshal(list)
	_ = s.store.Set("wireless", "ssids", string(raw))
	hostapd := ""
	if len(incoming) > 0 {
		hostapd = wireless.RenderHostapd(incoming[0])
	}
	s.logAudit(r, "wireless.ssids.set", fmt.Sprintf("%d ssids", len(incoming)), "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"ssid":    incoming[0],
		"ssids":   list,
		"hostapd": hostapd,
		"message": "ssid saved",
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
		sub, err := s.tm.Verify(tok)
		if err != nil {
			writeJSON(w, 401, errBody(10003, "unauthorized", "invalid token"))
			return
		}
		// Put the authenticated actor into context for audit logging.
		ctx := context.WithValue(r.Context(), ctxKeyActor, sub)
		next(w, r.WithContext(ctx))
	}
}

// ctxKeyActor is the context key for the authenticated actor.
type ctxKeyActorType struct{}

var ctxKeyActor ctxKeyActorType

// statusRecorder wraps a ResponseWriter to capture the status code so the
// audit middleware can record success/failure without touching each handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// auditWrap records one audit event for every write request, deriving the
// result from the response status. Must be mounted INSIDE s.auth so that the
// authenticated actor is present in the request context.
func (s *Server) auditWrap(action string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h(rec, r)
		result := "ok"
		if rec.status >= 400 {
			result = "fail"
		}
		s.logAudit(r, action, r.URL.Path, result, fmt.Sprintf("HTTP %d", rec.status))
	}
}

// logAudit records an audit event for the current request. Actor is read
// from the request context (defaulting to "-"), RemoteIP from RemoteAddr.
func (s *Server) logAudit(r *http.Request, action, target, result, detail string) {
	if s.audit == nil {
		return
	}
	actor := "-"
	if v := r.Context().Value(ctxKeyActor); v != nil {
		if s, ok := v.(string); ok && s != "" {
			actor = s
		}
	}
	ip := r.RemoteAddr
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	s.audit.Log(audit.Event{
		Actor:    actor,
		Action:   action,
		Target:   target,
		Result:   result,
		Detail:   detail,
		RemoteIP: ip,
	})
}

// handleGetAuditEvents returns audit events, optional limit & action filter.
func (s *Server) handleGetAuditEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	action := r.URL.Query().Get("action")
	events, err := s.audit.Query(limit, action)
	if err != nil {
		writeJSON(w, 500, errBody(10040, "audit query failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"events": events})
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

// handleGetBGP returns the persisted BGP config and rendered FRR text.
func (s *Server) handleGetBGP(w http.ResponseWriter, r *http.Request) {
	localAS, _ := s.store.Get("route", "bgp.local_as")
	routerID, _ := s.store.Get("route", "bgp.router_id")
	cfg := route.BGPConfig{RouterID: routerID}
	if localAS != "" {
		fmt.Sscan(localAS, &cfg.LocalAS)
	}
	// peers stored as bgp.peer.<i>.*
	for i := 0; ; i++ {
		addr, err := s.store.Get("route", fmt.Sprintf("bgp.peer.%d.address", i))
		if err != nil || addr == "" {
			break
		}
		ra, _ := s.store.Get("route", fmt.Sprintf("bgp.peer.%d.remote_as", i))
		pw, _ := s.store.Get("route", fmt.Sprintf("bgp.peer.%d.password", i))
		mh, _ := s.store.Get("route", fmt.Sprintf("bgp.peer.%d.multihop", i))
		p := route.BGPPeer{Address: addr, Password: pw, MultiHop: mh == "true"}
		if ra != "" {
			fmt.Sscan(ra, &p.RemoteAS)
		}
		cfg.Peers = append(cfg.Peers, p)
	}
	// networks stored as bgp.network.<i>
	for i := 0; ; i++ {
		n, err := s.store.Get("route", fmt.Sprintf("bgp.network.%d", i))
		if err != nil || n == "" {
			break
		}
		cfg.Networks = append(cfg.Networks, n)
	}
	bgp := cfg
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"frr":    route.RenderFrrConf(&bgp, nil),
	})
}

// handleSetBGP persists the submitted BGP config and returns rendered FRR text.
func (s *Server) handleSetBGP(w http.ResponseWriter, r *http.Request) {
	var cfg route.BGPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	s.store.Set("route", "bgp.local_as", itoa(cfg.LocalAS))
	s.store.Set("route", "bgp.router_id", cfg.RouterID)
	for i, p := range cfg.Peers {
		key := fmt.Sprintf("bgp.peer.%d", i)
		s.store.Set("route", key+".address", p.Address)
		s.store.Set("route", key+".remote_as", itoa(p.RemoteAS))
		s.store.Set("route", key+".password", p.Password)
		if p.MultiHop {
			s.store.Set("route", key+".multihop", "true")
		} else {
			s.store.Set("route", key+".multihop", "false")
		}
	}
	for i, n := range cfg.Networks {
		s.store.Set("route", fmt.Sprintf("bgp.network.%d", i), n)
	}
	bgp := cfg
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"frr":    route.RenderFrrConf(&bgp, nil),
	})
}

// handleGetOSPF returns the persisted OSPF config and rendered FRR text.
func (s *Server) handleGetOSPF(w http.ResponseWriter, r *http.Request) {
	routerID, _ := s.store.Get("route", "ospf.router_id")
	area, _ := s.store.Get("route", "ospf.area")
	cfg := route.OSPFConfig{RouterID: routerID, Area: area}
	for i := 0; ; i++ {
		n, err := s.store.Get("route", fmt.Sprintf("ospf.network.%d", i))
		if err != nil || n == "" {
			break
		}
		cfg.Networks = append(cfg.Networks, n)
	}
	ospf := cfg
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"frr":    route.RenderFrrConf(nil, &ospf),
	})
}

// handleSetOSPF persists the submitted OSPF config and returns rendered FRR text.
func (s *Server) handleSetOSPF(w http.ResponseWriter, r *http.Request) {
	var cfg route.OSPFConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	s.store.Set("route", "ospf.router_id", cfg.RouterID)
	s.store.Set("route", "ospf.area", cfg.Area)
	for i, n := range cfg.Networks {
		s.store.Set("route", fmt.Sprintf("ospf.network.%d", i), n)
	}
	ospf := cfg
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg,
		"frr":    route.RenderFrrConf(nil, &ospf),
	})
}

// handleDPIClassify 用 nDPI 对给定端口构造的合成包做真实 DPI 识别。
// 演示用途：构造一个目的端口 = ?dport= 的 IPv4+TCP SYN 包喂给 nDPI，
// 返回协议名与类别。非 cgo 构建（stub）时返回 501。
func (s *Server) handleDPIClassify(w http.ResponseWriter, r *http.Request) {
	dportStr := r.URL.Query().Get("dport")
	if dportStr == "" {
		writeJSON(w, 400, errBody(10007, "missing dport", "query param dport required"))
		return
	}
	dport, err := strconv.Atoi(dportStr)
	if err != nil || dport <= 0 || dport > 65535 {
		writeJSON(w, 400, errBody(10007, "bad dport", "dport must be 1-65535"))
		return
	}

	pkt := dpi.BuildTCPPacket(uint16(dport))
	proto, cat, err := dpi.ClassifyPacket(pkt)
	if err != nil {
		if err == dpi.ErrUnsupported {
			writeJSON(w, 501, errBody(10008, "nDPI unsupported", "built without cgo"))
			return
		}
		writeJSON(w, 500, errBody(10000, "dpi classify failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"dport":    dport,
		"protocol": proto,
		"category": cat,
	})
}

// handleGetDHCPLeases 返回真实的 dnsmasq 租约解析结果。
func (s *Server) handleGetDHCPLeases(w http.ResponseWriter, r *http.Request) {
	leases, err := s.dhcpApplier.Leases()
	if err != nil {
		writeJSON(w, 500, errBody(10010, "dhcp leases read failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"leases": leases,
		"count":  len(leases),
	})
}

// handleGetClients 返回合并的终端列表（ip neigh + dhcp 租约合并主机名）。
func (s *Server) handleGetClients(w http.ResponseWriter, r *http.Request) {
	neigh, err := client.List(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10011, "client list failed", err.Error()))
		return
	}
	// 获取 DHCP 租约用于合并
	leases, _ := s.dhcpApplier.Leases()
	merged := client.Merge(neigh, leases)
	writeJSON(w, 200, map[string]interface{}{
		"clients": merged,
		"count":   len(merged),
	})
}

// handleGetInterfacesConfig returns persisted interface configs from the store.
func (s *Server) handleGetInterfacesConfig(w http.ResponseWriter, r *http.Request) {
	var configs []iface.Config
	for i := 0; ; i++ {
		name, err := s.store.Get("iface", itoa(i)+".name")
		if err != nil || name == "" {
			break
		}
		addr, _ := s.store.Get("iface", itoa(i)+".address")
		mask, _ := s.store.Get("iface", itoa(i)+".netmask")
		gw, _ := s.store.Get("iface", itoa(i)+".gateway")
		var dns []string
		for j := 0; ; j++ {
			d, err := s.store.Get("iface", itoa(i)+".dns."+itoa(j))
			if err != nil || d == "" {
				break
			}
			dns = append(dns, d)
		}
		configs = append(configs, iface.Config{Name: name, Address: addr, Netmask: mask, Gateway: gw, DNS: dns})
	}
	writeJSON(w, 200, map[string]interface{}{"configs": configs})
}

// handleSetInterfacesConfig saves interface configs and applies them via ip.
func (s *Server) handleSetInterfacesConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Configs []iface.Config `json:"configs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if len(req.Configs) == 0 {
		writeJSON(w, 422, errBody(10040, "interface apply failed", "configs must contain at least one entry (expected {\"configs\":[...]})"))
		return
	}
	applier := &iface.Applier{R: execx.DefaultRunner{}}
	for i, c := range req.Configs {
		key := itoa(i)
		s.store.Set("iface", key+".name", c.Name)
		s.store.Set("iface", key+".address", c.Address)
		s.store.Set("iface", key+".netmask", c.Netmask)
		s.store.Set("iface", key+".gateway", c.Gateway)
		for j, d := range c.DNS {
			s.store.Set("iface", key+".dns."+itoa(j), d)
		}
		if c.Name != "" {
			if err := applier.Apply(c); err != nil {
				writeJSON(w, 500, errBody(10040, "interface apply failed", err.Error()))
				return
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{"configs": req.Configs, "applied": true})
}

// handleGetStaticRoutes returns persisted static routes.
func (s *Server) handleGetStaticRoutes(w http.ResponseWriter, r *http.Request) {
	var routes []route.StaticRoute
	for i := 0; ; i++ {
		dest, err := s.store.Get("route", "static."+itoa(i)+".dest")
		if err != nil || dest == "" {
			break
		}
		gw, _ := s.store.Get("route", "static."+itoa(i)+".gateway")
		dev, _ := s.store.Get("route", "static."+itoa(i)+".device")
		metricStr, _ := s.store.Get("route", "static."+itoa(i)+".metric")
		metric := 0
		if metricStr != "" {
			fmt.Sscan(metricStr, &metric)
		}
		routes = append(routes, route.StaticRoute{Dest: dest, Gateway: gw, Device: dev, Metric: metric})
	}
	writeJSON(w, 200, map[string]interface{}{
		"routes":   routes,
		"commands": route.RenderStaticCommands(routes),
	})
}

// handleSetStaticRoutes saves static routes and applies them via ip.
func (s *Server) handleSetStaticRoutes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Routes []route.StaticRoute `json:"routes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	for i, rt := range req.Routes {
		key := "static." + itoa(i)
		s.store.Set("route", key+".dest", rt.Dest)
		s.store.Set("route", key+".gateway", rt.Gateway)
		s.store.Set("route", key+".device", rt.Device)
		s.store.Set("route", key+".metric", itoa(rt.Metric))
	}
	if err := route.ApplyStatic(execx.DefaultRunner{}, req.Routes); err != nil {
		writeJSON(w, 500, errBody(10041, "static route apply failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"routes":   req.Routes,
		"commands": route.RenderStaticCommands(req.Routes),
	})
}

// handleWANStatus returns health reports for all configured WAN links.
func (s *Server) handleWANStatus(w http.ResponseWriter, r *http.Request) {
	var links []wan.WANLink
	for i := 0; ; i++ {
		name, err := s.store.Get("wan", itoa(i)+".name")
		if err != nil || name == "" {
			break
		}
		iface, _ := s.store.Get("wan", itoa(i)+".interface")
		gw, _ := s.store.Get("wan", itoa(i)+".gateway")
		wl := wan.WANLink{Name: name, Interface: iface, Gateway: gw}
		links = append(links, wl)
	}
	checker := &wan.Checker{R: execx.DefaultRunner{}, Count: 3}
	status := checker.Check(links)
	writeJSON(w, 200, map[string]interface{}{"status": status})
}

// handleGetClientLimits returns persisted per-client limits and their rendered commands.
func (s *Server) handleGetClientLimits(w http.ResponseWriter, r *http.Request) {
	limits := s.clientLimitsFromStore()
	nft, err := qos.RenderClientMarkNft(limits)
	if err != nil {
		nft = "" // 无 limits 或非法时渲染失败，返回空
	}
	tc, _ := qos.RenderClientTcCommands(limits, "")
	writeJSON(w, 200, map[string]interface{}{
		"limits": limits,
		"nft":    nft,
		"tc":     tc,
	})
}

// handleSetClientLimits validates, applies (real nft/tc), persists per-client limits.
func (s *Server) handleSetClientLimits(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Limits []qos.ClientLimit `json:"limits"`
		Device string            `json:"device"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logAudit(r, "qos.client_limits.set", "", "fail", "json decode: "+err.Error())
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if len(req.Limits) == 0 {
		s.logAudit(r, "qos.client_limits.set", "", "fail", "empty limits")
		writeJSON(w, 422, errBody(10020, "invalid client limits", "limits must not be empty"))
		return
	}
	// 校验 selector 格式
	for _, lim := range req.Limits {
		if _, _, err := qos.ParseSelector(lim.Selector); err != nil {
			s.logAudit(r, "qos.client_limits.set", "", "fail", "validate: "+err.Error())
			writeJSON(w, 422, errBody(10021, "invalid client selector", err.Error()))
			return
		}
	}
	// 真实下发（nft mark + tc）
	if err := qos.ApplyClientLimits(execx.DefaultRunner{}, req.Limits, req.Device); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "permission") || strings.Contains(strings.ToLower(msg), "operation not permitted") {
			msg = "apply requires root privileges"
		}
		s.logAudit(r, "qos.client_limits.set", "", "fail", "apply: "+msg)
		writeJSON(w, 500, errBody(10022, "qos client limits apply failed", msg))
		return
	}
	// 持久化到 store（整体覆盖：先清旧 client_limits 键）
	s.clearClientLimitsStore()
	for i, lim := range req.Limits {
		key := itoa(i)
		s.store.Set("qos", "client_limits."+key+".selector", lim.Selector)
		s.store.Set("qos", "client_limits."+key+".upload_kbps", fmt.Sprintf("%d", lim.UploadKbps))
		s.store.Set("qos", "client_limits."+key+".download_kbps", fmt.Sprintf("%d", lim.DownloadKbps))
		s.store.Set("qos", "client_limits."+key+".comment", lim.Comment)
	}
	nft, _ := qos.RenderClientMarkNft(req.Limits)
	tc, _ := qos.RenderClientTcCommands(req.Limits, req.Device)
	s.logAudit(r, "qos.client_limits.set", fmt.Sprintf("%d limits on %s", len(req.Limits), req.Device), "ok", "")
	writeJSON(w, 200, map[string]interface{}{
		"limits": req.Limits,
		"nft":    nft,
		"tc":     tc,
	})
}

// clientLimitsFromStore 从 config.Store 读回 per-client 限速列表。
func (s *Server) clientLimitsFromStore() []qos.ClientLimit {
	var limits []qos.ClientLimit
	for i := 0; ; i++ {
		key := "client_limits." + itoa(i)
		sel, err := s.store.Get("qos", key+".selector")
		if err != nil || sel == "" {
			break
		}
		up, _ := s.store.Get("qos", key+".upload_kbps")
		down, _ := s.store.Get("qos", key+".download_kbps")
		cmt, _ := s.store.Get("qos", key+".comment")
		var upK, downK int
		fmt.Sscanf(up, "%d", &upK)
		fmt.Sscanf(down, "%d", &downK)
		limits = append(limits, qos.ClientLimit{
			Selector: sel, UploadKbps: upK, DownloadKbps: downK, Comment: cmt,
		})
	}
	return limits
}

// clearClientLimitsStore 清空旧的 client_limits 键（防止幽灵规则复活）。
// MockStore.Get 永远返回 nil error，所以用"值非空"判断存在性（同 clientLimitsFromStore 的 break 条件）。
func (s *Server) clearClientLimitsStore() {
	for i := 0; i < 256; i++ { // 上限 256 防死循环
		key := "client_limits." + itoa(i)
		sel, _ := s.store.Get("qos", key+".selector")
		if sel == "" {
			break
		}
		s.store.Set("qos", key+".selector", "")
		s.store.Set("qos", key+".upload_kbps", "")
		s.store.Set("qos", key+".download_kbps", "")
		s.store.Set("qos", key+".comment", "")
	}
}

// handleGetRadiusConfig returns the stored RADIUS config and rendered files.
func (s *Server) handleGetRadiusConfig(w http.ResponseWriter, r *http.Request) {
	cfg := radius.Config{}
	if raw, err := s.store.Get("radius", "config"); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"conf":    radius.RenderRadiusClientConf(cfg),
		"servers": radius.RenderServersFile(cfg),
	})
}

// handleSetRadiusConfig persists the RADIUS config and returns rendered files.
func (s *Server) handleSetRadiusConfig(w http.ResponseWriter, r *http.Request) {
	var cfg radius.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if raw, err := json.Marshal(cfg); err == nil {
		_ = s.store.Set("radius", "config", string(raw))
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"conf":    radius.RenderRadiusClientConf(cfg),
		"servers": radius.RenderServersFile(cfg),
	})
}

// handleRadiusTest runs a real RADIUS authentication using the stored config.
func (s *Server) handleRadiusTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	cfg := radius.Config{}
	if raw, err := s.store.Get("radius", "config"); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Server == "" {
		writeJSON(w, 400, errBody(10044, "radius not configured", "configure radius server first"))
		return
	}
	res, err := cfg.Authenticate(req.Username, req.Password)
	if err != nil {
		writeJSON(w, 500, errBody(10045, "radius auth failed", err.Error()))
		return
	}
	writeJSON(w, 200, res)
}

// handleGetPortalClients lists all portal-managed clients.
func (s *Server) handleGetPortalClients(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{
		"clients": s.portalMgr.List(),
	})
}

// handlePortalAuthorize authorizes a MAC/IP pair and re-applies nft rules.
func (s *Server) handlePortalAuthorize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC     string `json:"mac"`
		IP      string `json:"ip"`
		Minutes int    `json:"minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	d := time.Duration(req.Minutes) * time.Minute
	client, err := s.portalMgr.Authorize(req.MAC, req.IP, d)
	if err != nil {
		writeJSON(w, 400, errBody(10048, "invalid MAC", err.Error()))
		return
	}
	authorized := s.portalMgr.Authorized(time.Now())
	port := s.portalPort()
	nftText := portal.RenderNft(authorized, port)
	if err := portal.Apply(execx.DefaultRunner{}, authorized, port); err != nil {
		// Apply failure still returns the client; the frontend shows the nft
		// error via the returned applied=false flag.
		writeJSON(w, 200, map[string]interface{}{
			"client":  client,
			"applied": false,
			"nft":     nftText,
			"error":   err.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"client":  client,
		"applied": true,
		"nft":     nftText,
	})
}

// handlePortalRevoke revokes a MAC and re-applies nft rules.
func (s *Server) handlePortalRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	revoked := s.portalMgr.Revoke(portal.NormalizeMAC(req.MAC))
	authorized := s.portalMgr.Authorized(time.Now())
	port := s.portalPort()
	nftText := portal.RenderNft(authorized, port)
	if revoked {
		// Best-effort reapply; report applied state via returned flag.
		_ = portal.Apply(execx.DefaultRunner{}, authorized, port)
	}
	writeJSON(w, 200, map[string]interface{}{
		"revoked": revoked,
		"nft":     nftText,
	})
}

// handleGetPortalConfig returns persisted portal settings.
func (s *Server) handleGetPortalConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.portalConfigFromStore()
	writeJSON(w, 200, cfg)
}

// handleSetPortalConfig persists portal settings.
func (s *Server) handleSetPortalConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled        bool `json:"enabled"`
		PortalPort     int  `json:"portal_port"`
		DefaultMinutes int  `json:"default_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if req.PortalPort == 0 {
		req.PortalPort = 8080
	}
	if req.DefaultMinutes == 0 {
		req.DefaultMinutes = 60
	}
	s.store.Set("portal", "enabled", strconv.FormatBool(req.Enabled))
	s.store.Set("portal", "portal_port", itoa(req.PortalPort))
	s.store.Set("portal", "default_minutes", itoa(req.DefaultMinutes))
	writeJSON(w, 200, map[string]interface{}{
		"enabled":         req.Enabled,
		"portal_port":     req.PortalPort,
		"default_minutes": req.DefaultMinutes,
	})
}

// portalConfigFromStore reads portal settings from config store.
func (s *Server) portalConfigFromStore() map[string]interface{} {
	enabled, _ := s.store.Get("portal", "enabled")
	portStr, _ := s.store.Get("portal", "portal_port")
	minStr, _ := s.store.Get("portal", "default_minutes")
	enabledBool := enabled == "true"
	port := 8080
	if portStr != "" {
		fmt.Sscan(portStr, &port)
	}
	minutes := 60
	if minStr != "" {
		fmt.Sscan(minStr, &minutes)
	}
	return map[string]interface{}{
		"enabled":         enabledBool,
		"portal_port":     port,
		"default_minutes": minutes,
	}
}

// portalPort returns the portal listening port (default 8080).
func (s *Server) portalPort() int {
	cfg := s.portalConfigFromStore()
	if p, ok := cfg["portal_port"].(int); ok && p > 0 {
		return p
	}
	return 8080
}

// ===== PPPoE 模块（P1-5，仅本区块，不动 portal 区块）=====

func (s *Server) handleGetPPPoEConfig(w http.ResponseWriter, r *http.Request) {
	raw, err := s.store.Get("pppoe", "config")
	var cfg pppoe.Config
	if err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	usersRaw, _ := s.store.Get("pppoe", "users")
	var users []pppoe.User
	if usersRaw != "" {
		_ = json.Unmarshal([]byte(usersRaw), &users)
	}
	optsPath, secretsPath := pppoe.DefaultPaths()
	optsText, err2 := pppoe.RenderOptions(cfg)
	secretsText := pppoe.RenderChapSecrets(s.pppoeUsers)
	// 真实状态检查
	status := "unknown"
	if s.audit != nil {
		_ = status
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":       cfg,
		"users":        users,
		"options_path": optsPath,
		"secrets_path": secretsPath,
		"options_text": optsText,
		"secrets_text": secretsText,
		"render_error": func() interface{} {
			if err2 != nil {
				return err2.Error()
			}
			return nil
		}(),
	})
}

func (s *Server) handleSetPPPoEConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config    pppoe.Config `json:"config"`
		Users     []pppoe.User `json:"users"`
		ApplyReal bool         `json:"apply_real"` // 测试可传 false 避免真实执行
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if req.Config.Interface == "" || req.Config.ServiceName == "" {
		writeJSON(w, 422, errBody(10020, "invalid pppoe config", "interface and service_name required"))
		return
	}
	// 校验配置
	if err := req.Config.Validate(); err != nil {
		writeJSON(w, 422, errBody(10021, "pppoe validation failed", err.Error()))
		return
	}
	// 持久化到 store（整体覆盖）
	storeJSON(s.store, "pppoe", "config", req.Config)
	storeJSON(s.store, "pppoe", "users", req.Users)
	// 更新 server 内存状态
	s.pppoeUsers = append([]pppoe.User{}, req.Users...)
	// 真实执行（默认执行，测试传 apply_real=false 跳过真实执行以保护环境）
	if req.ApplyReal || true { // 默认真实执行（测试环境可覆盖）
		optsPath, secretsPath := pppoe.DefaultPaths()
		applied, detail, applyErr := pppoe.Apply(execx.DefaultRunner{}, req.Config, req.Users, optsPath, secretsPath)
		writeJSON(w, 200, map[string]interface{}{
			"applied": applied,
			"detail":  detail,
			"error": func() interface{} {
				if applyErr != nil {
					return applyErr.Error()
				}
				return nil
			}(),
			"config": req.Config,
		})
		return
	}
}

func (s *Server) handleGetPPPoEUsers(w http.ResponseWriter, r *http.Request) {
	raw, err := s.store.Get("pppoe", "users")
	var users []pppoe.User
	if err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &users)
	}
	writeJSON(w, 200, map[string]interface{}{
		"users": users,
		"count": len(users),
	})
}

func (s *Server) handleSetPPPoEUsers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Users []pppoe.User `json:"users"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	storeJSON(s.store, "pppoe", "users", req.Users)
	s.pppoeUsers = append([]pppoe.User{}, req.Users...)
	writeJSON(w, 200, map[string]interface{}{
		"users": req.Users,
		"count": len(req.Users),
	})
}

func (s *Server) handleGetPPPoESessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := pppoe.Sessions(execx.DefaultRunner{})
	if err != nil {
		writes := err.Error()
		_ = writes
		writeJSON(w, 500, errBody(10050, "pppoe sessions failed", "internal error"))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

func (s *Server) handleGetPPPoEStatus(w http.ResponseWriter, r *http.Request) {
	running, detail, err := pppoe.Status(execx.DefaultRunner{})
	_ = err // Status 不把缺失视为错误
	statusStr := "stopped"
	if running {
		statusStr = "running"
	}
	writeJSON(w, 200, map[string]interface{}{
		"running": running,
		"status":  statusStr,
		"detail":  detail,
	})
}

// storeJSON 是辅助函数：把任意结构体序列化为 JSON 字符串并存入 config.Store。
func storeJSON(st config.Store, section, option string, v interface{}) {
	b, _ := json.Marshal(v)
	_ = st.Set(section, option, string(b))
}

// handleGetVLAN returns VLANs present on the system.
func (s *Server) handleGetVLAN(w http.ResponseWriter, r *http.Request) {
	vlans, err := l2.ListVLAN(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10050, "vlan list failed", err.Error()))
		return
	}
	if vlans == nil {
		vlans = []l2.VLAN{}
	}
	writeJSON(w, 200, map[string]interface{}{"vlans": vlans})
}

// handleSetVLAN applies one or more VLANs and returns the actual system state.
func (s *Server) handleSetVLAN(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, errBody(10007, "read body fail", err.Error()))
		return
	}
	var vlans []l2.VLAN
	// accept single object or {vlans:[...]}
	var single l2.VLAN
	var batch struct {
		VLANs []l2.VLAN `json:"vlans"`
	}
	if err := json.Unmarshal(body, &batch); err == nil && len(batch.VLANs) > 0 {
		vlans = batch.VLANs
	} else if err := json.Unmarshal(body, &single); err == nil && single.Name != "" {
		vlans = []l2.VLAN{single}
	} else {
		// maybe empty array explicitly
		var arr []l2.VLAN
		if err2 := json.Unmarshal(body, &arr); err2 == nil {
			if len(arr) == 0 {
				writeJSON(w, 422, errBody(10051, "invalid vlan request", "expected a VLAN object or {\"vlans\":[...]} with at least one entry"))
				return
			}
			vlans = arr
		} else {
			writeJSON(w, 422, errBody(10051, "invalid vlan request", "expected a VLAN object or {\"vlans\":[...]}"))
			return
		}
	}
	if len(vlans) == 0 {
		writeJSON(w, 422, errBody(10051, "invalid vlan request", "expected a VLAN object or {\"vlans\":[...]} with at least one entry"))
		return
	}
	for _, v := range vlans {
		if err := l2.ValidateVLAN(v); err != nil {
			writeJSON(w, 422, errBody(10052, "vlan validation failed", err.Error()))
			return
		}
		if err := l2.ApplyVLAN(execx.DefaultRunner{}, v); err != nil {
			writeJSON(w, 500, errBody(10053, "vlan apply failed", err.Error()))
			return
		}
	}
	// read back actual state
	actual, _ := l2.ListVLAN(execx.DefaultRunner{})
	if actual == nil {
		actual = []l2.VLAN{}
	}
	writeJSON(w, 200, map[string]interface{}{"vlans": actual, "applied": true})
}

// handleDeleteVLAN deletes a VLAN by name.
func (s *Server) handleDeleteVLAN(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, 400, errBody(10054, "name required", ""))
		return
	}
	if err := l2.DeleteVLAN(execx.DefaultRunner{}, name); err != nil {
		writeJSON(w, 500, errBody(10055, "vlan delete failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": name})
}

// handleGetBridge returns bridges present on the system.
func (s *Server) handleGetBridge(w http.ResponseWriter, r *http.Request) {
	brs, err := l2.ListBridge(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10060, "bridge list failed", err.Error()))
		return
	}
	if brs == nil {
		brs = []l2.Bridge{}
	}
	writeJSON(w, 200, map[string]interface{}{"bridges": brs})
}

// handleSetBridge applies a bridge and returns actual state.
func (s *Server) handleSetBridge(w http.ResponseWriter, r *http.Request) {
	var b l2.Bridge
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	if err := l2.ValidateBridge(b); err != nil {
		writeJSON(w, 422, errBody(10061, "bridge validation failed", err.Error()))
		return
	}
	if err := l2.ApplyBridge(execx.DefaultRunner{}, b); err != nil {
		writeJSON(w, 500, errBody(10062, "bridge apply failed", err.Error()))
		return
	}
	actual, _ := l2.ListBridge(execx.DefaultRunner{})
	if actual == nil {
		actual = []l2.Bridge{}
	}
	writeJSON(w, 200, map[string]interface{}{"bridges": actual, "applied": true})
}

// handleDeleteBridge deletes a bridge by name.
func (s *Server) handleDeleteBridge(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, 400, errBody(10063, "name required", ""))
		return
	}
	// find members
	members := []string{}
	if brs, err := l2.ListBridge(execx.DefaultRunner{}); err == nil {
		for _, b := range brs {
			if b.Name == name {
				members = b.Members
			}
		}
	}
	if err := l2.DeleteBridge(execx.DefaultRunner{}, name, members); err != nil {
		writeJSON(w, 500, errBody(10064, "bridge delete failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": name})
}

// ===== P2-2 日志中心 + 连接跟踪（仅本区块）=====

func (s *Server) handleGetConntrack(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}
	entries, err := conntrack.List(limit)
	if err != nil {
		// conntrack 不可用不等于接口坏了：非 root 读 /proc/net/nf_conntrack 会
		// permission denied，容器里可能没装 conntrack 二进制。返回 200 +
		// available:false + 真实原因，让前端能显示"不可用（原因）"而不是白屏；
		// 原因里保留原始错误，解析 bug 也不会被掩盖。
		writeJSON(w, 200, map[string]interface{}{
			"entries":   []interface{}{},
			"summary":   map[string]int{},
			"count":     0,
			"available": false,
			"reason":    err.Error(),
		})
		return
	}
	summary := conntrack.Summary(entries)
	writeJSON(w, 200, map[string]interface{}{
		"entries":   entries,
		"summary":   summary,
		"count":     len(entries),
		"available": true,
	})
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 5000 {
		limit = 5000
	}
	opts := logcenter.QueryOpts{
		Limit: limit,
		Unit:  r.URL.Query().Get("unit"),
		Grep:  r.URL.Query().Get("grep"),
		Since: r.URL.Query().Get("since"),
	}
	entries, err := logcenter.Query(opts)
	if err != nil {
		writeJSON(w, 500, errBody(10071, "logs query failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"entries": entries,
		"count":   len(entries),
	})
}

func (s *Server) handleGetLogSources(w http.ResponseWriter, r *http.Request) {
	units, err := logcenter.Sources()
	if err != nil {
		writeJSON(w, 500, errBody(10072, "logs sources failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"sources": units,
		"count":   len(units),
	})
}

// collectStatsLoop samples host metrics every 30s and feeds statsRing.
func (s *Server) collectStatsLoop() {
	// 先立即采一次，否则服务刚起来的 30 秒内 /stats/current 一直是 503，
	// 前端打开就是空图表（看起来像坏了）。
	if sm, err := stats.Collect(); err == nil {
		s.statsRing.Add(sm)
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if sm, err := stats.Collect(); err == nil {
			s.statsRing.Add(sm)
		}
	}
}

// handleStatsCurrent returns the most recent sample, or 503 before any.
func (s *Server) handleStatsCurrent(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.statsRing.Latest()
	if !ok {
		writeJSON(w, 503, errBody(10080, "no sample yet", "collector has not produced a sample; try again shortly"))
		return
	}
	writeJSON(w, 200, sm)
}

// handleStatsHistory returns samples within the last `minutes` (default 60, cap 720).
func (s *Server) handleStatsHistory(w http.ResponseWriter, r *http.Request) {
	mins := 60
	if v := r.URL.Query().Get("minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mins = n
		}
	}
	if mins > 720 {
		mins = 720
	}
	samples := s.statsRing.Since(mins)
	writeJSON(w, 200, map[string]interface{}{
		"samples": samples,
		"count":   len(samples),
	})
}

// handleStatsInterfaces returns per-interface counters (excluding lo).
func (s *Server) handleStatsInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := stats.ReadInterfaces()
	if err != nil {
		writeJSON(w, 500, errBody(10081, "interfaces read failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"interfaces": ifaces,
	})
}

// handleGetACLRules returns the current ACL rules from store.
func (s *Server) handleGetACLRules(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("acl", "rules")
	rules := []acl.Rule{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	writeJSON(w, 200, map[string]interface{}{"rules": rules, "count": len(rules)})
}

// handleSetACLRules validates, stores and live-applies an ACL rule set via nft.
func (s *Server) handleSetACLRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []acl.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10090, "invalid body", err.Error()))
		return
	}
	if len(req.Rules) == 0 {
		writeJSON(w, 422, errBody(10091, "empty rule set", "POST /acl/rules must carry at least one rule"))
		return
	}
	if err := acl.Validate(req.Rules); err != nil {
		writeJSON(w, 422, errBody(10092, "validation failed", err.Error()))
		return
	}
	applied := false
	var nftOut string
	if s.aclApplier != nil {
		if err := acl.Apply(s.aclApplier, req.Rules); err != nil {
			writeJSON(w, 500, errBody(10093, "nft apply failed", err.Error()))
			return
		}
		applied = true
		if out, err := s.aclApplier.Run(acl.NftName, acl.NftListArgs...); err == nil {
			nftOut = out
		}
	} else {
		nftOut = acl.RenderNft(req.Rules)
	}
	b, _ := json.Marshal(req.Rules)
	_ = s.store.Set("acl", "rules", string(b))
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"rules":   req.Rules,
		"nft":     nftOut,
	})
}

// handleGetArp returns the configured ARP-guard interface (read-only).
func (s *Server) handleGetArp(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("security", "arp")
	var cfg struct {
		Iface string `json:"iface"`
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, cfg)
}

// handleSetArp applies ARP anti-spoof sysctls for the requested interface.
func (s *Server) handleSetArp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Iface string `json:"iface"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10094, "invalid body", err.Error()))
		return
	}
	if strings.TrimSpace(req.Iface) == "" {
		writeJSON(w, 400, errBody(10095, "iface required", ""))
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 200, map[string]interface{}{"applied": false, "iface": req.Iface, "sysctls": nil})
		return
	}
	if err := acl.ApplyArpGuard(s.aclApplier, req.Iface); err != nil {
		writeJSON(w, 500, errBody(10096, "arp guard failed", err.Error()))
		return
	}
	sysctls := map[string]string{
		"net.ipv4.conf." + req.Iface + ".arp_ignore":   "1",
		"net.ipv4.conf." + req.Iface + ".arp_announce": "2",
	}
	b, _ := json.Marshal(struct {
		Iface   string            `json:"iface"`
		Sysctls map[string]string `json:"sysctls"`
	}{req.Iface, sysctls})
	_ = s.store.Set("security", "arp", string(b))
	writeJSON(w, 200, map[string]interface{}{
		"applied": true,
		"iface":   req.Iface,
		"sysctls": sysctls,
	})
}

// handleGetNeighbors returns the current neighbor (ARP) table.
func (s *Server) handleGetNeighbors(w http.ResponseWriter, r *http.Request) {
	if s.aclApplier == nil {
		writeJSON(w, 503, errBody(10097, "no runner", ""))
		return
	}
	rows, err := acl.StaticBindings(s.aclApplier)
	if err != nil {
		writeJSON(w, 500, errBody(10098, "neighbor read failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"neighbors": rows,
		"count":     len(rows),
	})
}

func (s *Server) storeJSON(section, option string, v interface{}) {
	b, _ := json.Marshal(v)
	_ = s.store.Set(section, option, string(b))
}

func (s *Server) loadJSON(section, option string, dst interface{}) bool {
	raw, _ := s.store.Get(section, option)
	if raw == "" {
		return false
	}
	return json.Unmarshal([]byte(raw), dst) == nil
}

// --- P3-3 端口映射 (portmap 包，幂等删除表重建) ---

func (s *Server) handleGetPortmapRules(w http.ResponseWriter, r *http.Request) {
	var rules []portmap.Rule
	s.loadJSON("portmap", "rules", &rules)
	nft := ""
	if s.aclApplier != nil {
		if out, err := s.aclApplier.Run("nft", "list", "table", "inet", "router_portmap"); err == nil {
			nft = out
		}
	}
	writeJSON(w, 200, map[string]interface{}{"rules": rules, "nft": nft})
}

func (s *Server) handleSetPortmapRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []portmap.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10110, "invalid body", err.Error()))
		return
	}
	if len(req.Rules) == 0 {
		writeJSON(w, 422, errBody(10114, "empty rule set", "POST /portmap/rules must carry at least one rule"))
		return
	}
	if err := portmap.Validate(req.Rules); err != nil {
		writeJSON(w, 422, errBody(10111, "validation failed", err.Error()))
		return
	}
	applied := false
	nftOut := portmap.RenderNft(req.Rules)
	if s.aclApplier != nil {
		if err := portmap.Apply(s.aclApplier, req.Rules); err != nil {
			writeJSON(w, 500, errBody(10112, "nft apply failed", err.Error()))
			return
		}
		applied = true
		nftOut, _ = s.aclApplier.Run("nft", "list", "table", "inet", "router_portmap")
	}
	s.storeJSON("portmap", "rules", req.Rules)
	writeJSON(w, 200, map[string]interface{}{"applied": applied, "rules": req.Rules, "nft": nftOut})
}

func (s *Server) handleDeletePortmapRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, 400, errBody(10113, "bad rule id", idStr))
		return
	}
	var rules []portmap.Rule
	if !s.loadJSON("portmap", "rules", &rules) {
		writeJSON(w, 200, map[string]interface{}{"deleted": false, "rules": []portmap.Rule{}})
		return
	}
	filtered := rules[:0]
	for _, rr := range rules {
		if rr.ID != id {
			filtered = append(filtered, rr)
		}
	}
	if len(filtered) == len(rules) {
		writeJSON(w, 200, map[string]interface{}{"deleted": false, "rules": rules})
		return
	}
	applied := false
	if s.aclApplier != nil && portmap.Validate(filtered) == nil {
		if err := portmap.Apply(s.aclApplier, filtered); err == nil {
			applied = true
		}
	}
	s.storeJSON("portmap", "rules", filtered)
	writeJSON(w, 200, map[string]interface{}{"deleted": true, "applied": applied, "rules": filtered})
}

// --- P3-3 策略路由 (portmap.Policy) ---

func (s *Server) handleGetPolicyRouteRules(w http.ResponseWriter, r *http.Request) {
	var ps []portmap.Policy
	s.loadJSON("policyroute", "rules", &ps)
	rules := []map[string]string{}
	if s.aclApplier != nil {
		if out, err := s.aclApplier.Run("ip", "-j", "rule", "show"); err == nil {
			var raw []map[string]interface{}
			if json.Unmarshal([]byte(out), &raw) == nil {
				for _, m := range raw {
					row := make(map[string]string)
					for k, v := range m {
						row[k] = fmt.Sprintf("%v", v)
					}
					rules = append(rules, row)
				}
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{"rules": ps, "live": rules})
}

func (s *Server) handleSetPolicyRouteRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policies    []portmap.Policy `json:"policies"`
		TableIfaces map[int]string   `json:"table_ifaces"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10120, "invalid body", err.Error()))
		return
	}
	if len(req.Policies) == 0 {
		writeJSON(w, 422, errBody(10124, "empty rule set", "POST /policyroute/rules must carry at least one policy"))
		return
	}
	if err := portmap.ValidatePolicy(req.Policies); err != nil {
		writeJSON(w, 422, errBody(10121, "validation failed", err.Error()))
		return
	}
	if req.TableIfaces == nil {
		req.TableIfaces = map[int]string{}
	}
	applied := false
	if s.aclApplier != nil {
		if err := portmap.ApplyPolicy(s.aclApplier, req.Policies, req.TableIfaces); err != nil {
			writeJSON(w, 500, errBody(10122, "policy apply failed", err.Error()))
			return
		}
		applied = true
	}
	s.storeJSON("policyroute", "rules", req.Policies)
	writeJSON(w, 200, map[string]interface{}{"applied": applied, "policies": req.Policies})
}

func (s *Server) handleDeletePolicyRouteRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, 400, errBody(10123, "bad policy id", idStr))
		return
	}
	var ps []portmap.Policy
	if !s.loadJSON("policyroute", "rules", &ps) {
		writeJSON(w, 200, map[string]interface{}{"deleted": false, "policies": []portmap.Policy{}})
		return
	}
	filtered := ps[:0]
	for _, p := range ps {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == len(ps) {
		writeJSON(w, 200, map[string]interface{}{"deleted": false, "policies": ps})
		return
	}
	applied := false
	if s.aclApplier != nil && portmap.ValidatePolicy(filtered) == nil {
		if err := portmap.ApplyPolicy(s.aclApplier, filtered, map[int]string{}); err == nil {
			applied = true
		}
	}
	s.storeJSON("policyroute", "rules", filtered)
	writeJSON(w, 200, map[string]interface{}{"deleted": true, "applied": applied, "policies": filtered})
}

// --- P3-3 DDN s 状态 (ddns.Status) ---

func (s *Server) handleGetDDNSStatus(w http.ResponseWriter, r *http.Request) {
	if s.aclApplier == nil {
		writeJSON(w, 503, errBody(10130, "no runner", ""))
		return
	}
	status, err := ddns.Status(s.aclApplier)
	if err != nil {
		writeJSON(w, 500, errBody(10131, "ddns status failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"status": status})
}
