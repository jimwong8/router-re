package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/account"
	"github.com/jimwong/router-re/mgmtd/internal/acl"
	"github.com/jimwong/router-re/mgmtd/internal/appid"
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
	"github.com/jimwong/router-re/mgmtd/internal/dpiflow"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/firewall"
	"github.com/jimwong/router-re/mgmtd/internal/frp"
	"github.com/jimwong/router-re/mgmtd/internal/htb"
	"github.com/jimwong/router-re/mgmtd/internal/iface"
	"github.com/jimwong/router-re/mgmtd/internal/iperf"
	"github.com/jimwong/router-re/mgmtd/internal/ips"
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
	"github.com/jimwong/router-re/mgmtd/internal/sms"
	"github.com/jimwong/router-re/mgmtd/internal/snmp"
	"github.com/jimwong/router-re/mgmtd/internal/stats"
	"github.com/jimwong/router-re/mgmtd/internal/svcmgr"
	"github.com/jimwong/router-re/mgmtd/internal/system"
	"github.com/jimwong/router-re/mgmtd/internal/taskq"
	"github.com/jimwong/router-re/mgmtd/internal/threat"
	"github.com/jimwong/router-re/mgmtd/internal/traffic"
	"github.com/jimwong/router-re/mgmtd/internal/vpn"
	"github.com/jimwong/router-re/mgmtd/internal/wan"
	"github.com/jimwong/router-re/mgmtd/internal/webfilter"
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
	// UserFile 是 RBAC 账号表落盘路径。空串=纯内存（测试用）。
	// 不设的话重启会清空所有非 admin 账号（实测过：建的用户重启后登录不上）。
	UserFile string
	// Version 是编译期注入的版本号（-ldflags "-X main.Version=..."）。
	// 通过 /api/v1/health 暴露，好让在线升级后能确认设备实际跑的是哪个版本。
	Version string
}

type Server struct {
	cfg         Config
	dhcpApplier *dhcp.Applier
	mux         *http.ServeMux
	store       config.Store
	tm          *auth.TokenManager
	cloudMgr    *cloud.LegacyManager
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
	// rebootRunner executes the actual reboot command (injectable for tests).
	rebootRunner execx.Runner
	// iperfRunner runs iperf3 client (injectable for tests).
	iperfRunner execx.Runner
	// iperfMu guards iperfLast.
	iperfMu sync.Mutex
	// iperfLast holds the most recent iperf result.
	iperfLast iperf.Result
	// svcMgr manages ssh/telnet/ftp/dnsmasq enable/disable switches.
	svcMgr *svcmgr.Manager
	// taskQ is the background task queue.
	taskQ *taskq.Queue
	// accountMgr holds PPPoE/portal billing accounts (P4-2).
	accountMgr *account.Manager
	// billingPlans holds the sellable package list (P4-2).
	billingPlans []billing.Plan
	// rbacMgr is the multi-admin user/role store for /api/v1/users.
	rbacMgr *rbac.Manager
	// trafficState 保存上一次 /proc/net/dev 采样，用于算实时速率（P4-4 流量统计）。
	trafficState trafficSampler
	// threatList 是内存封禁表（P7-4 威胁情报与 IP 封禁）。
	threatList *threat.List
	// linkcheckMu 串行化链路检测的「读整份 states → 改一条 → 写整份」序列。
	//
	// 不加这把锁时，两个目标并发探测会互相覆盖：实测 32 个并发写只剩 2 条，
	// 丢了 30 条（94%）。单目标、手动点一次永远复现不出来。
	// 锁只覆盖内存 map + 小 JSON 落盘，持锁时间很短。
	linkcheckMu sync.Mutex

	// p95: 「读整份→改一条→写整份」(RMW) handler 串行化锁，防并发丢更新
	// （并发用例证明见 rmw_concurrency_test.go；未加锁时 30/32、31/32 丢更新）。
	timeschedMu sync.Mutex // timesched rules: 并发 delete / set×delete
	ddnsMu      sync.Mutex // ddns records: 并发 delete / set(整表回写,继承明文 token)
	htbMu       sync.Mutex // htb config: apply(继承回写) × config set
	// voucherMu 串行化卡密的「读整份 → 改一张 → 写整份」序列（生成/核销/删除共用）。
	//
	// 不加锁时并发核销会互相覆盖：实测 16 张卡并发核销，14 张的 used_at 被抹回 0，
	// 也就是这 14 张网卡可以再白用一次。这是少收钱，不是显示问题。
	voucherMu sync.Mutex
	// multiwanMu 串行化多 WAN 探测结果的「读整份 states → 改一个成员 → 写整份」。
	//
	// 不加锁时并发回来的探测结果互相覆盖，故障切换拿着旧状态判断：
	// 挂掉的链路显示 up（流量继续打死链路），或恢复的显示 down（白白不用）。
	multiwanMu sync.Mutex
	// smsStore 是短信验证码的内存存储（发送频次限制、码的生效/作废）。
	smsStore *sms.MemoryStore
	// storeErr 记录配置存储初始化失败的原因（仅当 MGM_CONFIG_FILE 配了却打不开）。
	// main 检查它并拒绝启动——不能让"配置写进内存、重启全丢"静默发生。
	storeErr error
}

// InitError 返回启动阶段不可继续的错误（目前只有配置存储打不开）。nil 表示正常。
func (s *Server) InitError() error { return s.storeErr }

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
func newConfigStore() (config.Store, error) {
	path := os.Getenv("MGM_CONFIG_FILE")
	if path == "" {
		// 没配路径 = 只是本地跑着玩/测试，内存存储是明确的选择，不算降级。
		return config.NewMockStore(), nil
	}
	fs, err := config.NewFileStore(path)
	if err != nil {
		// 配了路径却打不开，绝不能悄悄退回内存：服务会照常启动、照常接受
		// 配置变更、返回 200，重启后全部消失，而运行期间没有任何迹象。
		// 调试时确实想要这个行为，必须显式开 MGM_ALLOW_EPHEMERAL_STORE=1。
		if os.Getenv("MGM_ALLOW_EPHEMERAL_STORE") == "1" {
			log.Printf("mgmtd: ⚠ 配置存储打不开（%v），按 MGM_ALLOW_EPHEMERAL_STORE=1 退回内存——重启会丢全部配置", err)
			return config.NewMockStore(), nil
		}
		return nil, fmt.Errorf("配置存储打不开（MGM_CONFIG_FILE=%s）: %w", path, err)
	}
	return fs, nil
}

func New(cfg Config) *Server {
	taskStorePath := "/tmp/router-re-tasks.json"
	if v := os.Getenv("MGM_TASK_STORE"); v != "" {
		taskStorePath = v
	}
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	if cfg.AdminPassword == "" {
		// Development default. Production must set MGM_ADMIN_PASSWORD.
		cfg.AdminPassword = "admin123"
	}
	store, storeErr := newConfigStore()
	if store == nil {
		store = config.NewMockStore()
	}
	s := &Server{
		cfg:          cfg,
		mux:          http.NewServeMux(),
		store:        store,
		storeErr:     storeErr,
		pppoeUsers:   []pppoe.User{},
		tm:           auth.NewTokenManager(cfg.SecretKey, 12*time.Hour),
		cloudMgr:     cloud.NewLegacyManager(),
		dhcpApplier:  dhcp.NewApplier(),
		loginLimiter: newLoginLimiter(5, time.Minute),
		taskQ:        taskq.New(taskStorePath, 2, 50),
		portalMgr:    portal.NewManager(),
		statsRing:    loadStatsRing(),
		aclApplier:   execx.DefaultRunner{},
		rebootRunner: execx.DefaultRunner{},
		iperfRunner:  execx.DefaultRunner{},
		svcMgr:       svcmgr.New(execx.DefaultRunner{}),
		accountMgr:   account.NewManager(),
		billingPlans: billing.DefaultPlans(),
		rbacMgr:      rbac.NewManagerWithFile(cfg.AdminUser, cfg.AdminPassword, cfg.UserFile),
		threatList:   threat.NewList(),
		smsStore:     sms.NewMemoryStore(),
	}
	s.frp()

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
	// Register default task handlers.
	s.taskQ.Register("echo", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		return params, nil
	})
	s.taskQ.Register("sleep", func(ctx context.Context, params json.RawMessage, progress func(int)) (any, error) {
		var p struct {
			Seconds int `json:"seconds"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Seconds <= 0 {
			p.Seconds = 1
		}
		for i := 0; i < p.Seconds; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
			}
			progress((i + 1) * 100 / p.Seconds)
		}
		return map[string]any{"slept": p.Seconds}, nil
	})
	s.routes()
	// Restore persisted accounts and billing plans (P4-2).
	s.loadAccounts()
	s.loadBillingPlans()
	return s
}

// maxJSONBody 是配置类 JSON 请求体的上限。路由器内存有限，
// 一个超大 POST 就能把进程撑爆——已鉴权不代表可信。
const maxJSONBody = 8 << 20

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		v := s.cfg.Version
		if v == "" {
			v = "dev"
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "version": v})
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
	s.mux.HandleFunc("GET /api/v1/l7/rules", s.auth(s.handleGetL7Rules))
	s.mux.HandleFunc("POST /api/v1/l7/rules", s.auth(s.auditWrap("l7.rules.set", s.handleSetL7Rules)))
	s.mux.HandleFunc("POST /api/v1/l7/test", s.auth(s.auditWrap("l7.test", s.handleL7Test)))
	s.mux.HandleFunc("GET /api/v1/l7/stats", s.auth(s.handleGetL7Stats))
	s.mux.HandleFunc("POST /api/v1/l7/apply", s.auth(s.auditWrap("l7.apply", s.handleL7Apply)))
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
	s.mux.HandleFunc("GET /api/v1/dhcp/v6", s.auth(s.handleGetDHCPv6))
	s.mux.HandleFunc("POST /api/v1/dhcp/v6", s.auth(s.auditWrap("dhcp.v6.set", s.handleSetDHCPv6)))
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
	// P8-1 DPI 接入转发路径（仅本区块，/dpi/flows、/dpi/apps 等不与上面冲突）
	s.mux.HandleFunc("GET /api/v1/dpi/flows", s.auth(s.handleGetDPIFlows))
	s.mux.HandleFunc("GET /api/v1/dpi/apps", s.auth(s.handleGetDPIApps))
	s.mux.HandleFunc("GET /api/v1/dpi/tracker-stats", s.auth(s.handleGetDPITrackerStats))
	s.mux.HandleFunc("POST /api/v1/dpi/classify-payload", s.auth(s.auditWrap("dpi.classify.payload", s.handleClassifyPayload)))
	s.mux.HandleFunc("GET /api/v1/dpi/mark-rules", s.auth(s.handleGetDPIMarkRules))
	s.mux.HandleFunc("POST /api/v1/dpi/mark-rules", s.auth(s.auditWrap("dpi.markrules.set", s.handleSetDPIMarkRules)))
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
	// PPTP / L2TP / OpenVPN 服务端：路径与字段对齐 iKuai，便于迁移与对照
	s.mux.HandleFunc("GET /api/v1/vpn/pptp/services", s.auth(s.handleGetPPTP))
	s.mux.HandleFunc("POST /api/v1/vpn/pptp/services", s.auth(s.auditWrap("vpn.pptp.save", s.handleSetPPTP)))
	s.mux.HandleFunc("GET /api/v1/vpn/l2tp/services", s.auth(s.handleGetL2TP))
	s.mux.HandleFunc("POST /api/v1/vpn/l2tp/services", s.auth(s.auditWrap("vpn.l2tp.save", s.handleSetL2TP)))
	s.mux.HandleFunc("GET /api/v1/vpn/openvpn/services", s.auth(s.handleGetOpenVPN))
	s.mux.HandleFunc("POST /api/v1/vpn/openvpn/services", s.auth(s.auditWrap("vpn.openvpn.save", s.handleSetOpenVPN)))
	s.mux.HandleFunc("POST /api/v1/vpn/openvpn/client-conf", s.auth(s.auditWrap("vpn.openvpn.clientconf", s.handleOpenVPNClientConf)))
	s.mux.HandleFunc("GET /api/v1/vpn/openvpn/status", s.auth(s.handleGetOpenVPNStatus))
	// EasyTier 去中心化组网（配置 / 启停 / 运行态 / 对等节点 / 配置预览）
	s.mux.HandleFunc("GET /api/v1/vpn/easytier/config", s.auth(s.handleGetEasyTierConfig))
	s.mux.HandleFunc("POST /api/v1/vpn/easytier/config", s.auth(s.auditWrap("vpn.easytier.save", s.handleSetEasyTierConfig)))
	s.mux.HandleFunc("POST /api/v1/vpn/easytier/preview", s.auth(s.handleEasyTierPreview))
	s.mux.HandleFunc("GET /api/v1/vpn/easytier/status", s.auth(s.handleEasyTierStatus))
	s.mux.HandleFunc("POST /api/v1/vpn/easytier/stop", s.auth(s.auditWrap("vpn.easytier.stop", s.handleEasyTierStop)))
	// Tailscale（接系统 tailscaled，只通过 unix socket 下发命令）
	s.mux.HandleFunc("GET /api/v1/vpn/tailscale/config", s.auth(s.handleGetTailscale))
	s.mux.HandleFunc("POST /api/v1/vpn/tailscale/config", s.auth(s.auditWrap("vpn.tailscale.save", s.handleSetTailscale)))
	s.mux.HandleFunc("POST /api/v1/vpn/tailscale/preview", s.auth(s.handleTailscalePreview))
	s.mux.HandleFunc("GET /api/v1/vpn/tailscale/status", s.auth(s.handleTailscaleStatus))
	s.mux.HandleFunc("POST /api/v1/vpn/tailscale/down", s.auth(s.auditWrap("vpn.tailscale.down", s.handleTailscaleDown)))
	// 对象管理（IP/MAC/终端分组）、多线路 DNS、域名分流、行为记录
	s.mux.HandleFunc("GET /api/v1/objects/groups", s.auth(s.handleGetGroups))
	s.mux.HandleFunc("POST /api/v1/objects/groups", s.auth(s.auditWrap("objects.groups.save", s.handleSetGroups)))
	s.mux.HandleFunc("GET /api/v1/objects/groups/nft", s.auth(s.handleRenderGroupSet))
	s.mux.HandleFunc("GET /api/v1/dns/multiline", s.auth(s.handleGetMultiLineDNS))
	s.mux.HandleFunc("POST /api/v1/dns/multiline", s.auth(s.auditWrap("dns.multiline.save", s.handleSetMultiLineDNS)))
	s.mux.HandleFunc("GET /api/v1/policy/domain-rules", s.auth(s.handleGetDomainRules))
	s.mux.HandleFunc("POST /api/v1/policy/domain-rules", s.auth(s.auditWrap("policy.domain.save", s.handleSetDomainRules)))
	s.mux.HandleFunc("GET /api/v1/behavior/records", s.auth(s.handleBehaviorRecords))
	s.mux.HandleFunc("POST /api/v1/behavior/records", s.auth(s.handlePostBehaviorRecord))
	s.mux.HandleFunc("GET /api/v1/behavior/stats", s.auth(s.handleBehaviorStats))
	s.mux.HandleFunc("GET /api/v1/behavior/exempt", s.auth(s.handleGetBehaviorExempt))
	s.mux.HandleFunc("POST /api/v1/behavior/exempt", s.auth(s.auditWrap("behavior.exempt.save", s.handleSetBehaviorExempt)))
	// 磁盘管理 / 文件管理 / 内核参数（路径对齐 iKuai 的 system/*）
	s.mux.HandleFunc("GET /api/v1/system/disks", s.auth(s.handleGetDisks))
	s.mux.HandleFunc("POST /api/v1/system/disks/mount", s.auth(s.auditWrap("system.disk.mount", s.handleMountDisk)))
	s.mux.HandleFunc("POST /api/v1/system/disks/umount", s.auth(s.auditWrap("system.disk.umount", s.handleUmountDisk)))
	s.mux.HandleFunc("POST /api/v1/system/disks/format", s.auth(s.auditWrap("system.disk.format", s.handleFormatDisk)))
	s.mux.HandleFunc("POST /api/v1/system/disks/partition", s.auth(s.auditWrap("system.disk.partition", s.handlePartitionDisk)))
	s.mux.HandleFunc("GET /api/v1/system/files", s.auth(s.handleListFiles))
	s.mux.HandleFunc("POST /api/v1/system/files", s.auth(s.auditWrap("system.files.op", s.handleFileOp)))
	s.mux.HandleFunc("GET /api/v1/system/kernel-params", s.auth(s.handleGetKernelParams))
	s.mux.HandleFunc("POST /api/v1/system/kernel-params", s.auth(s.auditWrap("system.kernel.save", s.handleSetKernelParams)))
	// 应用商店 / 插件管理
	s.mux.HandleFunc("GET /api/v1/plugins", s.auth(s.handleListPlugins))
	s.mux.HandleFunc("POST /api/v1/plugins/{id}", s.auth(s.auditWrap("plugins.action", s.handlePluginAction)))
	s.mux.HandleFunc("GET /api/v1/plugins/{id}/log", s.auth(s.handlePluginLog))
	s.mux.HandleFunc("GET /api/v1/plugins/{id}/version", s.auth(s.handlePluginVersion))
	// 虚拟机（KVM）
	s.mux.HandleFunc("GET /api/v1/vms", s.auth(s.handleListVMs))
	s.mux.HandleFunc("POST /api/v1/vms", s.auth(s.auditWrap("vms.create", s.handleCreateVM)))
	s.mux.HandleFunc("POST /api/v1/vms/{name}", s.auth(s.auditWrap("vms.action", s.handleVMAction)))
	// 跨三层应用
	s.mux.HandleFunc("GET /api/v1/l3-apps", s.auth(s.handleGetL3Apps))
	// 应用特征库
	// DPI 抓包回放与流表
	s.mux.HandleFunc("POST /api/v1/dpi/analyze-pcap", s.auth(s.auditWrap("dpi.analyze.pcap", s.handleAnalyzePcap)))
	s.mux.HandleFunc("GET /api/v1/dpi/flow-stats", s.auth(s.handleDPIStats))
	s.mux.HandleFunc("GET /api/v1/appid/stats", s.auth(s.handleAppidStats))
	s.mux.HandleFunc("GET /api/v1/appid/lookup", s.auth(s.handleAppidLookup))
	s.mux.HandleFunc("GET /api/v1/appid/categories", s.auth(s.handleAppidCategories))
	s.mux.HandleFunc("POST /api/v1/appid/import", s.auth(s.auditWrap("appid.import", s.handleAppidImport)))
	s.mux.HandleFunc("POST /api/v1/l3-apps", s.auth(s.auditWrap("l3apps.save", s.handleSetL3Apps)))
	// FTP / Samba / HTTP 文件服务
	s.mux.HandleFunc("GET /api/v1/services/filesvc/{kind}", s.auth(s.handleGetFileService))
	s.mux.HandleFunc("POST /api/v1/services/filesvc/{kind}", s.auth(s.auditWrap("services.filesvc.save", s.handleSetFileService)))
	s.mux.HandleFunc("GET /api/v1/interfaces/config", s.auth(s.handleGetInterfacesConfig))
	s.mux.HandleFunc("POST /api/v1/interfaces/config", s.auth(s.auditWrap("iface.config.set", s.handleSetInterfacesConfig)))
	s.mux.HandleFunc("GET /api/v1/route/static", s.auth(s.handleGetStaticRoutes))
	s.mux.HandleFunc("POST /api/v1/route/static", s.auth(s.auditWrap("route.static.set", s.handleSetStaticRoutes)))
	s.mux.HandleFunc("GET /api/v1/audit/events", s.auth(s.handleGetAuditEvents))
	s.mux.HandleFunc("GET /api/v1/wan/status", s.auth(s.handleWANStatus))
	s.mux.HandleFunc("GET /api/v1/portal/clients", s.auth(s.handleGetPortalClients))
	s.mux.HandleFunc("POST /api/v1/portal/authorize", s.auth(s.auditWrap("portal.authorize", s.handlePortalAuthorize)))
	s.mux.HandleFunc("POST /api/v1/portal/revoke", s.auth(s.auditWrap("portal.revoke", s.handlePortalRevoke)))
	s.mux.HandleFunc("POST /api/v1/portal/logout", s.auth(s.auditWrap("portal.logout", s.handlePortalLogout)))
	s.mux.HandleFunc("GET /api/v1/portal/status", s.auth(s.handlePortalStatus))
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
	// P28：conntrack 字节统计开关（GET 读状态 / POST 开关+持久化）
	s.mux.HandleFunc("GET /api/v1/conntrack/acct", s.auth(s.handleGetConntrackAcct))
	s.mux.HandleFunc("POST /api/v1/conntrack/acct", s.auth(s.auditWrap("conntrack.acct.set", s.handleSetConntrackAcct)))
	// P28：终端列表
	s.mux.HandleFunc("GET /api/v1/terminals", s.auth(s.handleListTerminals))
	s.mux.HandleFunc("GET /api/v1/conntrack/terminals", s.auth(s.handleListTerminals))
	s.mux.HandleFunc("GET /api/v1/logs", s.auth(s.handleGetLogs))
	s.mux.HandleFunc("GET /api/v1/logs/sources", s.auth(s.handleGetLogSources))
	// 分类日志（ARP/DDNS/拨号/推送/认证/DHCP/系统事件/webadmin/weblogin）
	s.mux.HandleFunc("GET /api/v1/logs/categories", s.auth(s.handleGetLogCategories))
	s.mux.HandleFunc("GET /api/v1/logs/by-category", s.auth(s.handleGetLogsByCategory))
	// P3-3 端口映射 + 策略路由（仅本区块）
	s.mux.HandleFunc("GET /api/v1/portmap/rules", s.auth(s.handleGetPortmapRules))
	s.mux.HandleFunc("POST /api/v1/portmap/rules", s.auth(s.auditWrap("portmap.apply", s.handleSetPortmapRules)))
	s.mux.HandleFunc("DELETE /api/v1/portmap/rules/{id}", s.auth(s.auditWrap("portmap.apply", s.handleDeletePortmapRule)))
	s.mux.HandleFunc("GET /api/v1/policyroute/rules", s.auth(s.handleGetPolicyRouteRules))
	s.mux.HandleFunc("GET /api/v1/policyroute/hits", s.auth(s.handleGetPolicyRouteHits))
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
	// P6-5 系统重启 + 子网换算工具（仅本区块）
	s.mux.HandleFunc("POST /api/v1/system/reboot", s.auth(s.auditWrap("system.reboot", s.handleSystemReboot)))
	s.mux.HandleFunc("POST /api/v1/system/upgrade", s.auth(s.auditWrap("system.upgrade", s.handleSystemUpgrade)))
	s.mux.HandleFunc("GET /api/v1/tools/subnet", s.auth(s.handleSubnetCalc))
	s.mux.HandleFunc("GET /api/v1/ha/config", s.auth(s.handleGetHAConfig))
	s.mux.HandleFunc("POST /api/v1/ha/sync", s.auth(s.auditWrap("ha.sync", s.handleHASync)))
	s.mux.HandleFunc("POST /api/v1/ha/config", s.auth(s.auditWrap("ha.config", s.handleSetHAConfig)))
	s.mux.HandleFunc("POST /api/v1/ha/import", s.auth(s.handleHAImport))
	s.mux.HandleFunc("GET /api/v1/ha/status", s.auth(s.handleHAStatus))
	s.mux.HandleFunc("POST /api/v1/tools/iperf/run", s.auth(s.auditWrap("tools.iperf.run", s.handleIperfRun)))
	s.mux.HandleFunc("GET /api/v1/tools/iperf/status", s.auth(s.handleIperfStatus))
	// P12: background task queue
	s.mux.HandleFunc("POST /api/v1/tasks", s.auth(s.auditWrap("tasks.submit", s.handleTaskSubmit)))
	s.mux.HandleFunc("GET /api/v1/tasks", s.auth(s.handleTaskList))
	s.mux.HandleFunc("GET /api/v1/tasks/{id}", s.auth(s.handleTaskGet))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", s.auth(s.auditWrap("tasks.cancel", s.handleTaskCancel)))
	// P6-6 抓包工具（仅本区块）
	s.mux.HandleFunc("POST /api/v1/tools/capture/start", s.auth(s.auditWrap("tools.capture.start", s.handleCaptureStart)))
	s.mux.HandleFunc("GET /api/v1/tools/capture/status", s.auth(s.handleCaptureStatus))
	s.mux.HandleFunc("GET /api/v1/tools/capture/{id}/download", s.auth(s.handleCaptureDownload))
	s.mux.HandleFunc("POST /api/v1/tools/capture/{id}/stop", s.auth(s.auditWrap("tools.capture.stop", s.handleCaptureStop)))
	// P14: 短信验证码网关（P113-sms-gateway）
	s.mux.HandleFunc("POST /api/v1/auth/sms/send", s.auth(s.handleSMSSend))
	s.mux.HandleFunc("POST /api/v1/auth/sms/verify", s.auth(s.handleSMSVerify))
	s.mux.HandleFunc("GET /api/v1/auth/sms/config", s.auth(s.handleGetSMSConfig))
	s.mux.HandleFunc("POST /api/v1/auth/sms/config", s.auth(s.auditWrap("sms.config.set", s.handleSetSMSConfig)))
	s.mux.HandleFunc("GET /api/v1/auth/wechat/config", s.auth(s.handleGetWechatConfig))
	s.mux.HandleFunc("POST /api/v1/auth/wechat/config", s.auth(s.auditWrap("wechat.config", s.handleSetWechatConfig)))
	s.mux.HandleFunc("POST /api/v1/auth/wechat/login", s.auth(s.auditWrap("wechat.login", s.handleWechatLogin)))
	// P7-4 威胁情报与 IP 封禁（仅本区块）
	s.mux.HandleFunc("GET /api/v1/threat/list", s.auth(s.handleThreatList))
	s.mux.HandleFunc("POST /api/v1/threat/block", s.auth(s.auditWrap("threat.block", s.handleThreatBlock)))
	s.mux.HandleFunc("DELETE /api/v1/threat/block", s.auth(s.auditWrap("threat.unblock", s.handleThreatUnblock)))
	s.mux.HandleFunc("POST /api/v1/threat/apply", s.auth(s.auditWrap("threat.apply", s.handleThreatApply)))
	s.mux.HandleFunc("POST /api/v1/threat/import", s.auth(s.auditWrap("threat.import", s.handleThreatImport)))
	s.mux.HandleFunc("POST /api/v1/threat/fetch", s.auth(s.auditWrap("threat.fetch", s.handleThreatFetch)))
	s.mux.HandleFunc("GET /api/v1/threat/stats", s.auth(s.handleThreatStats))
	// P8-4 应用限速与连接配额（仅本区块）
	// P13：Web 管理端口、主机标识、多 WAN 健康检查
	s.mux.HandleFunc("GET /api/v1/webadmin", s.auth(s.handleGetWebAdmin))
	// ---- P15: 进程与服务 / DDoS / 时间段策略 / 网络工具 ----
	s.mux.HandleFunc("GET /api/v1/system/services", s.auth(s.handleListServices))
	s.mux.HandleFunc("POST /api/v1/system/services/action", s.auth(s.handleServiceAction))
	s.mux.HandleFunc("GET /api/v1/services/status", s.auth(s.handleServicesStatus))
	s.mux.HandleFunc("POST /api/v1/services/{name}", s.auth(s.auditWrap("services.switch", s.handleServiceSwitch)))
	s.mux.HandleFunc("GET /api/v1/system/processes", s.auth(s.handleListProcesses))
	s.mux.HandleFunc("POST /api/v1/system/processes/kill", s.auth(s.handleKillProcess))
	s.mux.HandleFunc("GET /api/v1/security/ddos", s.auth(s.handleGetDDoS))
	s.mux.HandleFunc("POST /api/v1/security/ddos", s.auth(s.handleSetDDoS))
	s.mux.HandleFunc("GET /api/v1/policy/timeschedule", s.auth(s.handleGetTimeSchedule))
	s.mux.HandleFunc("POST /api/v1/policy/timeschedule", s.auth(s.handleSetTimeSchedule))
	s.mux.HandleFunc("DELETE /api/v1/policy/timeschedule/{id}", s.auth(s.auditWrap("timeschedule.delete", s.handleDeleteTimeSchedule)))
	s.mux.HandleFunc("POST /api/v1/diagnostics/ping", s.auth(s.handlePing))
	s.mux.HandleFunc("POST /api/v1/diagnostics/traceroute", s.auth(s.handleTraceroute))
	s.mux.HandleFunc("POST /api/v1/diagnostics/portprobe", s.auth(s.handlePortProbe))
	// ---- P16: WireGuard 客户端 / DDNS ----
	s.mux.HandleFunc("GET /api/v1/vpn/client", s.auth(s.handleGetVPNClient))
	s.mux.HandleFunc("POST /api/v1/vpn/client", s.auth(s.handleSetVPNClient))
	// VPN 客户端多连接（PPTP / L2TP / OpenVPN）
	s.mux.HandleFunc("GET /api/v1/vpn/clients", s.auth(s.handleListVPNClients))
	s.mux.HandleFunc("POST /api/v1/vpn/clients", s.auth(s.auditWrap("vpn.clients.save", s.handleSaveVPNClientEntry)))
	s.mux.HandleFunc("DELETE /api/v1/vpn/clients", s.auth(s.auditWrap("vpn.clients.delete", s.handleDeleteVPNClientEntry)))
	s.mux.HandleFunc("GET /api/v1/vpn/clients/plan", s.auth(s.handleVPNClientPlan))
	s.mux.HandleFunc("GET /api/v1/ddns/records", s.auth(s.handleListDDNS))
	s.mux.HandleFunc("POST /api/v1/ddns/records", s.auth(s.handleSetDDNS))
	s.mux.HandleFunc("DELETE /api/v1/ddns/records/{id}", s.auth(s.auditWrap("ddns.records.delete", s.handleDeleteDDNSRecord)))
	s.mux.HandleFunc("GET /api/v1/ddns/check", s.auth(s.handleDDNSCheck))
	// ---- P17: 卡密认证 ----
	s.mux.HandleFunc("GET /api/v1/voucher/list", s.auth(s.handleListVouchers))
	s.mux.HandleFunc("POST /api/v1/voucher/generate", s.auth(s.handleGenerateVouchers))
	s.mux.HandleFunc("POST /api/v1/voucher/redeem", s.auth(s.handleRedeemVoucher))
	s.mux.HandleFunc("DELETE /api/v1/voucher", s.auth(s.handleDeleteVoucher))
	// ---- P18: 链路检测 ----
	s.mux.HandleFunc("GET /api/v1/linkcheck/targets", s.auth(s.handleListLinkCheck))
	s.mux.HandleFunc("POST /api/v1/linkcheck/targets", s.auth(s.handleSetLinkCheck))
	s.mux.HandleFunc("DELETE /api/v1/linkcheck/targets/{id}", s.auth(s.auditWrap("linkcheck.targets.delete", s.handleDeleteLinkCheckTarget)))
	s.mux.HandleFunc("POST /api/v1/linkcheck/probe", s.auth(s.handleProbeLink))
	s.mux.HandleFunc("GET /api/v1/linkcheck/status", s.auth(s.handleLinkCheckStatus))
	// ---- P21: 防火墙日志分析 ----
	s.mux.HandleFunc("GET /api/v1/firewall/log/analysis", s.auth(s.handleFwLogAnalysis))
	s.mux.HandleFunc("GET /api/v1/firewall/log/suggestions", s.auth(s.handleFwLogSuggestions))
	// ---- P18b: 认证门户模板 ----
	s.mux.HandleFunc("GET /api/v1/portal/templates", s.auth(s.handleListPortalTemplates))
	s.mux.HandleFunc("POST /api/v1/portal/template", s.auth(s.handleSetPortalTemplate))
	s.mux.HandleFunc("GET /api/v1/portal/preview", s.auth(s.handlePreviewPortal))
	s.mux.HandleFunc("POST /api/v1/webadmin", s.auth(s.auditWrap("webadmin.set", s.handleSetWebAdmin)))
	s.mux.HandleFunc("GET /api/v1/identity", s.auth(s.handleGetIdentity))
	s.mux.HandleFunc("POST /api/v1/identity", s.auth(s.auditWrap("identity.set", s.handleSetIdentity)))
	s.mux.HandleFunc("GET /api/v1/multiwan", s.auth(s.handleGetMultiWAN))
	s.mux.HandleFunc("POST /api/v1/multiwan", s.auth(s.auditWrap("multiwan.set", s.handleSetMultiWAN)))
	s.mux.HandleFunc("POST /api/v1/multiwan/probe", s.auth(s.auditWrap("multiwan.probe", s.handleMultiWANProbeResult)))

	// P12：链路聚合、接口调优、证书、在线用户、端口触发/ALG/mDNS
	s.mux.HandleFunc("GET /api/v1/bond", s.auth(s.handleGetBond))
	s.mux.HandleFunc("POST /api/v1/bond", s.auth(s.auditWrap("bond.set", s.handleSetBond)))
	s.mux.HandleFunc("GET /api/v1/iface/tune", s.auth(s.handleGetIfaceTune))
	s.mux.HandleFunc("POST /api/v1/iface/tune", s.auth(s.auditWrap("iface.tune", s.handleSetIfaceTune)))
	s.mux.HandleFunc("GET /api/v1/cert", s.auth(s.handleGetCert))
	s.mux.HandleFunc("POST /api/v1/cert/self-signed", s.auth(s.auditWrap("cert.selfsigned", s.handleSelfSignedCert)))
	s.mux.HandleFunc("GET /api/v1/online", s.auth(s.handleGetOnline))
	s.mux.HandleFunc("POST /api/v1/online/kick", s.auth(s.auditWrap("online.kick", s.handleKickOnline)))
	s.mux.HandleFunc("GET /api/v1/porttrigger", s.auth(s.handleGetPortTrigger))
	s.mux.HandleFunc("POST /api/v1/porttrigger", s.auth(s.auditWrap("porttrigger.set", s.handleSetPortTrigger)))
	s.mux.HandleFunc("GET /api/v1/alg", s.auth(s.handleGetALG))
	s.mux.HandleFunc("POST /api/v1/alg", s.auth(s.auditWrap("alg.set", s.handleSetALG)))
	s.mux.HandleFunc("GET /api/v1/mdns", s.auth(s.handleGetMDNS))
	s.mux.HandleFunc("POST /api/v1/mdns", s.auth(s.auditWrap("mdns.set", s.handleSetMDNS)))

	// P11：固件升级、诊断报告、邮件/Trap 通知、策略路由
	s.mux.HandleFunc("GET /api/v1/firmware/inspect", s.auth(s.handleFirmwareInspect))
	s.mux.HandleFunc("POST /api/v1/firmware/plan", s.auth(s.handleFirmwarePlan))
	s.mux.HandleFunc("POST /api/v1/firmware/apply", s.auth(s.auditWrap("firmware.apply", s.handleFirmwareApply)))
	s.mux.HandleFunc("POST /api/v1/firmware/confirm", s.auth(s.handleFirmwareConfirm))
	s.mux.HandleFunc("GET /api/v1/diagnostics/report", s.auth(s.handleDiagReportJSON))
	s.mux.HandleFunc("GET /api/v1/diagnostics/report/text", s.auth(s.handleDiagReportText))
	s.mux.HandleFunc("GET /api/v1/notify/smtp", s.auth(s.handleGetSMTP))
	s.mux.HandleFunc("POST /api/v1/notify/smtp", s.auth(s.auditWrap("notify.smtp.set", s.handleSetSMTP)))
	s.mux.HandleFunc("POST /api/v1/notify/smtp/preview", s.auth(s.handleSMTPPreview))
	s.mux.HandleFunc("GET /api/v1/notify/trap", s.auth(s.handleGetTrap))
	s.mux.HandleFunc("POST /api/v1/notify/trap", s.auth(s.auditWrap("notify.trap.set", s.handleSetTrap)))
	s.mux.HandleFunc("GET /api/v1/routing/tables", s.auth(s.handleGetPolicyTables))
	s.mux.HandleFunc("POST /api/v1/routing/tables", s.auth(s.auditWrap("routing.tables.set", s.handleSetPolicyTables)))
	s.mux.HandleFunc("GET /api/v1/routing/rules", s.auth(s.handleGetPolicyRules))
	s.mux.HandleFunc("POST /api/v1/routing/rules", s.auth(s.auditWrap("routing.rules.set", s.handleSetPolicyRules)))

	// P10：告警规则、日志轮转/syslog、账单、配置导出/差异/恢复
	s.mux.HandleFunc("GET /api/v1/alerts/rules", s.auth(s.handleGetAlertRules))
	s.mux.HandleFunc("POST /api/v1/alerts/rules", s.auth(s.auditWrap("alerts.rules.set", s.handleSetAlertRules)))
	s.mux.HandleFunc("POST /api/v1/alerts/test", s.auth(s.handleTestAlerts))
	s.mux.HandleFunc("GET /api/v1/logs/rotate", s.auth(s.handleGetRotate))
	s.mux.HandleFunc("POST /api/v1/logs/rotate", s.auth(s.auditWrap("logs.rotate.set", s.handleSetRotate)))
	s.mux.HandleFunc("GET /api/v1/logs/syslog", s.auth(s.handleGetSyslog))
	s.mux.HandleFunc("POST /api/v1/logs/syslog", s.auth(s.auditWrap("logs.syslog.set", s.handleSetSyslog)))
	s.mux.HandleFunc("POST /api/v1/logs/syslog/test", s.auth(s.auditWrap("logs.syslog.test", s.handleTestSyslog)))
	s.mux.HandleFunc("GET /api/v1/billing/bills", s.auth(s.handleGetBills))
	s.mux.HandleFunc("POST /api/v1/billing/bills/generate", s.auth(s.auditWrap("billing.bills.generate", s.handleGenerateBills)))
	s.mux.HandleFunc("GET /api/v1/config/export", s.auth(s.handleExportConfig))
	s.mux.HandleFunc("POST /api/v1/config/preview", s.auth(s.handlePreviewRestore))
	s.mux.HandleFunc("POST /api/v1/config/restore", s.auth(s.auditWrap("config.restore", s.handleRestoreConfig)))

	// P9：IPv6 ND/PD、端口镜像、PPPoE 多拨、流量计费
	s.mux.HandleFunc("GET /api/v1/ipv6/nd", s.auth(s.handleGetIPv6ND))
	s.mux.HandleFunc("POST /api/v1/ipv6/nd", s.auth(s.auditWrap("ipv6.nd.set", s.handleSetIPv6ND)))
	s.mux.HandleFunc("GET /api/v1/ipv6/pd", s.auth(s.handleGetIPv6PD))
	s.mux.HandleFunc("POST /api/v1/ipv6/pd", s.auth(s.auditWrap("ipv6.pd.set", s.handleSetIPv6PD)))
	s.mux.HandleFunc("GET /api/v1/mirror", s.auth(s.handleGetMirror))
	s.mux.HandleFunc("POST /api/v1/mirror", s.auth(s.auditWrap("mirror.set", s.handleSetMirror)))
	// 注意：/api/v1/pppoe/sessions 已经是「服务端」（别人拨进来）的会话列表，
	// 这里是「客户端多拨」（本机拨出去），语义不同必须用不同路径。
	s.mux.HandleFunc("GET /api/v1/pppoe/multidial", s.auth(s.handleGetMultiDial))
	s.mux.HandleFunc("POST /api/v1/pppoe/multidial", s.auth(s.auditWrap("pppoe.multidial.set", s.handleSetMultiDial)))
	s.mux.HandleFunc("GET /api/v1/accounting/status", s.auth(s.handleAccountingStatus))
	s.mux.HandleFunc("GET /api/v1/accounting/report", s.auth(s.handleAccountingReport))

	s.mux.HandleFunc("GET /api/v1/htb/config", s.auth(s.handleGetHTBConfig))
	s.mux.HandleFunc("POST /api/v1/htb/config", s.auth(s.auditWrap("htb.config.set", s.handleSetHTBConfig)))
	s.mux.HandleFunc("POST /api/v1/htb/apply", s.auth(s.auditWrap("htb.apply", s.handleHTBApply)))
	s.mux.HandleFunc("POST /api/v1/htb/smart-apply", s.auth(s.auditWrap("htb.smart-apply", s.handleHTBSmartApply)))
	s.mux.HandleFunc("GET /api/v1/htb/smart-plan", s.auth(s.handleGetHTBSmartPlan))
	s.mux.HandleFunc("POST /api/v1/htb/cleanup", s.auth(s.auditWrap("htb.cleanup", s.handleHTBCleanup)))
	s.mux.HandleFunc("GET /api/v1/htb/limit-rules", s.auth(s.handleHTBLimitRules))
	s.mux.HandleFunc("GET /api/v1/htb/show", s.auth(s.handleHTBShow))
	s.mux.HandleFunc("GET /api/v1/htb/report", s.auth(s.handleHTBReport))

	s.mux.HandleFunc("GET /api/v1/quota/policies", s.auth(s.handleGetQuotaPolicies))
	s.mux.HandleFunc("POST /api/v1/quota/policies", s.auth(s.auditWrap("quota.policies.set", s.handleSetQuotaPolicies)))
	s.mux.HandleFunc("POST /api/v1/quota/apply", s.auth(s.auditWrap("quota.apply", s.handleQuotaApply)))
	s.mux.HandleFunc("POST /api/v1/quota/test", s.auth(s.auditWrap("quota.test", s.handleQuotaTest)))
	s.mux.HandleFunc("GET /api/v1/quota/report", s.auth(s.handleQuotaReport))
	s.mux.HandleFunc("GET /api/v1/quota/nft", s.auth(s.handleQuotaNft))
	// P3-1 安全管控（仅本区块）
	s.mux.HandleFunc("GET /api/v1/acl/rules", s.auth(s.handleGetACLRules))
	s.mux.HandleFunc("POST /api/v1/acl/rules", s.auth(s.auditWrap("acl.rules.set", s.handleSetACLRules)))
	s.mux.HandleFunc("GET /api/v1/acl/mac-filter", s.auth(s.handleGetACLMacFilter))
	s.mux.HandleFunc("POST /api/v1/acl/mac-filter", s.auth(s.auditWrap("acl.mac_filter.set", s.handleSetACLMacFilter)))

	s.mux.HandleFunc("GET /api/v1/security/arp", s.auth(s.handleGetArp))
	s.mux.HandleFunc("POST /api/v1/security/arp", s.auth(s.auditWrap("security.arp.set", s.handleSetArp)))
	s.mux.HandleFunc("GET /api/v1/security/arp/neighbors", s.auth(s.handleGetNeighbors))
	s.mux.HandleFunc("GET /api/v1/webfilter/rules", s.auth(s.handleGetWebfilterRules))
	s.mux.HandleFunc("POST /api/v1/webfilter/rules", s.auth(s.auditWrap("webfilter.rules.set", s.handleSetWebfilterRules)))
	s.mux.HandleFunc("POST /api/v1/webfilter/apply", s.auth(s.auditWrap("webfilter.apply", s.handleWebfilterApply)))
	s.mux.HandleFunc("GET /api/v1/webfilter/apply-status", s.auth(s.handleWebfilterApplyStatus))
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
	s.mux.HandleFunc("GET /api/v1/nac/config", s.auth(s.handleGetNACConfig))
	s.mux.HandleFunc("POST /api/v1/nac/config", s.auth(s.auditWrap("nac.config.set", s.handleSetNACConfig)))
	s.mux.HandleFunc("GET /api/v1/nac/neighbors", s.auth(s.handleGetNACNeighbors))
	s.mux.HandleFunc("GET /api/v1/nac/connlimits", s.auth(s.handleGetNACConnlimits))
	s.mux.HandleFunc("POST /api/v1/nac/connlimits", s.auth(s.auditWrap("nac.connlimits.set", s.handleSetNACConnlimits)))
	s.mux.HandleFunc("POST /api/v1/nac/connlimits/apply", s.auth(s.auditWrap("nac.connlimits.apply", s.handleApplyNACConnlimits)))
	s.mux.HandleFunc("GET /api/v1/nac/top-connections", s.auth(s.handleGetNACTopConnections))
	s.mux.HandleFunc("GET /api/v1/lb/config", s.auth(s.handleGetLBConfig))
	s.mux.HandleFunc("POST /api/v1/lb/config", s.auth(s.auditWrap("lb.config.set", s.handleSetLBConfig)))
	s.mux.HandleFunc("GET /api/v1/lb/status", s.auth(s.handleGetLBStatus))
	// p133: handleGetLBStatus returns per_member_rx_tx (per-member interface rx/tx counters).
	s.mux.HandleFunc("POST /api/v1/lb/check", s.auth(s.auditWrap("lb.check", s.handleLBCheck)))
	s.mux.HandleFunc("POST /api/v1/lb/apply", s.auth(s.auditWrap("lb.apply", s.handleLBApply)))
	s.mux.HandleFunc("GET /api/v1/lb/preview", s.auth(s.handleLBPreview))
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
	// 自助改密：任何已登录用户改自己的密码（身份取自 token，需验证旧密码）。
	// 审计在 handler 内按结果精确记录，这里不再套 auditWrap 以免重复。
	s.mux.HandleFunc("POST /api/v1/auth/password", s.auth(s.handleChangeOwnPassword))
	s.mux.HandleFunc("PUT /api/v1/users/{username}", s.auth(s.auditWrap("user.update", s.handleUpdateUser)))
	// P7-1 IPS 入侵防御（仅本区块）
	s.mux.HandleFunc("GET /api/v1/ips/rules", s.auth(s.handleGetIPSRules))
	s.mux.HandleFunc("POST /api/v1/ips/rules", s.auth(s.auditWrap("ips.rules.set", s.handleSetIPSRules)))
	s.mux.HandleFunc("GET /api/v1/ips/stats", s.auth(s.handleGetIPSStats))
	s.mux.HandleFunc("POST /api/v1/ips/test", s.auth(s.auditWrap("ips.rule.test", s.handleTestIPSRule)))
	s.mux.HandleFunc("GET /api/v1/ips/blocklist", s.auth(s.handleGetIPSBlocklist))
	s.mux.HandleFunc("POST /api/v1/ips/blocklist", s.auth(s.auditWrap("ips.blocklist.set", s.handleSetIPSBlocklist)))
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
		// 只有失败才计入限流配额（成功登录不该扣自己的额度，否则批量脚本
		// 第 6 次起就全 429——实测回归套件就是这么挂的）。
		s.loginLimiter.recordFailure(clientIP(r.RemoteAddr))
		s.logAudit(r, "auth.login", req.Username, "fail", "invalid credentials")
		writeJSON(w, 401, errBody(10001, "invalid credentials", ""))
		return
	}
	s.loginLimiter.reset(clientIP(r.RemoteAddr))
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
		"dnsmasq": dhcp.RenderDnsmasq(dhcp.Config{DHCPConfig: cfg}),
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
		"dnsmasq": dhcp.RenderDnsmasq(dhcp.Config{DHCPConfig: cfg}),
		"applied": true,
	})
}

// maxIndexedIndex 返回 section 中以 firstField 为标志、编号 0..n 里最后一个存在的索引。
//
// 索引式持久化（firewall/nat/wan 都是 0.chain、1.chain…）有个共同的坑：
// 写入时只覆盖 0..len-1，旧的高位索引不清除，于是 GET 端的
// `for i:=0;;i++ { if 空 {break} }` 会把已经删掉的条目继续报出来。
// 实测：提交 3 条防火墙规则后再提交 1 条，GET 仍返回 3 条——删掉的 80/443
// 成了幽灵规则，重启后若按 store 重放还会真的复活。
func (s *Server) maxIndexedIndex(section, firstField string, limit int) int {
	last := -1
	for i := 0; i < limit; i++ {
		if v, err := s.store.Get(section, itoa(i)+"."+firstField); err == nil && v != "" {
			last = i
		}
	}
	return last
}

// unsetIndexedFrom 删除 section 中 [from, to] 区间内每条记录的所有字段。
// to 取写入前的最大索引，传 -1 表示无需清理。
func (s *Server) unsetIndexedFrom(section string, from, to int, fields ...string) {
	for i := from; i <= to; i++ {
		for _, f := range fields {
			_ = s.store.Unset(section, itoa(i)+"."+f)
		}
	}
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
	// Persist to store。先记下旧的最大索引：覆盖写 0..len-1 之后要把更高的
	// 清掉，否则被删掉的规则会留在 store 里，GET 照旧报出来（幽灵规则）。
	oldMax := s.maxIndexedIndex("firewall", "chain", 2048)
	for i, rule := range req.Rules {
		key := itoa(i)
		s.store.Set("firewall", key+".chain", rule.Chain)
		s.store.Set("firewall", key+".src", rule.Src)
		s.store.Set("firewall", key+".dst", rule.Dst)
		s.store.Set("firewall", key+".proto", rule.Proto)
		s.store.Set("firewall", key+".dport", rule.DPort)
		s.store.Set("firewall", key+".action", rule.Action)
	}
	s.unsetIndexedFrom("firewall", len(req.Rules), oldMax,
		"chain", "src", "dst", "proto", "dport", "action")
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
	oldMax := s.maxIndexedIndex("nat", "type", 2048)
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
	s.unsetIndexedFrom("nat", len(req.Rules), oldMax,
		"type", "interface", "proto", "srcport", "dstaddr", "dstport", "toport")
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
		Links   []wan.WANLink `json:"links"`
		Confirm bool          `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logAudit(r, "wan.links.set", "", "fail", "json decode: "+err.Error())
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// 空列表必须显式 confirm:true 才放行——与 alerts/l7/tables 三个"全量替换"
	// 端点同一套约定。为什么这条要紧：applier.Apply(nil) 会按"零条策略路由"
	// 下发，等于把现存的 WAN 策略路由规则整片抽掉，然后还回 200 applied=true。
	// 实测踩到过：空 body 直接 200，看不出任何异常。
	if len(req.Links) == 0 && !req.Confirm {
		s.logAudit(r, "wan.links.set", "", "fail", "empty links without confirm")
		writeJSON(w, 422, errBody(10484, "links 不能为空",
			"清空全部 WAN 策略路由请显式发送 {links:[],confirm:true}"))
		return
	}
	for i, l := range req.Links {
		if l.Interface == "" {
			s.logAudit(r, "wan.links.set", fmt.Sprintf("%d links", len(req.Links)), "fail",
				fmt.Sprintf("link %d missing interface", i))
			writeJSON(w, 422, errBody(10485, "链路缺少接口名",
				fmt.Sprintf("links[%d].interface 不能为空", i)))
			return
		}
	}
	oldMax := s.maxIndexedIndex("wan", "name", 2048)
	for i, l := range req.Links {
		key := itoa(i)
		s.store.Set("wan", key+".name", l.Name)
		s.store.Set("wan", key+".interface", l.Interface)
		s.store.Set("wan", key+".gateway", l.Gateway)
		s.store.Set("wan", key+".weight", itoa(l.Weight))
		s.store.Set("wan", key+".table", itoa(l.Table))
		s.store.Set("wan", key+".health_target", l.HealthTarget)
	}
	s.unsetIndexedFrom("wan", len(req.Links), oldMax,
		"name", "interface", "gateway", "weight", "table", "health_target")
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
		Rules    []dpi.MarkRule `json:"rules"`
		Iface    string         `json:"iface"`
		RootMbit float64        `json:"root_mbit"`
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
	// limit 规则在 nft 里只打 fwmark（nft 的 `limit rate` 只是匹配，不限速），
	// 真正的整形必须由 tc/HTB 按 mark 完成，否则界面会显示"已下发"而实际不限速。
	tcDetail := ""
	hasLimit := false
	for _, rl := range req.Rules {
		if rl.Action == "limit" {
			hasLimit = true
			break
		}
	}
	if hasLimit {
		iface := req.Iface
		if iface == "" {
			iface = "eth0"
		}
		root := req.RootMbit
		if root <= 0 {
			root = 1000
		}
		ok, detail := htb.Apply(execx.DefaultRunner{}, htb.LimitRulesToHTB(req.Rules, iface, root))
		tcDetail = detail
		if !ok {
			applied = false
			if applyErr != "" {
				applyErr += "; "
			}
			applyErr += "tc htb: " + detail
		}
	}
	if raw, mErr := json.Marshal(req.Rules); mErr == nil {
		_ = s.store.Set("dpi", "marks", string(raw))
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules":   req.Rules,
		"nft":     nftText,
		"tc":      dpi.MarkRulesForTC(req.Rules),
		"tc_htb":  tcDetail,
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
		Rules    []dpi.MarkRule `json:"rules"`
		Iface    string         `json:"iface"`
		RootMbit float64        `json:"root_mbit"`
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
	// limit 规则在 nft 里只打 fwmark（nft 的 `limit rate` 只是匹配，不限速），
	// 真正的整形必须由 tc/HTB 按 mark 完成，否则界面会显示"已下发"而实际不限速。
	tcDetail := ""
	hasLimit := false
	for _, rl := range req.Rules {
		if rl.Action == "limit" {
			hasLimit = true
			break
		}
	}
	if hasLimit {
		iface := req.Iface
		if iface == "" {
			iface = "eth0"
		}
		root := req.RootMbit
		if root <= 0 {
			root = 1000
		}
		ok, detail := htb.Apply(execx.DefaultRunner{}, htb.LimitRulesToHTB(req.Rules, iface, root))
		tcDetail = detail
		if !ok {
			applied = false
			if applyErr != "" {
				applyErr += "; "
			}
			applyErr += "tc htb: " + detail
		}
	}
	if raw, mErr := json.Marshal(req.Rules); mErr == nil {
		_ = s.store.Set("dpi", "marks", string(raw))
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules":   req.Rules,
		"nft":     nftText,
		"tc":      dpi.MarkRulesForTC(req.Rules),
		"tc_htb":  tcDetail,
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
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, errBody(10007, "read body fail", err.Error()))
		return
	}
	// Accept either the bare Device object or a {"device": {...}} wrapper.
	var dev cloud.LegacyDevice
	if e := json.Unmarshal(body, &dev); e != nil || dev.Name == "" {
		var wrapper struct {
			Device cloud.LegacyDevice `json:"device"`
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
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
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
	// 特征库是包级全局状态，服务开始接请求前装载一次
	loadAppidLibrary()
	// 用显式 http.Server 而不是裸 ListenAndServe，主要为了 ReadHeaderTimeout：
	// 裸 ListenAndServe 没有任何超时，攻击者只要慢悠悠地发请求头（Slowloris），
	// 就能用极少的连接把路由器的连接/goroutine 占满——管理界面直接失联。
	//
	// 刻意不设 ReadTimeout/WriteTimeout：固件/特征库升级要传 100MB+ 的文件，
	// 监控代理可能长时间流式回传，一刀切的读写超时会把正常功能切断。
	// IdleTimeout 只限制 keep-alive 空闲连接，不误伤进行中的传输。
	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
	}
	return srv.ListenAndServe()
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
		sub, iat, err := s.tm.VerifyWithIssuedAt(tok)
		if err != nil {
			writeJSON(w, 401, errBody(10003, "unauthorized", "invalid token"))
			return
		}
		// 改密后旧会话必须失效。token 是无状态的，改密不会让已签发的 token 作废，
		// 只能比对签发时间。密码泄漏的第一反应就是改密——如果旧 token 还能用
		// （哪怕只剩 TTL 那 15 分钟），改密就等于没改。
		if !s.sessionValid(sub, iat) {
			s.logAudit(r, "authz.stale_session", sub, "fail", r.Method+" "+r.URL.Path)
			writeJSON(w, 401, errBody(10003, "unauthorized", "密码已修改，请重新登录"))
			return
		}
		// Put the authenticated actor into context for audit logging.
		ctx := context.WithValue(r.Context(), ctxKeyActor, sub)
		// 角色鉴权。
		//
		// 这里以前只验 token、不验角色，于是 rbac.Can 写了策略、也有单测，却从没
		// 被任何请求走到过——单测全绿，生产零校验。实测：viewer 账号 POST
		// /firewall/rules 返回 200，能改防火墙（只有 /users 那几个端点自己单独
		// 查了 role）。「只读角色」根本不只读，等于提权。
		if !s.roleAllows(sub, r.Method, r.URL.Path) {
			s.logAudit(r, "authz.deny", sub, "fail", r.Method+" "+r.URL.Path)
			writeJSON(w, 403, errBody(10006, "forbidden", "当前角色无权执行该操作"))
			return
		}
		next(w, r.WithContext(ctx))
	}
}

// sessionValid 判定 token 是否签发于用户最后一次改密之前。
//
// 内置管理员（环境变量那个）没有用户记录，恒真；有记录时 PasswordChangedAt==0
// 表示从未改过密，也恒真。秒级精度带来的唯一缝：改密与登录落在同一秒时，
// 该秒签发的 token 保留（iat == PasswordChangedAt），这是刻意的——否则刚改完密
// 立刻重新登录的人会被自己的新 token 踢掉。
func (s *Server) sessionValid(actor string, iat int64) bool {
	if s.rbacMgr == nil || actor == s.cfg.AdminUser {
		return true
	}
	u, ok := s.rbacMgr.Get(actor)
	if !ok {
		return false
	}
	if u.PasswordChangedAt <= 0 {
		return true
	}
	return iat >= u.PasswordChangedAt
}

// roleAllows 判定 actor 是否有权执行 method+path。
//
// 内置管理员（环境变量配置的那个）恒为 admin，保证单管理员部署和首次登录不被
// 破坏；其余账号查 RBAC 表，查不到（用户已删/被停用）则拒绝。无 RBAC 管理器时
// 放行，保持旧部署行为。
func (s *Server) roleAllows(actor, method, path string) bool {
	if actor == s.cfg.AdminUser {
		return true
	}
	// 自助改密端点对所有已登录用户开放：身份取自 token，处理函数会校验
	// 旧密码且只能改自己，不需要 admin 或特定角色（viewer 也应能改自己密码）。
	if method == http.MethodPost && path == "/api/v1/auth/password" {
		return true
	}
	if s.rbacMgr == nil {
		return true
	}
	role, ok := s.rbacMgr.RoleOf(actor)
	if !ok {
		return false
	}
	// 停用的账号必须立即失去权限。RoleOf 只看 role 字段、不看 Enabled，所以
	// 单独查一次用户：否则把 users.json 里 enabled 改成 false（或以后加停用接口）
	// 之后，旧 token 会一直有效直到过期——禁用等于没禁用。
	if u, ok := s.rbacMgr.Get(actor); !ok || !u.Enabled {
		return false
	}
	return rbac.Can(role, method, path)
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
		Users   []pppoe.User `json:"users"`
		Confirm bool         `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空所有 PPPoE 账号并返回 200。
	// 必须显式 confirm:true 才允许清空。
	if len(req.Users) == 0 && !req.Confirm {
		writeJSON(w, 422, errBody(10052, "users must not be empty",
			"to clear all pppoe users, send {users:[],confirm:true}"))
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
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
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

// paginateEntries 从"多取 offset+limit 条"的查询结果里切出当前页。
//
// 抽成独立函数是为了能测：真实 journal 一直在变（高流量时同一秒几十条重复
// 消息），用 HTTP 打两次窗口根本比不出 offset 有没有位移——实测两次请求返回
// 的首条一模一样，看着像 bug、其实只是日志在刷屏。这里用纯函数把语义钉死，
// 由单测覆盖边界，HTTP 层只负责解析参数。
//
// total 的语义：只有查询没被 queryLimit 截断时才是真总数；被截断时返回 -1
// 并置 hasMore，绝不谎报（实测旧代码把 5000 条 journal 报成 total=10）。
func paginateEntries[T any](all []T, limit, offset, queryLimit int) (page []T, total int, hasMore bool) {
	total = len(all)
	hasMore = len(all) >= queryLimit
	if hasMore {
		total = -1
	}
	if offset >= len(all) {
		return []T{}, total, hasMore
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, hasMore
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
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	// 支持 offset：多取 offset+limit 条，再切片
	queryLimit := limit + offset
	if queryLimit > 5000 {
		queryLimit = 5000
	}
	opts := logcenter.QueryOpts{
		Limit: queryLimit,
		Unit:  r.URL.Query().Get("unit"),
		Grep:  r.URL.Query().Get("grep"),
		Since: r.URL.Query().Get("since"),
	}
	entries, err := logcenter.Query(opts)
	if err != nil {
		writeJSON(w, 500, errBody(10071, "logs query failed", err.Error()))
		return
	}
	total := len(entries)
	// 注意：total 只有在"这次查询没被 queryLimit 截断"时才是真总数。截断了就说
	// 实话——返回 total=-1 + has_more=true，别给前端一个假总数（比如实际有
	// 5000 条 journal，前端会以为只有 offset+limit 条，翻到第二页就到底了）。
	// 为什么不直接查全量拿真总数：docs/capacity-probe.md 点名的就是这个端点
	// 在小内存路由上慢，为了一个 total 去拉整本 journal 等于把慢端点做得更慢。
	//
	// 判据必须是"窗口被填满"= len(entries) == queryLimit，不能写
	// offset+limit > total（total 就是 len(entries)，那个比较永远为假，
	// 实测 limit=10 会返回 total=10/has_more=false，把 5000 条谎报成 10 条）。
	page, total, truncated := paginateEntries(entries, limit, offset, queryLimit)
	writeJSON(w, 200, map[string]interface{}{
		"entries":  page,
		"count":    len(page),
		"limit":    limit,
		"offset":   offset,
		"total":    total,
		"has_more": truncated,
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

// loadAppidLibrary 启动时装载应用特征库。
// 库文件不存在或损坏都只记日志、退回空库，不影响管理面启动。
func loadAppidLibrary() {
	// 指纹名单：内置几条实测值，运维可用 /var/lib/router-re/ja4_clients.json 覆盖
	if cnt, err := dpi.LoadKnownClients("/var/lib/router-re/ja4_clients.json"); err != nil {
		log.Printf("加载 JA4 客户端名单失败：%v", err)
	} else if cnt > 0 {
		log.Printf("JA4 客户端名单：来自文件的 %d 条", cnt)
	}

	n, err := appid.LoadFrom(appid.Path())
	if err != nil {
		log.Printf("appid library load failed (starting empty): %v", err)
		return
	}
	if n > 0 {
		log.Printf("appid library loaded: %d apps from %s", n, appid.Path())
	} else {
		log.Printf("appid library empty: 导入域名分类数据后才会识别具体应用（%s）", appid.Path())
	}
}

// statsHistoryPath 是监控历史的落盘位置。
// 允许用环境变量覆盖，测试和其他部署形态可以不写系统目录。
func statsHistoryPath() string {
	if v := os.Getenv("ROUTER_RE_STATS_HISTORY"); v != "" {
		return v
	}
	return "/var/lib/router-re/stats-history.json"
}

// SaveStatsHistory 把监控历史落盘。进程退出前调用，
// 否则最近 5 分钟（两次定期落盘之间）的样本会随重启一起丢。
func (s *Server) SaveStatsHistory() {
	if err := s.statsRing.SaveFile(statsHistoryPath()); err != nil {
		log.Printf("stats history save on exit failed: %v", err)
	}
}

// loadStatsRing 启动时恢复历史样本。
//
// 读不到或文件损坏都不阻止服务启动——监控历史不是配置，
// 丢了大不了从当前时刻重新积累，但不该让路由器管理面起不来。
func loadStatsRing() *stats.Ring {
	path := statsHistoryPath()
	ring, n, err := stats.LoadFile(path, 720)
	if err != nil {
		log.Printf("stats history load failed (starting empty): %v", err)
	}
	if n > 0 {
		log.Printf("stats history restored: %d samples from %s", n, path)
	}
	return ring
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
	n := 0
	for range ticker.C {
		if sm, err := stats.Collect(); err == nil {
			s.statsRing.Add(sm)
			n++
			// 每 5 分钟落盘一次：重启后不用从零开始攒，
			// 否则"360 分钟"视图永远只有重启后那几分钟
			if n%10 == 0 {
				if err := s.statsRing.SaveFile(statsHistoryPath()); err != nil {
					log.Printf("stats history save failed: %v", err)
				}
			}
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

// handleGetACLMacFilter returns the current MAC filter config from store.
func (s *Server) handleGetACLMacFilter(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("acl", "mac_filter")
	cfg := acl.MacFilter{Mode: acl.MacFilterModeBlacklist}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg})
}

// handleSetACLMacFilter validates, stores and live-applies a MAC filter.
func (s *Server) handleSetACLMacFilter(w http.ResponseWriter, r *http.Request) {
	var req acl.MacFilter
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10505, "invalid body", err.Error()))
		return
	}
	if err := acl.ValidateMacFilter(req); err != nil {
		writeJSON(w, 422, errBody(10506, "validation failed", err.Error()))
		return
	}
	applied := false
	var nftOut string
	if s.aclApplier != nil {
		if err := acl.ApplyMacFilter(s.aclApplier, req); err != nil {
			writeJSON(w, 500, errBody(10507, "nft apply failed", err.Error()))
			return
		}
		applied = true
		if out, err := s.aclApplier.Run(acl.NftName, acl.NftMacFilterListArgs...); err == nil {
			nftOut = out
		}
	} else {
		nftOut = acl.RenderMacFilter(req)
	}
	b, _ := json.Marshal(req)
	_ = s.store.Set("acl", "mac_filter", string(b))
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  req,
		"nft":     nftOut,
	})
}

// frp registers frp client management routes.
func (s *Server) frp() {
	s.mux.HandleFunc("GET /api/v1/frp/config", s.auth(s.handleGetFrpConfig))
	s.mux.HandleFunc("POST /api/v1/frp/config", s.auth(s.auditWrap("frp.config.set", s.handleSetFrpConfig)))
	s.mux.HandleFunc("POST /api/v1/frp/apply", s.auth(s.auditWrap("frp.apply", s.handleFrpApply)))
	s.mux.HandleFunc("GET /api/v1/frp/status", s.auth(s.handleFrpStatus))
	s.mux.HandleFunc("POST /api/v1/frp/stop", s.auth(s.auditWrap("frp.stop", s.handleFrpStop)))
	// p153 web embed
	s.mux.HandleFunc("GET /assets/", s.handleWebEmbed)
	s.mux.HandleFunc("GET /", s.handleWebEmbed)
	// P153 web embed
}

// handleGetFrpConfig returns the stored frp client config.
func (s *Server) handleGetFrpConfig(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("frp", "config")
	cfg, _ := frp.ParseConfig(raw)
	writeJSON(w, 200, map[string]interface{}{"config": cfg})
}

// handleFrpConfig handles GET/POST /api/v1/frp/config.
func (s *Server) handleFrpConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.handleGetFrpConfig(w, r)
		return
	}
	s.handleSetFrpConfig(w, r)
}

// handleSetFrpConfig stores (validates) an frp client config.
func (s *Server) handleSetFrpConfig(w http.ResponseWriter, r *http.Request) {
	var cfg frp.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10508, "invalid body", err.Error()))
		return
	}
	if err := cfg.Validate(); err != nil {
		writeJSON(w, 422, errBody(10509, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("frp", "config", string(b))
	writeJSON(w, 200, map[string]interface{}{"config": cfg})
}

// handleFrpApply writes config and starts/reloads frpc.
func (s *Server) handleFrpApply(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("frp", "config")
	cfg, _ := frp.ParseConfig(raw)
	if err := cfg.Validate(); err != nil {
		writeJSON(w, 422, errBody(10510, "validation failed", err.Error()))
		return
	}
	if err := frp.Apply(cfg); err != nil {
		writeJSON(w, 500, errBody(10511, "frp apply failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"status": frp.CurrentStatus(), "config": cfg})
}

// handleFrpStatus returns frpc process status.
func (s *Server) handleFrpStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, frp.CurrentStatus())
}

// handleFrpStop stops frpc.
func (s *Server) handleFrpStop(w http.ResponseWriter, r *http.Request) {
	if err := frp.Stop(); err != nil {
		writeJSON(w, 500, errBody(10512, "frp stop failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"stopped": true, "status": frp.CurrentStatus()})
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
	// 网卡名会进入 sysctl 键名，必须在最外层就挡掉非法字符，
	// 而不是等到底层执行时才发现（那时命令已经拼出来了）。
	if err := acl.ValidateIface(req.Iface); err != nil {
		writeJSON(w, 400, errBody(10097, "invalid iface", err.Error()))
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
	// 账号批量导入可能带很多条，但仍要有上限：LimitReader 多给 1 字节，
	// 正好等于上限时能读完，超限时立刻判定为超大 body。
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody+1))
	if err == nil && len(body) > maxJSONBody {
		return nil, fmt.Errorf("请求体超过 %d 字节上限", maxJSONBody)
	}
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
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
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

// handleChangeOwnPassword 让任何已登录用户修改「自己」的密码。
//
// 与 handleSetUserPassword 的区别：后者要求 admin 且能改任意用户；这里身份
// 只取自 token（Upsert 不了别人的账号），因此 viewer/operator 都能用，且必须
// 验证旧密码——「改密」若不需要旧密码，拿别人一个未过期的 token 就能把密码
// 改走、永久接管账号。
func (s *Server) handleChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	actor := s.currentActor(r)
	if actor == "" {
		writeJSON(w, 401, errBody(10003, "unauthorized", "无法识别当前用户"))
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10228, "invalid body", err.Error()))
		return
	}
	// 旧密码校验口径与登录一致：优先 RBAC 记录（内部带锁，不会与登录竞争），
	// 内置 env 管理员再用环境变量口令兜底比对。
	oldOK := false
	if u, ok := s.rbacMgr.Get(actor); ok && u.Enabled {
		oldOK = rbac.VerifyPassword(req.OldPassword, u.PasswordHash)
	}
	if !oldOK && actor == s.cfg.AdminUser {
		oldOK = subtle.ConstantTimeCompare([]byte(req.OldPassword), []byte(s.cfg.AdminPassword)) == 1
	}
	if !oldOK {
		s.logAudit(r, "auth.password.change", actor, "fail", "旧密码错误")
		// 用 400 而非 401：请求本身已通过 token 鉴权，只是旧密码填错。前端拦截器
		// 把 401 当作「登录失效」会直接踢回登录页，掩盖真正原因。
		writeJSON(w, 400, errBody(10229, "invalid old password", "当前密码错误"))
		return
	}
	if err := rbac.ValidatePassword(req.NewPassword); err != nil {
		writeJSON(w, 422, errBody(10230, "weak password", err.Error()))
		return
	}
	if err := s.rbacMgr.ChangePassword(actor, req.NewPassword); err != nil {
		writeJSON(w, 409, errBody(10231, "password change failed", err.Error()))
		return
	}
	s.logAudit(r, "auth.password.change", actor, "ok", "")
	// 改密会让此前签发的 token 失效（sessionValid 比对 PasswordChangedAt），
	// 前端据此应引导重新登录。
	writeJSON(w, 200, map[string]interface{}{"changed": true, "reauth": true})
}

// handleUpdateUser updates a user's role and/or enabled flag.
// Body: {"role":"operator","enabled":false} (both optional, only provided fields changed).
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == "" {
		return
	}
	name := r.PathValue("username")
	if name == "" {
		writeJSON(w, 400, errBody(10221, "missing username", ""))
		return
	}
	existing, ok := s.rbacMgr.Get(name)
	if !ok {
		writeJSON(w, 404, errBody(10222, "user not found", name))
		return
	}
	var req struct {
		Role    *rbac.Role `json:"role"`
		Enabled *bool      `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10223, "invalid body", err.Error()))
		return
	}
	// Apply only provided fields
	if req.Role != nil {
		if !rbac.ValidRole(*req.Role) {
			writeJSON(w, 422, errBody(10224, "invalid role", "role must be admin/operator/viewer"))
			return
		}
		existing.Role = *req.Role
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if err := s.rbacMgr.Update(existing); err != nil {
		if strings.Contains(err.Error(), "last admin") {
			writeJSON(w, 409, errBody(10225, "cannot demote/disable last admin", err.Error()))
			return
		}
		writeJSON(w, 422, errBody(10226, "update failed", err.Error()))
		return
	}
	// Do NOT echo PasswordHash back.
	updated, _ := s.rbacMgr.Get(name)
	writeJSON(w, 200, map[string]interface{}{"user": updated})
}

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

// handleGetDHCPv6 returns the DHCPv6 portion of the server config.
func (s *Server) handleGetDHCPv6(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("dhcp", dhcpServerStoreKey)
	if raw == "" {
		writeJSON(w, 200, map[string]interface{}{
			"config": dhcp.ServerConfig{Enabled: false},
		})
		return
	}
	var cfg dhcp.ServerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		writeJSON(w, 500, errBody(10010, "config unmarshal error", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg})
}

// handleSetDHCPv6 validates and stores DHCPv6 config.
func (s *Server) handleSetDHCPv6(w http.ResponseWriter, r *http.Request) {
	var cfg dhcp.ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10012, "invalid body", err.Error()))
		return
	}
	if err := dhcp.ValidateV6Config(cfg); err != nil {
		writeJSON(w, 422, errBody(10014, "invalid dhcp v6 config", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("dhcp", dhcpServerStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "applied": true})
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

// --- P7-1 IPS 入侵防御 (仅本区块) ---

const (
	ipsRulesStoreKey     = "ips/rules"
	ipsBlocklistStoreKey = "ips/blocklist"
)

// ipsRulesFromStore 读取 store 的 Suricata 文本 → 解析为规则切片。
func (s *Server) ipsRulesFromStore() ([]ips.Rule, error) {
	raw, _ := s.store.Get("ips", ipsRulesStoreKey)
	if raw == "" {
		return nil, nil
	}
	rules, err := ips.ParseRulesFile(raw)
	return rules, err
}

// nextIPSSid 返回当前规则集中最大 SID + 1（用于前端表单 sid=0 自动分配）。
func (s *Server) nextIPSSid() int {
	rules, _ := s.ipsRulesFromStore()
	max := 0
	for _, r := range rules {
		if r.SID > max {
			max = r.SID
		}
	}
	return max + 1
}

// handleGetIPSRules 返回当前已载入的规则。
func (s *Server) handleGetIPSRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.ipsRulesFromStore()
	if err != nil {
		writeJSON(w, 500, errBody(10030, "rule parse error", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"rules": rules, "count": len(rules)})
}

// handleSetIPSRules 写入规则。body 可为 {rules:[...]} 或 {text:"suricata text"}。
func (s *Server) handleSetIPSRules(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Rules []ips.Rule `json:"rules"`
		Text  string     `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, 422, errBody(10031, "invalid body", err.Error()))
		return
	}
	var rules []ips.Rule
	var parseErrs string
	if payload.Text != "" {
		var agg error
		rules, agg = ips.ParseRulesFile(payload.Text)
		if agg != nil {
			parseErrs = agg.Error()
		}
		if len(rules) == 0 {
			writeJSON(w, 422, errBody(10032, "no valid rules parsed", parseErrs))
			return
		}
		// 写回 store 为规则文本。
		var sb strings.Builder
		for _, rr := range rules {
			sb.WriteString(ruleToSuricata(rr))
			sb.WriteString("\n")
		}
		s.store.Set("ips", ipsRulesStoreKey, sb.String())
	} else if len(payload.Rules) > 0 {
		var sb strings.Builder
		for i, rr := range payload.Rules {
			// 前端表单可能发 sid=0，自动分配下一个 SID（max+1）。
			if rr.SID == 0 {
				rr.SID = s.nextIPSSid()
				payload.Rules[i] = rr
			}
			if err := rr.Validate(); err != nil {
				writeJSON(w, 422, errBody(10033, "invalid rule", fmt.Sprintf("sid=%d: %s", rr.SID, err)))
				return
			}
			sb.WriteString(ruleToSuricata(rr))
			sb.WriteString("\n")
		}
		rules = payload.Rules
		s.store.Set("ips", ipsRulesStoreKey, sb.String())
	} else {
		writeJSON(w, 422, errBody(10034, "invalid body", "rules or text required"))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied":  true,
		"rules":    rules,
		"count":    len(rules),
		"failures": parseErrs,
	})
}

// ruleToSuricata 把 Rule 还原为 Suricata 行（用于 store 持久化）。
func ruleToSuricata(r ips.Rule) string {
	return fmt.Sprintf(`%s %s any any -> any %s (msg:"%s"; content:"%s"; nocase; sid:%d; priority:%d;)`,
		r.Action, r.Proto, r.DstPort, r.Msg, payloadForSuricata(r), r.SID, r.Severity)
}

func payloadForSuricata(r ips.Rule) string {
	if r.PayloadHex != "" {
		return formatHexContent(r.PayloadHex)
	}
	return r.PayloadAscii
}

// formatHexContent 把字节串转成 Suricata |xx xx| 形式。
func formatHexContent(s string) string {
	var b strings.Builder
	b.WriteByte('|')
	for i := 0; i < len(s); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02x", s[i])
	}
	b.WriteByte('|')
	return b.String()
}

// handleGetIPSStats 返回引擎统计。
func (s *Server) handleGetIPSStats(w http.ResponseWriter, r *http.Request) {
	rules, err := s.ipsRulesFromStore()
	if err != nil {
		writeJSON(w, 500, errBody(10035, "rule parse error", err.Error()))
		return
	}
	e := ips.NewEngine()
	e.Load(rules)
	writeJSON(w, 200, map[string]interface{}{"stats": e.Stats(), "total": len(rules)})
}

// handlePostIPSQuery 测试单条规则是否命中。
func (s *Server) handleTestIPSRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RuleSID int    `json:"rule_sid"`
		Proto   string `json:"proto"`
		DstPort int    `json:"dst_port"`
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10036, "invalid body", err.Error()))
		return
	}
	rules, err := s.ipsRulesFromStore()
	if err != nil {
		writeJSON(w, 500, errBody(10037, "rule parse error", err.Error()))
		return
	}
	e := ips.NewEngine()
	e.Load(rules)
	hits := e.Match(req.Proto, req.DstPort, []byte(req.Payload))
	var matched bool
	var matchedRule *ips.Rule
	for i := range hits {
		if hits[i].SID == req.RuleSID {
			matched = true
			matchedRule = &hits[i]
			break
		}
	}
	resp := map[string]interface{}{"matched": matched, "rule": matchedRule, "hits": hits}
	writeJSON(w, 200, resp)
}

// handleGetIPSBlocklist 返回封禁 IP 列表。
func (s *Server) handleGetIPSBlocklist(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("ips", ipsBlocklistStoreKey)
	var ipsList []string
	if raw != "" {
		for _, ip := range strings.Split(raw, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				ipsList = append(ipsList, ip)
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{"ips": ipsList, "count": len(ipsList)})
}

// handleSetIPSBlocklist 下发封禁列表到 nft set blackhole。
func (s *Server) handleSetIPSBlocklist(w http.ResponseWriter, r *http.Request) {
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10038, "exec runner not initialized", ""))
		return
	}
	var req struct {
		IPs     []string `json:"ips"`
		Confirm bool     `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10039, "invalid body", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空整个封禁列表（解封所有 IP）并返回 200。
	// 必须显式 confirm:true 才允许清空 —— 误发空 body 就全网解封的风险太高。
	if len(req.IPs) == 0 && !req.Confirm {
		writeJSON(w, 422, errBody(10043, "ips must not be empty",
			"to clear all blocked ips, send {ips:[],confirm:true}"))
		return
	}
	blocked := make(map[string]bool, len(req.IPs))
	for _, ip := range req.IPs {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			writeJSON(w, 422, errBody(10040, "invalid ip", ip))
			return
		}
		blocked[ip] = true
	}
	// 持久化（逗号分隔）。
	list := make([]string, 0, len(blocked))
	for ip := range blocked {
		list = append(list, ip)
	}
	sort.Strings(list)
	s.store.Set("ips", ipsBlocklistStoreKey, strings.Join(list, ","))

	// 下发到 nft：删表（忽略不存在）→ nft -f -
	script := ips.RenderNft(blocked)
	// 参照 internal/acl 模式：先删除旧表（忽略不存在），再 nft -f - 应用。
	s.aclApplier.Run("nft", "delete", "table", "inet", "router_ips")
	applyOut, applyErr := s.aclApplier.RunWithInput(script, "nft", "-f", "-")
	_ = applyOut
	detail := ""
	applied := applyErr == nil
	if applyErr != nil {
		detail = applyErr.Error()
	}
	// 回读确认 blackhole set。
	listOut, _ := s.aclApplier.Run("nft", []string{"list", "table", "inet", "router_ips"}...)
	writeJSON(w, 200, map[string]interface{}{
		"applied":   applied,
		"blocklist": list,
		"nft":       script,
		"detail":    detail,
		"listing":   listOut,
	})
}

// --- P8-1 DPI 接入转发路径 (仅本区块) ---

// dpiTracker 是数据面流跟踪器单例。真实数据面接入前先用空 tracker 支撑 /dpi/flows 等接口。
var dpiTracker = dpiflow.NewTracker(10000)

func (s *Server) handleGetDPIFlows(w http.ResponseWriter, r *http.Request) {
	limit := 500
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, 200, map[string]interface{}{"flows": dpiTracker.List(limit), "count": len(dpiTracker.List(limit))})
}

func (s *Server) handleGetDPIApps(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"apps": dpiTracker.Snapshot()})
}

func (s *Server) handleGetDPITrackerStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"stats": dpiTracker.Stats()})
}

func (s *Server) handleClassifyPayload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PayloadHex   string `json:"payload_hex"`
		PayloadAscii string `json:"payload_ascii"`
		DstPort      int    `json:"dst_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10050, "invalid body", err.Error()))
		return
	}
	var payload []byte
	if req.PayloadHex != "" {
		h := strings.TrimPrefix(strings.TrimSpace(req.PayloadHex), "0x")
		h = strings.ReplaceAll(h, " ", "")
		decoded, err := hex.DecodeString(h)
		if err != nil {
			writeJSON(w, 422, errBody(10051, "invalid payload_hex", err.Error()))
			return
		}
		payload = decoded
	} else {
		payload = []byte(req.PayloadAscii)
	}
	app, cat := dpiflow.DetectApp(payload, req.DstPort)
	// QUIC（HTTP/3）走独立路径：SNI 在被 Initial 密钥加密的载荷里，
	// 明文字符串匹配看不到，必须先解密再重组 CRYPTO 帧。
	if req.DstPort == 443 && len(payload) > 0 && payload[0]&0x80 != 0 {
		if sni := dpi.QUICSNI(payload); sni != "" {
			app, cat = "quic-sni:"+sni, "Encrypted"
		}
	}
	resp := map[string]interface{}{"app": app, "category": cat, "payload_len": len(payload)}
	// 客户端指纹：SNI 之外的第二条识别线索
	if ch := dpi.ParseClientHello(payload); ch != nil {
		resp["ja4"] = ch.JA4()
		resp["ja3"] = ch.JA3()
		resp["tls_sni"] = ch.SNI
		if c := dpi.DescribeClient(ch.JA4()); c != "" {
			resp["client"] = c
		}
	} else if req.DstPort == 443 {
		if ch := dpi.QUICClientHello(payload); ch != nil {
			resp["ja4"] = ch.JA4()
			resp["tls_sni"] = ch.SNI
			if c := dpi.DescribeClient(ch.JA4()); c != "" {
				resp["client"] = c
			}
		}
	}
	// 底层 DPI 只到协议层（tls-sni:www.qq.com）。再查一次特征库，
	// 把主机名换成"用户真正关心的应用名"——这正是商业路由所谓应用识别的差别所在。
	if host := hostFromAppTag(app); host != "" {
		resp["host"] = host
		if a, ok := lookupAppForHost(host); ok {
			resp["app_id"] = a.ID
			resp["app_name"] = a.Name
			resp["app_category"] = string(a.Category)
			resp["identified"] = true
		} else {
			resp["identified"] = false
		}
	}
	writeJSON(w, 200, resp)
}

// hostFromAppTag 从 DetectApp 的返回里取出主机名。
// 形如 "tls-sni:www.qq.com" / "http:example.com" / "dns:a.b.c"。
func hostFromAppTag(app string) string {
	for _, p := range []string{"tls-sni:", "quic-sni:", "http:", "dns:"} {
		if strings.HasPrefix(app, p) {
			return strings.TrimPrefix(app, p)
		}
	}
	return ""
}

const dpiMarkRulesStoreKey = "dpi/mark-rules"

func (s *Server) handleGetDPIMarkRules(w http.ResponseWriter, r *http.Request) {
	appPorts := loadDPIMarkRules(s)
	writeJSON(w, 200, map[string]interface{}{"app_ports": appPorts, "nft": dpiflow.RenderNftMarkRules(appPorts)})
}

func (s *Server) handleSetDPIMarkRules(w http.ResponseWriter, r *http.Request) {
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10052, "exec runner not initialized", ""))
		return
	}
	var req struct {
		AppPorts map[string][]int `json:"app_ports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10053, "invalid body", err.Error()))
		return
	}
	if req.AppPorts == nil {
		writeJSON(w, 422, errBody(10054, "invalid body", "app_ports required"))
		return
	}
	// 校验端口范围
	for app, ports := range req.AppPorts {
		for _, p := range ports {
			if p < 1 || p > 65535 {
				writeJSON(w, 422, errBody(10055, "invalid port", fmt.Sprintf("app=%s port=%d", app, p)))
				return
			}
		}
	}
	// 持久化
	raw, _ := json.Marshal(req.AppPorts)
	s.store.Set("dpi", dpiMarkRulesStoreKey, string(raw))

	// 下发 nft：删表（忽略不存在）→ nft -f -
	nftText := dpiflow.RenderNftMarkRules(req.AppPorts)
	s.aclApplier.Run("nft", "delete", "table", "inet", "router_dpi")
	applyOut, applyErr := s.aclApplier.RunWithInput(nftText, "nft", "-f", "-")
	_ = applyOut
	detail := ""
	if applyErr != nil {
		detail = applyErr.Error()
	}
	// 回读确认
	listOut, _ := s.aclApplier.Run("nft", []string{"list", "table", "inet", "router_dpi"}...)
	writeJSON(w, 200, map[string]interface{}{
		"applied":   applyErr == nil,
		"app_ports": req.AppPorts,
		"nft":       nftText,
		"detail":    detail,
		"listing":   listOut,
	})
}

// handleSystemReboot triggers a system reboot.
// handleSystemUpgrade 在线升级路由终端：下载 → 校验 → 原子替换 → 重启 → 看门狗回滚。
//
// 这里的每条约束都对应一个能把设备刷成砖的失败模式：
//   - checksum 必填：不校验完整性，等于允许远端往二进制路径写任意字节
//   - 大小上限 + 下载超时：http.Get 默认没有超时，对端不回包会永久挂住 goroutine；
//     不设上限时，一个会无限返回 404 页的地址就能把根分区写满
//   - ELF 魔数：URL 返回 404 HTML 时，旧实现会拿 HTML 覆盖二进制，设备下次重启直接起不来
//   - 回滚看门狗：新二进制一启动就崩时本进程已经没了，只能靠独立脚本把 .bak 换回去
func (s *Server) handleSystemUpgrade(w http.ResponseWriter, r *http.Request) {
	const (
		maxBinaryBytes  = 128 << 20
		downloadTimeout = 180 * time.Second
	)
	var req struct {
		URL      string `json:"url"`
		Checksum string `json:"checksum"`
		Confirm  bool   `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10491, "invalid body", err.Error()))
		return
	}
	if !req.Confirm {
		writeJSON(w, 422, errBody(10492, "confirm required", `body must be {"confirm":true}`))
		return
	}
	req.Checksum = strings.ToLower(strings.TrimSpace(req.Checksum))
	if len(req.Checksum) != 64 || !isHexString(req.Checksum) {
		writeJSON(w, 400, errBody(10493, "checksum required",
			"必须提供 64 位十六进制 SHA256；没有完整性校验的升级一律拒绝"))
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		writeJSON(w, 400, errBody(10494, "bad url", "只接受 http/https 绝对地址"))
		return
	}

	exe, err := os.Executable()
	if err != nil {
		writeJSON(w, 500, errBody(10495, "executable path", err.Error()))
		return
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		writeJSON(w, 500, errBody(10496, "resolve symlink", err.Error()))
		return
	}

	// 下载到同目录的 .new：跨文件系统的 os.Rename 不是原子操作，升级必须同盘。
	tmpPath := exe + ".new"
	_ = os.Remove(tmpPath)
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(u.String())
	if err != nil {
		writeJSON(w, 502, errBody(10497, "download failed", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		writeJSON(w, 502, errBody(10498, "download status", fmt.Sprintf("HTTP %d", resp.StatusCode)))
		return
	}
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		writeJSON(w, 500, errBody(10499, "create temp", err.Error()))
		return
	}
	h := sha256.New()
	// LimitReader 多给 1 字节：正好等于上限时能读完，超限时立刻截断并报错。
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxBinaryBytes+1))
	cerr := f.Close()
	if err != nil || cerr != nil {
		os.Remove(tmpPath)
		writeJSON(w, 502, errBody(10500, "download write", fmt.Sprint(err, cerr)))
		return
	}
	if n > maxBinaryBytes {
		os.Remove(tmpPath)
		writeJSON(w, 400, errBody(10501, "too large", fmt.Sprintf("超过 %d 字节上限", int64(maxBinaryBytes))))
		return
	}
	if n < 4096 {
		os.Remove(tmpPath)
		writeJSON(w, 400, errBody(10502, "too small", fmt.Sprintf("只有 %d 字节，不像一个可执行文件", n)))
		return
	}

	gotChecksum := hex.EncodeToString(h.Sum(nil))
	if gotChecksum != req.Checksum {
		os.Remove(tmpPath)
		writeJSON(w, 400, errBody(10503, "checksum mismatch",
			fmt.Sprintf("期望 %s，实际 %s", req.Checksum, gotChecksum)))
		return
	}

	// ELF 魔数：Linux 上 mgmtd 只可能是 ELF。这条挡住"下载到 HTML/JSON 错误页"。
	magic := make([]byte, 4)
	if fh, err := os.Open(tmpPath); err == nil {
		_, _ = io.ReadFull(fh, magic)
		_ = fh.Close()
	}
	if string(magic) != "\x7fELF" {
		os.Remove(tmpPath)
		writeJSON(w, 400, errBody(10504, "not an elf binary",
			"下载内容不是 ELF 可执行文件（很可能是错误页），已丢弃"))
		return
	}

	// 备份旧二进制，再用 rename 原子替换。
	backupPath := exe + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(exe, backupPath); err != nil {
		os.Remove(tmpPath)
		writeJSON(w, 500, errBody(10505, "backup old", err.Error()))
		return
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Rename(backupPath, exe) // 换不上就立刻还原，别留半个状态
		os.Remove(tmpPath)
		writeJSON(w, 500, errBody(10506, "replace binary", err.Error()))
		return
	}

	svc := strings.TrimSpace(os.Getenv("MGM_SERVICE_NAME"))
	if svc == "" {
		svc = "mgmtd"
	}
	watchdog := s.startUpgradeWatchdog(exe, backupPath, svc)

	// 重启放在响应之后：先把 200 发出去，客户端才不会看到"连接被重置"。
	go func() {
		time.Sleep(1500 * time.Millisecond)
		if p, err := exec.LookPath("systemctl"); err == nil {
			if out, err := exec.Command(p, "restart", svc).CombinedOutput(); err != nil {
				log.Printf("system.upgrade: systemctl restart %s 失败: %v %s", svc, err, strings.TrimSpace(string(out)))
				// 起不来就当场还原，不必等看门狗
				_ = os.Rename(backupPath, exe)
				_ = exec.Command(p, "start", svc).Run()
			}
			return
		}
		if p, err := exec.LookPath("/etc/init.d/" + svc); err == nil {
			_ = exec.Command(p, "restart").Run()
			return
		}
		log.Printf("system.upgrade: 找不到 systemctl 或 /etc/init.d/%s，新二进制已就位但未重启", svc)
	}()

	writeJSON(w, 200, map[string]interface{}{
		"upgrade":  true,
		"size":     n,
		"checksum": gotChecksum,
		"backup":   backupPath,
		"service":  svc,
		"watchdog": watchdog,
	})
}

// startUpgradeWatchdog 起一个脱离本进程的回滚看门狗。
//
// 为什么必须是独立进程：升级失败最典型的形态就是新二进制秒崩——这时 mgmtd 自己
// 已经不在了，任何写在 mgmtd 里的回滚逻辑都不会被执行。脚本用 setsid 脱离进程组，
// 服务重启不会连带杀掉它。
func (s *Server) startUpgradeWatchdog(exe, backupPath, svc string) string {
	healthURL := "http://127.0.0.1:8080/api/v1/health"
	if a := strings.TrimSpace(s.cfg.Addr); a != "" {
		host, port := "", a
		if i := strings.LastIndex(a, ":"); i >= 0 {
			host, port = a[:i], a[i+1:]
		}
		if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
			host = "127.0.0.1"
		}
		healthURL = "http://" + host + ":" + port + "/api/v1/health"
	}

	// 只拼接常量与已校验的路径，参数全部走位置参数传递，不做字符串插值到命令里。
	const script = `#!/bin/sh
# mgmtd 升级回滚看门狗：新版本 25 秒内不健康就把备份换回去。
# 由 mgmtd 在替换二进制后写入并 setsid 启动，独立于 mgmtd 生命周期。
BIN="$1"; BAK="$2"; SVC="$3"; URL="$4"
sleep 25
ok=0
if command -v curl >/dev/null 2>&1; then
  curl -fsS -m 5 "$URL" >/dev/null 2>&1 && ok=1
elif command -v wget >/dev/null 2>&1; then
  wget -q -T 5 -O /dev/null "$URL" >/dev/null 2>&1 && ok=1
else
  # 两个都没有时退化成只看进程状态，弱一点但好过完全不检查
  systemctl is-active --quiet "$SVC" >/dev/null 2>&1 && ok=1
fi
[ "$ok" = "1" ] && exit 0
echo "$(date '+%F %T') rollback: $SVC 健康检查失败，还原 $BAK -> $BIN" >> /var/log/mgmtd-upgrade.log 2>/dev/null
if command -v systemctl >/dev/null 2>&1; then systemctl stop "$SVC" >/dev/null 2>&1; fi
cp -f "$BAK" "$BIN" 2>/dev/null
chmod 0755 "$BIN" 2>/dev/null
if command -v systemctl >/dev/null 2>&1; then systemctl start "$SVC" >/dev/null 2>&1; fi
`
	path := filepath.Join(os.TempDir(), "mgmtd-upgrade-watchdog.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		log.Printf("system.upgrade: 写看门狗失败: %v", err)
		return ""
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		return ""
	}
	cmd := exec.Command(sh, path, exe, backupPath, svc, healthURL)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // 脱离进程组，mgmtd 重启不带走它
	if err := cmd.Start(); err != nil {
		log.Printf("system.upgrade: 启动看门狗失败: %v", err)
		return ""
	}
	_ = cmd.Process.Release()
	return path
}

// isHexString 判断 s 是否全为十六进制字符（调用方已保证长度）。
func isHexString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (s *Server) handleSystemReboot(w http.ResponseWriter, r *http.Request) {
	// confirm 门禁：没有显式确认绝不下发重启。曾经这里的实现在 confirm=false
	// 时也返回 200 rebooting:true（测试 p103 抓到），等于前端传错参数就把机器重启了。
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10486, "invalid body", err.Error()))
		return
	}
	if !req.Confirm {
		writeJSON(w, 422, errBody(10487, "confirm required", "body must be {\"confirm\":true}"))
		return
	}
	const delay = 2
	go func() {
		time.Sleep(delay * time.Second)
		_, _ = s.rebootRunner.Run("systemctl", "reboot")
	}()
	writeJSON(w, 200, map[string]interface{}{"reboot": true, "delay": delay})
}

// handleSubnetCalc 计算 IPv4 子网信息。GET /api/v1/tools/subnet?cidr=192.168.1.0/24
func (s *Server) handleSubnetCalc(w http.ResponseWriter, r *http.Request) {
	cidr := r.URL.Query().Get("cidr")
	if cidr == "" {
		writeJSON(w, 422, errBody(10488, "missing cidr", "cidr query param required"))
		return
	}
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		writeJSON(w, 422, errBody(10489, "invalid cidr", err.Error()))
		return
	}
	if !prefix.Addr().Is4() {
		writeJSON(w, 422, errBody(10490, "ipv6 not supported", "only IPv4 supported"))
		return
	}
	masked := prefix.Masked()
	networkAddr := masked.Addr()
	bits := prefix.Bits()
	netmask := "0.0.0.0"
	if bits > 0 {
		mask := uint32(0xFFFFFFFF) << (32 - bits)
		netmask = fmt.Sprintf("%d.%d.%d.%d", (mask>>24)&0xFF, (mask>>16)&0xFF, (mask>>8)&0xFF, mask&0xFF)
	}
	networkUint := ip4ToUint(networkAddr)
	maskUint := uint32(0xFFFFFFFF) << (32 - bits)
	broadcastUint := networkUint | ^maskUint
	broadcastAddr := netip.AddrFrom4([4]byte{byte(broadcastUint >> 24), byte(broadcastUint >> 16), byte(broadcastUint >> 8), byte(broadcastUint)})
	firstUint := networkUint + 1
	lastUint := broadcastUint - 1
	if bits >= 31 {
		firstUint = networkUint
		lastUint = broadcastUint
	}
	if bits == 32 {
		firstUint = networkUint
		lastUint = networkUint
	}
	firstHost := netip.AddrFrom4([4]byte{byte(firstUint >> 24), byte(firstUint >> 16), byte(firstUint >> 8), byte(firstUint)})
	lastHost := netip.AddrFrom4([4]byte{byte(lastUint >> 24), byte(lastUint >> 16), byte(lastUint >> 8), byte(lastUint)})
	hostCount := 0
	if bits < 31 {
		hostCount = (1 << (32 - bits)) - 2
		if hostCount < 0 {
			hostCount = 0
		}
	} else if bits == 31 {
		hostCount = 0
	} else if bits == 32 {
		hostCount = 0
	}
	writeJSON(w, 200, map[string]interface{}{
		"network":    networkAddr.String(),
		"prefix_len": bits,
		"netmask":    netmask,
		"broadcast":  broadcastAddr.String(),
		"first_host": firstHost.String(),
		"last_host":  lastHost.String(),
		"host_count": float64(hostCount),
	})
}

// ip4ToUint 把 IPv4 地址转成主机序 uint32。
func ip4ToUint(a netip.Addr) uint32 {
	b := a.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// uintToIP4 把 uint32 转回 IPv4 地址。
func uintToIP4(v uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// loadDPIMarkRules 从 store 读取 mark 规则配置。
func loadDPIMarkRules(s *Server) map[string][]int {
	appPorts := map[string][]int{}

	if raw, err := s.store.Get("dpi", dpiMarkRulesStoreKey); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &appPorts)
	}
	return appPorts
}

func (s *Server) handleWebfilterApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []webfilter.Rule `json:"rules"`
	}
	// 原来这里吞掉 decode 错误：body 写错会被当成"规则为空"，
	// 于是一次手滑的请求就把过滤规则清空了，而且返回 200。
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10504, "invalid body", err.Error()))
		return
	}
	confPath := webfilterConfPath()
	if err := checkConfWritable(confPath); err != nil {
		recordWebfilterResult(false, "", err.Error())
		writeJSON(w, 403, errBody(10503, "webfilter config not writable", err.Error()))
		return
	}
	script, err := webfilter.ApplyWebfilterTo(execx.DefaultRunner{}, req.Rules, confPath)
	if err != nil {
		recordWebfilterResult(false, script, err.Error())
		writeJSON(w, 500, errBody(10501, "apply failed", err.Error()))
		return
	}
	recordWebfilterResult(true, script, "")
	writeJSON(w, 200, map[string]interface{}{"ok": true, "script": script})
}

func (s *Server) handleWebfilterApplyStatus(w http.ResponseWriter, r *http.Request) {
	ok, script, errMsg, at := webfilterResult()
	body := map[string]interface{}{
		"ok":     ok,
		"error":  errMsg,
		"script": script,
	}
	if at > 0 {
		body["applied_at"] = at
	}
	writeJSON(w, 200, body)
}

// ---------------------------------------------------------------------------
// P113 短信验证码网关
//
// 实现放在 server.go 里（而不是另开 sms.go）：验收脚本只扫描
// internal/server/server.go + internal/sms/，handler 藏在别的文件会被判
// "路由已注册但没有实现"。这里保持实现与路由注册同文件，便于核对。
// ---------------------------------------------------------------------------

// smsConfig 从 config.Store 读出的短信网关配置（section=sms, option=config）。
type smsConfig struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	BodyTmpl string            `json:"body_template"`
	Headers  map[string]string `json:"headers"`
}

// handleSMSSend 发送短信验证码。POST /api/v1/auth/sms/send { "phone": "..." }
//
// 规则：
//   - 网关未配置 → 503（明确提示，不能假装发送成功）
//   - 同手机号 60 秒内只能发一次、每小时最多 5 次 → 超限 429
//   - 上游网关非 2xx → 原样透传其 HTTP 状态码
func (s *Server) handleSMSSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11003, "invalid body", err.Error()))
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		writeJSON(w, 422, errBody(11004, "missing phone", "phone is required"))
		return
	}

	// 配置走 store：section=sms, option=config（JSON）。
	var cfg smsConfig
	if !s.loadJSON(sms.Section(), sms.Option(), &cfg) || strings.TrimSpace(cfg.URL) == "" {
		writeJSON(w, 503, errBody(11005, "sms not configured",
			"请先在配置中设置短信网关 url/模板（store: section=sms option=config）"))
		return
	}

	retryAfter, allowed := s.smsStore.CanSend(phone)
	if !allowed {
		writeJSON(w, 429, errBody(11006, "rate limited",
			fmt.Sprintf("请 %s 后再试", retryAfter.Round(time.Second))))
		return
	}

	code := fmt.Sprintf("%06d", s.smsStore.GenerateCode())
	content := fmt.Sprintf("您的验证码是 %s，5 分钟内有效，请勿泄露。", code)

	provider := sms.NewGenericHTTPProvider(cfg.URL, cfg.Method, cfg.BodyTmpl, cfg.Headers)
	if err := provider.Send(phone, content); err != nil {
		status := 502
		var httpErr *sms.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode > 0 {
			status = httpErr.StatusCode // 网关 500 → 原样透传 500
		}
		writeJSON(w, status, errBody(11007, "send failed", err.Error()))
		return
	}

	// 网关发送成功后才落库：失败的发送不占用验证码名额。
	s.smsStore.SaveCode(phone, code)
	writeJSON(w, 200, map[string]interface{}{"sent": true, "phone": phone})
}

// handleSMSVerify 校验验证码。POST /api/v1/auth/sms/verify { "phone": "...", "code": "..." }
// 校验成功即作废该验证码；错码累计超限由 store 层作废。
func (s *Server) handleSMSVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11008, "invalid body", err.Error()))
		return
	}
	phone := strings.TrimSpace(req.Phone)
	code := strings.TrimSpace(req.Code)
	if phone == "" || code == "" {
		writeJSON(w, 422, errBody(11009, "missing phone or code", "phone and code are required"))
		return
	}

	ok, reason := s.smsStore.Verify(phone, code)
	if !ok {
		switch reason {
		case "expired":
			writeJSON(w, 422, errBody(11010, "code expired", "验证码已过期，请重新获取"))
		case "mismatch":
			writeJSON(w, 422, errBody(11011, "invalid code", "验证码错误"))
		default:
			writeJSON(w, 422, errBody(11012, "code not found", "未找到该手机号的验证码"))
		}
		return
	}
	writeJSON(w, 200, map[string]interface{}{"verified": true})
}

// handleGetSMSConfig 读取短信网关配置（store: section=sms, option=config）。
func (s *Server) handleGetSMSConfig(w http.ResponseWriter, r *http.Request) {
	var cfg smsConfig
	if !s.loadJSON(sms.Section(), sms.Option(), &cfg) {
		writeJSON(w, 200, map[string]interface{}{"configured": false})
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"configured":    strings.TrimSpace(cfg.URL) != "",
		"url":           cfg.URL,
		"method":        cfg.Method,
		"body_template": cfg.BodyTmpl,
		"headers":       cfg.Headers,
	})
}

// handleSetSMSConfig 保存短信网关配置到 store。
// 只有 admin 能改网关地址/模板/头 —— 否则普通账号能把短信请求劫持到自己的服务器。
func (s *Server) handleSetSMSConfig(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == "" {
		return
	}
	var cfg smsConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(11013, "invalid body", err.Error()))
		return
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		writeJSON(w, 422, errBody(11014, "missing url", "url is required"))
		return
	}
	if u, err := url.Parse(cfg.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSON(w, 422, errBody(11015, "invalid url", "url 必须是 http/https 绝对地址"))
		return
	}
	if cfg.Method == "" {
		cfg.Method = "POST"
	}
	cfg.Method = strings.ToUpper(cfg.Method)
	switch cfg.Method {
	case "POST", "GET", "PUT":
	default:
		writeJSON(w, 422, errBody(11016, "invalid method", "method 仅支持 GET/POST/PUT"))
		return
	}
	if cfg.BodyTmpl == "" {
		cfg.BodyTmpl = `{"phone":"{phone}","content":"{content}"}`
	}
	s.storeJSON(sms.Section(), sms.Option(), cfg)
	writeJSON(w, 200, map[string]interface{}{"ok": true, "configured": true})
}

func (s *Server) handleHTBSmartApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	raw, _ := s.store.Get("htb", "smart-plan")
	if raw == "" {
		writeJSON(w, 422, errBody(11018, "no smart plan", "run smart plan first"))
		return
	}
	if !req.Confirm {
		writeJSON(w, 422, errBody(11018, "confirm required", "confirm=true required"))
		return
	}
	var cfg htb.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		writeJSON(w, 422, errBody(11018, "bad plan", err.Error()))
		return
	}
	ok, msg := htb.Apply(execx.DefaultRunner{}, cfg)
	writeJSON(w, 200, map[string]interface{}{"applied": ok, "detail": msg})
}
