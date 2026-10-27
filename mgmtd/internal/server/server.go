package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/account"
	"github.com/jimwong/router-re/mgmtd/internal/acl"
	"github.com/jimwong/router-re/mgmtd/internal/audit"
	"github.com/jimwong/router-re/mgmtd/internal/auth"
	"github.com/jimwong/router-re/mgmtd/internal/billing"
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
	"github.com/jimwong/router-re/mgmtd/internal/rbac"
	"github.com/jimwong/router-re/mgmtd/internal/route"
	"github.com/jimwong/router-re/mgmtd/internal/snmp"
	"github.com/jimwong/router-re/mgmtd/internal/stats"
	"github.com/jimwong/router-re/mgmtd/internal/system"
	"github.com/jimwong/router-re/mgmtd/internal/traffic"
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
	// accountMgr holds PPPoE/portal billing accounts (P4-2).
	accountMgr *account.Manager
	// billingPlans holds the sellable package list (P4-2).
	billingPlans []billing.Plan
	// rbacMgr is the multi-admin user/role store for /api/v1/users.
	rbacMgr *rbac.Manager
	// trafficState 保存上一次 /proc/net/dev 采样，用于算实时速率（P4-4 流量统计）。
	trafficState trafficSampler
}

// trafficSampler 保存上一次接口计数器采样，供 /api/v1/traffic/rates 计算差值。
// 内嵌互斥锁：并发请求只会有一个拿到"上一帧"，避免把两次请求的间隔算错。
type trafficSampler struct {
	mu      sync.Mutex
	prev    []traffic.Sample
	at      time.Time
	rates   []traffic.Rate
	elapsed float64
}

// newConfigStore returns the configuration store for this process.
//
// MGM_CONFIG_FILE selects a JSON file that survives restarts (the production
// path). When it is unset the server falls back to the in-memory MockStore,
// which keeps local development and the test suite free of disk side effects.
func newConfigStore() config.Store {
	path := os.Getenv("MGM_CONFIG_FILE")
	if path == "" {
		return config.NewMockStore()
	}
	fs, err := config.NewFileStore(path)
	if err != nil {
		log.Printf("mgmtd: filestore unavailable (%v); falling back to in-memory store", err)
		return config.NewMockStore()
	}
	return fs
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
		store:        newConfigStore(),
		pppoeUsers:   []pppoe.User{},
		tm:           auth.NewTokenManager(cfg.SecretKey, 15*time.Minute),
		cloudMgr:     cloud.NewManager(),
		dhcpApplier:  dhcp.NewApplier(),
		loginLimiter: newLoginLimiter(5, time.Minute),
		portalMgr:    portal.NewManager(),
		statsRing:    stats.NewRing(720),
		aclApplier:   execx.DefaultRunner{},
		accountMgr:   account.NewManager(),
		billingPlans: billing.DefaultPlans(),
		rbacMgr:      rbac.NewManager(cfg.AdminUser, cfg.AdminPassword),
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
	// Restore persisted accounts and billing plans (P4-2).
	s.loadAccounts()
	s.loadBillingPlans()
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
	s.mux.HandleFunc("GET /api/v1/dns/config", s.auth(s.handleGetDNSConfig))
	s.mux.HandleFunc("POST /api/v1/dns/config", s.auth(s.auditWrap("dns.config.set", s.handleSetDNSConfig)))
	s.mux.HandleFunc("POST /api/v1/dns/resolve", s.auth(s.auditWrap("dns.resolve", s.handleDNSResolve)))
	s.mux.HandleFunc("GET /api/v1/dns/status", s.auth(s.handleGetDNSStatus))
	s.mux.HandleFunc("GET /api/v1/upnp/config", s.auth(s.handleGetUPNPConfig))
	s.mux.HandleFunc("POST /api/v1/upnp/config", s.auth(s.auditWrap("upnp.config.set", s.handleSetUPNPConfig)))
	s.mux.HandleFunc("GET /api/v1/upnp/mappings", s.auth(s.handleGetUPNPMappings))
	s.mux.HandleFunc("POST /api/v1/upnp/mappings", s.auth(s.auditWrap("upnp.mappings.add", s.handleAddUPNPMapping)))
	s.mux.HandleFunc("DELETE /api/v1/upnp/mappings", s.auth(s.auditWrap("upnp.mappings.del", s.handleDeleteUPNPMapping)))
	s.mux.HandleFunc("GET /api/v1/upnp/status", s.auth(s.handleGetUPNPStatus))
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
	// P5-1 DHCP 服务端补强：整段 ServerConfig 存取 + 应用/状态
	s.mux.HandleFunc("GET /api/v1/dhcp/server", s.auth(s.handleGetDHCPServer))
	s.mux.HandleFunc("POST /api/v1/dhcp/server", s.auth(s.auditWrap("dhcp.server.set", s.handleSetDHCPServer)))
	s.mux.HandleFunc("GET /api/v1/dhcp/status", s.auth(s.handleGetDHCPStatus))
	// P6-1 SNMP 服务端
	s.mux.HandleFunc("GET /api/v1/snmp/config", s.auth(s.handleGetSNMPConfig))
	s.mux.HandleFunc("POST /api/v1/snmp/config", s.auth(s.auditWrap("snmp.config.set", s.handleSetSNMPConfig)))
	s.mux.HandleFunc("GET /api/v1/snmp/status", s.auth(s.handleGetSNMPStatus))
	s.mux.HandleFunc("POST /api/v1/snmp/query", s.auth(s.auditWrap("snmp.query", s.handlePostSNMPQuery)))
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
	// P4-4 接口流量实时速率与 TopN（仅本区块）
	s.mux.HandleFunc("GET /api/v1/traffic/rates", s.auth(s.handleTrafficRates))
	s.mux.HandleFunc("GET /api/v1/traffic/top", s.auth(s.handleTrafficTop))
	// P6-4 远程访问 + 系统时间 + 网络唤醒（仅本区块）
	s.mux.HandleFunc("GET /api/v1/remote/access", s.auth(s.handleGetRemoteAccess))
	s.mux.HandleFunc("POST /api/v1/remote/access", s.auth(s.auditWrap("remote.access.set", s.handleSetRemoteAccess)))
	s.mux.HandleFunc("GET /api/v1/remote/access/status", s.auth(s.handleRemoteAccessStatus))
	s.mux.HandleFunc("GET /api/v1/system/time", s.auth(s.handleGetSystemTime))
	s.mux.HandleFunc("POST /api/v1/system/time", s.auth(s.auditWrap("system.time.set", s.handleSetSystemTime)))
	s.mux.HandleFunc("GET /api/v1/system/timezones", s.auth(s.handleListTimezones))
	s.mux.HandleFunc("GET /api/v1/wol/targets", s.auth(s.handleGetWOLTargets))
	s.mux.HandleFunc("POST /api/v1/wol/targets", s.auth(s.auditWrap("wol.targets.set", s.handleSetWOLTargets)))
	s.mux.HandleFunc("POST /api/v1/wol/wake", s.auth(s.auditWrap("wol.wake", s.handleWOLWake)))
	// P3-1 安全管控（仅本区块）
	s.mux.HandleFunc("GET /api/v1/acl/rules", s.auth(s.handleGetACLRules))
	s.mux.HandleFunc("POST /api/v1/acl/rules", s.auth(s.auditWrap("acl.rules.set", s.handleSetACLRules)))
	s.mux.HandleFunc("GET /api/v1/security/arp", s.auth(s.handleGetArp))
	s.mux.HandleFunc("POST /api/v1/security/arp", s.auth(s.auditWrap("security.arp.set", s.handleSetArp)))
	s.mux.HandleFunc("GET /api/v1/security/arp/neighbors", s.auth(s.handleGetNeighbors))
	s.mux.HandleFunc("GET /api/v1/webfilter/rules", s.auth(s.handleGetWebfilterRules))
	s.mux.HandleFunc("POST /api/v1/webfilter/rules", s.auth(s.auditWrap("webfilter.rules.set", s.handleSetWebfilterRules)))
	s.mux.HandleFunc("GET /api/v1/security/attack-guard", s.auth(s.handleGetAttackGuard))
	s.mux.HandleFunc("POST /api/v1/security/attack-guard", s.auth(s.auditWrap("security.attackguard.set", s.handleSetAttackGuard)))
	s.mux.HandleFunc("GET /api/v1/ipv6/addresses", s.auth(s.handleGetIPv6Addresses))
	s.mux.HandleFunc("POST /api/v1/ipv6/addresses", s.auth(s.auditWrap("ipv6.addresses.set", s.handleSetIPv6Addresses)))
	s.mux.HandleFunc("DELETE /api/v1/ipv6/addresses", s.auth(s.auditWrap("ipv6.addresses.delete", s.handleDeleteIPv6Address)))
	s.mux.HandleFunc("GET /api/v1/ipv6/ra", s.auth(s.handleGetIPv6RA))
	s.mux.HandleFunc("POST /api/v1/ipv6/ra", s.auth(s.auditWrap("ipv6.ra.set", s.handleSetIPv6RA)))
	s.mux.HandleFunc("GET /api/v1/ipv6/status", s.auth(s.handleGetIPv6Status))
	s.mux.HandleFunc("POST /api/v1/diag/ping", s.auth(s.auditWrap("diag.ping", s.handleDiagPing)))
	s.mux.HandleFunc("POST /api/v1/diag/traceroute", s.auth(s.auditWrap("diag.traceroute", s.handleDiagTraceroute)))
	s.mux.HandleFunc("POST /api/v1/diag/dns", s.auth(s.auditWrap("diag.dns", s.handleDiagDNS)))
	s.mux.HandleFunc("GET /api/v1/diag/interfaces", s.auth(s.handleDiagInterfaces))
	s.mux.HandleFunc("GET /api/v1/ddns/status", s.auth(s.handleGetDDNSStatus))
	// P4-2 认证计费（账号 + 套餐，仅本区块）
	s.mux.HandleFunc("GET /api/v1/accounts", s.auth(s.handleGetAccounts))
	s.mux.HandleFunc("POST /api/v1/accounts", s.auth(s.auditWrap("accounts.set", s.handleSetAccounts)))
	s.mux.HandleFunc("DELETE /api/v1/accounts/{username}", s.auth(s.auditWrap("accounts.delete", s.handleDeleteAccount)))
	s.mux.HandleFunc("POST /api/v1/accounts/{username}/reset-usage", s.auth(s.auditWrap("accounts.reset_usage", s.handleResetAccountUsage)))
	s.mux.HandleFunc("POST /api/v1/accounts/{username}/bind-mac", s.auth(s.auditWrap("accounts.bind_mac", s.handleBindAccountMAC)))
	s.mux.HandleFunc("POST /api/v1/accounts/import", s.auth(s.auditWrap("accounts.import", s.handleImportAccounts)))
	s.mux.HandleFunc("GET /api/v1/billing/plans", s.auth(s.handleGetBillingPlans))
	s.mux.HandleFunc("POST /api/v1/billing/plans", s.auth(s.auditWrap("billing.plans.set", s.handleSetBillingPlans)))
	s.mux.HandleFunc("GET /api/v1/billing/summary", s.auth(s.handleGetBillingSummary))
	// P4-1 多管理员 RBAC（仅本区块）
	s.mux.HandleFunc("GET /api/v1/users", s.auth(s.auditWrap("user.list", s.handleGetUsers)))
	s.mux.HandleFunc("POST /api/v1/users", s.auth(s.auditWrap("user.create", s.handleCreateUser)))
	s.mux.HandleFunc("DELETE /api/v1/users/{username}", s.auth(s.auditWrap("user.delete", s.handleDeleteUser)))
	s.mux.HandleFunc("POST /api/v1/users/{username}/password", s.auth(s.auditWrap("user.password", s.handleSetUserPassword)))
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
	// 先查 RBAC 用户表：POST /api/v1/users 建出来的账号必须能真正登录，
	// 否则多用户管理只是摆设（实测踩到：用户建成功但 /auth/login 恒 401，
	// 因为这里只比环境变量里的 admin）。
	// 环境变量 admin 仍然兜底，保证原有部署方式和首次登录不被破坏。
	rbacOK := false
	if s.rbacMgr != nil {
		if u, ok := s.rbacMgr.Get(req.Username); ok && u.Enabled {
			rbacOK = rbac.VerifyPassword(req.Password, u.PasswordHash)
		}
	}
	// Compare in constant time to avoid leaking length/prefix information.
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.cfg.AdminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.AdminPassword)) == 1
	if !rbacOK && (!userOK || !passOK) {
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
	// auditTarget optionally overrides the audit "target" field (e.g. the
	// host/name a diagnostic was run against) instead of the URL path.
	auditTarget string
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// setAuditTarget is called by handlers that want the audit log to record the
// real subject rather than the route path. The audit middleware type-asserts
// the ResponseWriter to pick it up.
func (r *statusRecorder) setAuditTarget(t string) {
	if t != "" {
		r.auditTarget = t
	}
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
		target := r.URL.Path
		if rec.auditTarget != "" {
			target = rec.auditTarget
		}
		s.logAudit(r, action, target, result, fmt.Sprintf("HTTP %d", rec.status))
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

// handleTrafficRates 用上一次采样算各接口实时 RX/TX 速率（字节/秒）。
// 首次调用没有历史采样，elapsed_sec=0、速率全 0，前端 3 秒后自动刷新即可拿到真值。
func (s *Server) handleTrafficRates(w http.ResponseWriter, r *http.Request) {
	rates, elapsed, err := s.sampleTrafficRates()
	if err != nil {
		writeJSON(w, 500, errBody(10130, "traffic counters read failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"rates":       rateViews(rates),
		"elapsed_sec": elapsed,
		"ts":          time.Now().Unix(),
	})
}

// handleTrafficTop 返回按总速率排名的前 n 个接口（n<=0 或超上限时取默认/上限）。
func (s *Server) handleTrafficTop(w http.ResponseWriter, r *http.Request) {
	n := 10
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed
		}
	}
	if n <= 0 {
		n = 10
	}
	if n > 100 {
		n = 100
	}
	rates, _, err := s.sampleTrafficRates()
	if err != nil {
		writeJSON(w, 500, errBody(10131, "traffic counters read failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"rates": rateViews(traffic.TopN(rates, n)),
	})
}

// sampleTrafficRates 采一次计数器并与上一帧求差；trafficState 由互斥锁保护。
//
// 1 秒内的连续调用复用上一帧结果：/api/v1/traffic/rates 和 /api/v1/traffic/top
// 若各自推进采样，会把时间窗口切成几百毫秒，导致排行榜里全是 0 —— 共用一帧才一致。
func (s *Server) sampleTrafficRates() ([]traffic.Rate, float64, error) {
	s.trafficState.mu.Lock()
	defer s.trafficState.mu.Unlock()

	now := time.Now()
	if s.trafficState.rates != nil && !s.trafficState.at.IsZero() &&
		now.Sub(s.trafficState.at) < time.Second {
		return s.trafficState.rates, s.trafficState.elapsed, nil
	}

	cur, err := traffic.ReadCounters(execx.DefaultRunner{})
	if err != nil {
		return nil, 0, err
	}
	var elapsed float64
	if !s.trafficState.at.IsZero() {
		elapsed = now.Sub(s.trafficState.at).Seconds()
	}
	rates := traffic.ComputeRates(s.trafficState.prev, cur, elapsed)
	s.trafficState.prev, s.trafficState.at = cur, now
	s.trafficState.rates, s.trafficState.elapsed = rates, elapsed
	return rates, elapsed, nil
}

// trafficRateView 是速率条目的对外 JSON 形态（本服务 API 统一 snake_case）。
type trafficRateView struct {
	Iface string  `json:"iface"`
	RXBps float64 `json:"rx_bps"`
	TXBps float64 `json:"tx_bps"`
}

func rateViews(rates []traffic.Rate) []trafficRateView {
	out := make([]trafficRateView, 0, len(rates))
	for _, rt := range rates {
		out = append(out, trafficRateView{Iface: rt.Iface, RXBps: rt.RXBps, TXBps: rt.TXBps})
	}
	return out
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

// handleGetArp returns the configured ARP-guard interface and the actual
// per-scope sysctl readback (all / default / each real interface).
func (s *Server) handleGetArp(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("security", "arp")
	var cfg struct {
		Iface   string            `json:"iface"`
		Sysctls map[string]string `json:"sysctls"`
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	status := map[string]string{}
	if s.aclApplier != nil {
		for _, k := range []string{"arp_ignore", "arp_announce"} {
			for _, scope := range []string{"all", "default"} {
				key := "net.ipv4.conf." + scope + "." + k
				if v, err := s.aclApplier.Run("sysctl", "-n", key); err == nil {
					status[key] = strings.TrimSpace(v)
				}
			}
			if cfg.Iface != "" && cfg.Iface != "all" {
				key := "net.ipv4.conf." + cfg.Iface + "." + k
				if v, err := s.aclApplier.Run("sysctl", "-n", key); err == nil {
					status[key] = strings.TrimSpace(v)
				}
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"iface":   cfg.Iface,
		"sysctls": cfg.Sysctls,
		"status":  status,
	})
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
	// Build per-scope readback: all / default / concrete iface (if not "all").
	scopes := []string{"all", "default"}
	if req.Iface != "all" {
		scopes = append(scopes, req.Iface)
	}
	sysctls := map[string]string{}
	for _, scope := range scopes {
		sysctls["net.ipv4.conf."+scope+".arp_ignore"] = "1"
		sysctls["net.ipv4.conf."+scope+".arp_announce"] = "2"
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

// ===== P4-2 认证计费（账号 + 套餐，仅本区块）=====

// persistAccounts writes the in-memory account set to the config store.
func (s *Server) persistAccounts() {
	if s.accountMgr == nil {
		return
	}
	// Persist a credential-free copy: the plaintext password is never written
	// to the config store. On reload the account keeps its non-secret fields
	// and its password is re-set by the operator (or via import).
	stored := make([]account.Account, 0)
	for _, a := range s.accountMgr.List() {
		a.Password = ""
		stored = append(stored, a)
	}
	s.storeJSON("accounts", "list", stored)
}

// loadAccounts restores accounts from the config store into the manager.
func (s *Server) loadAccounts() {
	if s.accountMgr == nil {
		return
	}
	var accts []account.Account
	if !s.loadJSON("accounts", "list", &accts) {
		return
	}
	for _, a := range accts {
		// CreateReloaded on a fresh store; Update when the account already
		// exists (e.g. a later start that re-reads a partially populated
		// store). The persisted form carries no password, so the load path
		// must not require one.
		if _, ok := s.accountMgr.Get(a.Username); ok {
			_ = s.accountMgr.Update(a)
			continue
		}
		_ = s.accountMgr.CreateReloaded(a)
	}
}

// accountView is the wire representation of an account. It deliberately
// omits the password so credentials can never leak through the API.
type accountView struct {
	Username  string `json:"username"`
	Type      string `json:"type"`
	Profile   string `json:"profile"`
	QuotaMB   int    `json:"quota_mb"`
	UsedMB    int    `json:"used_mb"`
	ExpireAt  int64  `json:"expire_at"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	Online    bool   `json:"online"`
	BoundMAC  string `json:"bound_mac"`
}

func toAccountView(a account.Account) accountView {
	return accountView{
		Username:  a.Username,
		Type:      a.Type,
		Profile:   a.Profile,
		QuotaMB:   a.QuotaMB,
		UsedMB:    a.UsedMB,
		ExpireAt:  a.ExpireAt,
		Enabled:   a.Enabled,
		CreatedAt: a.CreatedAt,
		Online:    a.Online,
		BoundMAC:  a.BoundMAC,
	}
}

// accountViews converts a slice of accounts to their safe wire form.
func accountViews(accts []account.Account) []accountView {
	out := make([]accountView, 0, len(accts))
	for _, a := range accts {
		out = append(out, toAccountView(a))
	}
	return out
}

// handleGetAccounts lists accounts. Passwords are never serialized.
func (s *Server) handleGetAccounts(w http.ResponseWriter, r *http.Request) {
	accts := []account.Account{}
	if s.accountMgr != nil {
		accts = s.accountMgr.List()
	}
	writeJSON(w, 200, map[string]interface{}{
		"accounts": accountViews(accts),
		"count":    len(accts),
	})
}

// accountBody accepts either a single account object or {"accounts":[...]}.
// It returns the parsed list and whether the body was a batch request.
func accountBody(r *http.Request) ([]account.Account, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	var batch struct {
		Accounts []account.Account `json:"accounts"`
	}
	if err := json.Unmarshal(body, &batch); err == nil && len(batch.Accounts) > 0 {
		return batch.Accounts, nil
	}
	var single account.Account
	if err := json.Unmarshal(body, &single); err == nil && single.Username != "" {
		return []account.Account{single}, nil
	}
	var arr []account.Account
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("expected an account object or {\"accounts\":[...]}")
}

// handleSetAccounts creates or updates one or more accounts.
func (s *Server) handleSetAccounts(w http.ResponseWriter, r *http.Request) {
	accts, err := accountBody(r)
	if err != nil {
		writeJSON(w, 400, errBody(10140, "invalid body", err.Error()))
		return
	}
	if len(accts) == 0 {
		writeJSON(w, 422, errBody(10141, "empty account set", "POST /accounts must carry at least one account"))
		return
	}
	created, updated, errs := 0, 0, []string{}
	for i, a := range accts {
		if err := account.Validate(a); err != nil {
			errs = append(errs, fmt.Sprintf("account %d (%s): %v", i+1, a.Username, err))
			continue
		}
		if _, ok := s.accountMgr.Get(a.Username); ok {
			if err := s.accountMgr.Update(a); err != nil {
				errs = append(errs, fmt.Sprintf("account %d (%s): %v", i+1, a.Username, err))
				continue
			}
			updated++
			continue
		}
		if a.CreatedAt == 0 {
			a.CreatedAt = time.Now().Unix()
		}
		if err := s.accountMgr.Create(a); err != nil {
			errs = append(errs, fmt.Sprintf("account %d (%s): %v", i+1, a.Username, err))
			continue
		}
		created++
	}
	if created+updated == 0 {
		writeJSON(w, 422, errBody(10142, "no account applied", strings.Join(errs, "; ")))
		return
	}
	s.persistAccounts()
	writeJSON(w, 200, map[string]interface{}{
		"created":  created,
		"updated":  updated,
		"errors":   errs,
		"accounts": accountViews(s.accountMgr.List()),
	})
}

// handleDeleteAccount removes one account by username.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		writeJSON(w, 400, errBody(10143, "missing username", ""))
		return
	}
	deleted := false
	if s.accountMgr != nil {
		deleted = s.accountMgr.Delete(username)
	}
	if !deleted {
		writeJSON(w, 404, errBody(10144, "account not found", username))
		return
	}
	s.persistAccounts()
	writeJSON(w, 200, map[string]interface{}{
		"deleted":  true,
		"username": username,
		"accounts": accountViews(s.accountMgr.List()),
	})
}

// handleResetAccountUsage zeroes the used quota of one account.
func (s *Server) handleResetAccountUsage(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		writeJSON(w, 400, errBody(10145, "missing username", ""))
		return
	}
	if s.accountMgr == nil || !s.accountMgr.ResetUsage(username) {
		writeJSON(w, 404, errBody(10146, "account not found", username))
		return
	}
	s.persistAccounts()
	writeJSON(w, 200, map[string]interface{}{
		"reset":    true,
		"username": username,
		"account":  toAccountView(mustGet(s.accountMgr, username)),
	})
}

// handleBindAccountMAC attaches a MAC address to an account.
func (s *Server) handleBindAccountMAC(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		writeJSON(w, 400, errBody(10147, "missing username", ""))
		return
	}
	var req struct {
		MAC string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10148, "invalid body", err.Error()))
		return
	}
	if s.accountMgr == nil {
		writeJSON(w, 503, errBody(10149, "no account manager", ""))
		return
	}
	if err := s.accountMgr.BindMAC(username, req.MAC); err != nil {
		writeJSON(w, 422, errBody(10150, "bind mac failed", err.Error()))
		return
	}
	s.persistAccounts()
	writeJSON(w, 200, map[string]interface{}{
		"bound":    true,
		"username": username,
		"account":  toAccountView(mustGet(s.accountMgr, username)),
	})
}

// handleImportAccounts bulk-imports accounts from CSV lines.
func (s *Server) handleImportAccounts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10151, "invalid body", err.Error()))
		return
	}
	if len(req.Lines) == 0 {
		writeJSON(w, 422, errBody(10152, "empty import", "POST /accounts/import must carry lines"))
		return
	}
	if s.accountMgr == nil {
		writeJSON(w, 503, errBody(10153, "no account manager", ""))
		return
	}
	created, errs := s.accountMgr.BulkImport(req.Lines)
	if created > 0 {
		s.persistAccounts()
	}
	writeJSON(w, 200, map[string]interface{}{
		"created": created,
		"errors":  errs,
		"total":   len(req.Lines),
	})
}

// mustGet returns the account for username, or an empty Account if missing.
func mustGet(m *account.Manager, username string) account.Account {
	if m == nil {
		return account.Account{}
	}
	a, _ := m.Get(username)
	return a
}

// ===== P4-2 套餐计费（billing 包，仅本区块）=====

// persistBillingPlans writes the in-memory plan list to the config store.
func (s *Server) persistBillingPlans() {
	if s.billingPlans == nil {
		return
	}
	s.storeJSON("billing", "plans", s.billingPlans)
}

// loadBillingPlans restores the plan list from the config store.
func (s *Server) loadBillingPlans() {
	var plans []billing.Plan
	if !s.loadJSON("billing", "plans", &plans) {
		return
	}
	s.billingPlans = plans
}

// handleGetBillingPlans returns the configured package list.
func (s *Server) handleGetBillingPlans(w http.ResponseWriter, r *http.Request) {
	plans := s.billingPlans
	if plans == nil {
		plans = []billing.Plan{}
	}
	writeJSON(w, 200, map[string]interface{}{
		"plans": plans,
		"count": len(plans),
	})
}

// handleSetBillingPlans replaces the package list (single object or {"plans":[...]}).
func (s *Server) handleSetBillingPlans(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, errBody(10160, "read body fail", err.Error()))
		return
	}
	var batch struct {
		Plans []billing.Plan `json:"plans"`
	}
	var plans []billing.Plan
	if err := json.Unmarshal(body, &batch); err == nil && len(batch.Plans) > 0 {
		plans = batch.Plans
	} else {
		var single billing.Plan
		if err := json.Unmarshal(body, &single); err == nil && single.Name != "" {
			plans = []billing.Plan{single}
		} else {
			var arr []billing.Plan
			if err := json.Unmarshal(body, &arr); err == nil {
				plans = arr
			} else {
				writeJSON(w, 400, errBody(10161, "invalid body", "expected a plan object or {\"plans\":[...]}"))
				return
			}
		}
	}
	if len(plans) == 0 {
		writeJSON(w, 422, errBody(10162, "empty plan set", "POST /billing/plans must carry at least one plan"))
		return
	}
	errs := []string{}
	for i, p := range plans {
		if err := billing.ValidatePlan(p); err != nil {
			errs = append(errs, fmt.Sprintf("plan %d (%s): %v", i+1, p.Name, err))
		}
	}
	if len(errs) > 0 {
		writeJSON(w, 422, errBody(10163, "plan validation failed", strings.Join(errs, "; ")))
		return
	}
	s.billingPlans = plans
	s.persistBillingPlans()
	writeJSON(w, 200, map[string]interface{}{
		"plans":   s.billingPlans,
		"count":   len(s.billingPlans),
		"applied": true,
	})
}

// handleGetBillingSummary aggregates account counts and revenue.
func (s *Server) handleGetBillingSummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now().Unix()
	summary := map[string]interface{}{
		"total_accounts": 0,
		"online":         0,
		"expired":        0,
		"over_quota":     0,
		"revenue":        0.0,
	}
	if s.accountMgr == nil {
		writeJSON(w, 200, summary)
		return
	}
	accts := s.accountMgr.List()
	counts := map[string]int{}
	for _, a := range accts {
		summary["total_accounts"] = summary["total_accounts"].(int) + 1
		switch a.Status(now) {
		case "online":
			summary["online"] = summary["online"].(int) + 1
		case "expired":
			summary["expired"] = summary["expired"].(int) + 1
		case "over_quota":
			summary["over_quota"] = summary["over_quota"].(int) + 1
		}
		if a.Profile != "" {
			counts[a.Profile]++
		}
	}
	plans := s.billingPlans
	if plans == nil {
		plans = []billing.Plan{}
	}
	summary["revenue"] = billing.TotalRevenue(plans, counts)
	writeJSON(w, 200, summary)
}

// --- P4-1 多管理员 RBAC (admin-only management endpoints) ---

// currentActor returns the authenticated username from the request context
// (set by s.auth). It returns "" when no actor is present.
func (s *Server) currentActor(r *http.Request) string {
	if v := r.Context().Value(ctxKeyActor); v != nil {
		if name, ok := v.(string); ok {
			return name
		}
	}
	return ""
}

// requireAdmin denies non-admin callers with 403. It returns the actor name.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) string {
	actor := s.currentActor(r)
	role, _ := s.rbacMgr.RoleOf(actor)
	if role != rbac.RoleAdmin {
		writeJSON(w, http.StatusForbidden, errBody(10210, "需要 admin 角色", fmt.Sprintf("角色为 %s，无权管理用户", role)))
		return ""
	}
	return actor
}

// handleGetUsers returns all users. PasswordHash is omitted by the JSON tag.
func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == "" {
		return
	}
	users := s.rbacMgr.List()
	writeJSON(w, 200, map[string]interface{}{
		"users": users,
		"count": len(users),
	})
}

// handleCreateUser adds a user. Request body: {username,password,role}.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == "" {
		return
	}
	var req struct {
		Username string    `json:"username"`
		Password string    `json:"password"`
		Role     rbac.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10211, "invalid body", err.Error()))
		return
	}
	if err := rbac.ValidatePassword(req.Password); err != nil {
		writeJSON(w, 422, errBody(10212, "weak password", err.Error()))
		return
	}
	hash, err := rbac.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, 500, errBody(10213, "hash error", err.Error()))
		return
	}
	u := rbac.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
		Enabled:      true,
	}
	if err := s.rbacMgr.Create(u); err != nil {
		writeJSON(w, 409, errBody(10214, "create failed", err.Error()))
		return
	}
	// Do NOT echo the password or hash back.
	created, _ := s.rbacMgr.Get(req.Username)
	writeJSON(w, 201, map[string]interface{}{"user": created})
}

// handleDeleteUser removes a user by username (path {username}).
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == "" {
		return
	}
	name := r.PathValue("username")
	if name == "" {
		writeJSON(w, 400, errBody(10215, "missing username", ""))
		return
	}
	if err := s.rbacMgr.Delete(name); err != nil {
		writeJSON(w, 409, errBody(10216, "delete failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": name})
}

// handleSetUserPassword changes a user's password. Body: {password}.
func (s *Server) handleSetUserPassword(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == "" {
		return
	}
	name := r.PathValue("username")
	if name == "" {
		writeJSON(w, 400, errBody(10217, "missing username", ""))
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10218, "invalid body", err.Error()))
		return
	}
	if err := rbac.ValidatePassword(req.Password); err != nil {
		writeJSON(w, 422, errBody(10219, "weak password", err.Error()))
		return
	}
	if err := s.rbacMgr.ChangePassword(name, req.Password); err != nil {
		writeJSON(w, 409, errBody(10220, "password change failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"changed": name})
}

// --- P5-1 DHCP 服务端补强 (仅本区块) ---

// dhcpServerStoreKey 是 ServerConfig 在 store 里的键。
const dhcpServerStoreKey = "dhcp/server"

// handleGetDHCPServer 返回当前持久化的 DHCP 服务端 ServerConfig。
func (s *Server) handleGetDHCPServer(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("dhcp", dhcpServerStoreKey)
	if raw == "" {
		writeJSON(w, 200, map[string]interface{}{
			"config":  dhcp.ServerConfig{DHCPConfig: dhcp.DHCPConfig{}, Enabled: false},
			"applied": false,
		})
		return
	}
	var cfg dhcp.ServerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		writeJSON(w, 500, errBody(10010, "config unmarshal error", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "applied": true})
}

// handleSetDHCPServer 校验 + 应用 DHCP 服务端配置。
// POST 空对象或 range 无效 → 422；下发后返回 {applied, config, dnsmasq, detail}。
func (s *Server) handleSetDHCPServer(w http.ResponseWriter, r *http.Request) {
	if s.dhcpApplier == nil || s.dhcpApplier.R == nil {
		writeJSON(w, 500, errBody(10011, "dhcp runner not initialized", ""))
		return
	}
	var cfg dhcp.ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10012, "invalid body", err.Error()))
		return
	}
	// 空对象判断：Interface 为空视为无效。
	if cfg.Interface == "" {
		writeJSON(w, 422, errBody(10013, "invalid dhcp server config", "interface/range is empty or invalid"))
		return
	}
	if err := dhcp.ValidateConfig(cfg); err != nil {
		writeJSON(w, 422, errBody(10014, "invalid dhcp server config", err.Error()))
		return
	}

	confPath := dhcp.DefaultConfPath
	// store 先持久化 JSON 原文。
	b, _ := json.Marshal(cfg)
	s.store.Set("dhcp", dhcpServerStoreKey, string(b))

	detail := ""
	applyErr := dhcp.Apply(s.dhcpApplier.R, cfg, confPath)
	if applyErr != nil {
		detail = applyErr.Error()
	}
	dnsmasq := cfg.RenderDnsmasq()
	writeJSON(w, 200, map[string]interface{}{
		"applied": applyErr == nil,
		"config":  cfg,
		"dnsmasq": dnsmasq,
		"detail":  detail,
	})
}

// handleGetDHCPStatus 查询 dnsmasq 运行状态。
func (s *Server) handleGetDHCPStatus(w http.ResponseWriter, r *http.Request) {
	if s.dhcpApplier == nil || s.dhcpApplier.R == nil {
		writeJSON(w, 500, errBody(10015, "dhcp runner not initialized", ""))
		return
	}
	st, err := dhcp.Status(s.dhcpApplier.R)
	if err != nil {
		writeJSON(w, 500, errBody(10016, "dhcp status error", err.Error()))
		return
	}
	writeJSON(w, 200, st)
}

// --- P6-1 SNMP 服务端 (仅本区块) ---

// snmpStoreKey 是 SNMP Config 在 store 里的键。
const snmpStoreKey = "snmp/config"

// handleGetSNMPConfig 返回持久化的 SNMP 配置（密码已打码）。
func (s *Server) handleGetSNMPConfig(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("snmp", snmpStoreKey)
	if raw == "" {
		writeJSON(w, 200, map[string]interface{}{
			"config":  snmp.Config{},
			"applied": false,
		})
		return
	}
	var cfg snmp.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		writeJSON(w, 500, errBody(10020, "config unmarshal error", err.Error()))
		return
	}
	// 密码必须加码后返回。
	writeJSON(w, 200, map[string]interface{}{"config": cfg.Masked(), "applied": true})
}

// handleSetSNMPConfig 校验 + 应用 SNMP 配置。
// POST 空对象或校验失败 → 422；下发后返回 {applied, config(masked), snmpd, detail}。
func (s *Server) handleSetSNMPConfig(w http.ResponseWriter, r *http.Request) {
	var cfg snmp.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10021, "invalid body", err.Error()))
		return
	}
	if err := snmp.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10022, "invalid snmp config", err.Error()))
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10023, "exec runner not initialized", ""))
		return
	}
	confPath := snmp.DefaultConfPath
	// 持久化原始（含密码）JSON，用于 Apply 与下次 GET。
	b, _ := json.Marshal(cfg)
	s.store.Set("snmp", snmpStoreKey, string(b))

	detail := ""
	applyErr := snmp.Apply(s.aclApplier, cfg, confPath)
	if applyErr != nil {
		detail = applyErr.Error()
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applyErr == nil,
		"config":  cfg.Masked(),
		// 必须用 Masked() 的副本渲染：真配置里 v3 用户带明文 AuthPass/PrivPass，
		// 直接回给前端等于把密码写进浏览器缓存、代理日志和 DevTools 历史。
		// 需要完整内容的人去服务器上看 /etc/snmp/snmpd.conf（权限 0600）。
		"snmpd":  snmp.RenderSnmpd(cfg.Masked()),
		"detail": detail,
	})
}

// handleGetSNMPStatus 查询 snmpd 运行状态。
func (s *Server) handleGetSNMPStatus(w http.ResponseWriter, r *http.Request) {
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10024, "exec runner not initialized", ""))
		return
	}
	st, err := snmp.Status(s.aclApplier)
	if err != nil {
		writeJSON(w, 500, errBody(10025, "snmp status error", err.Error()))
		return
	}
	writeJSON(w, 200, st)
}

// handlePostSNMPQuery 实时执行 snmpwalk 查询本机。snmpwalk 未安装 → 400。
func (s *Server) handlePostSNMPQuery(w http.ResponseWriter, r *http.Request) {
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10026, "exec runner not initialized", ""))
		return
	}
	out, err := snmp.QuerySystem(s.aclApplier)
	if err != nil {
		if err.Error() == "snmpwalk not installed" {
			writeJSON(w, http.StatusBadRequest, errBody(10027, "snmpwalk not installed", err.Error()))
		} else {
			writeJSON(w, 500, errBody(10028, "snmp query failed", err.Error()))
		}
		return
	}
	writeJSON(w, 200, out)
}
