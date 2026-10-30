package logcenter

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// 日志分类。
//
// iKuai 把日志按用途分成 ARP / DDNS / 拨号 / 推送 / 认证 / DHCP / 系统事件 /
// webadmin 访问 / weblogin 登录 九类。分类的价值不在于换个标题，而在于
// **每一类对应一个确定的数据来源**：判断"WAN 为什么断了"要去 dial，
// 判断"谁改了配置"要去 webadmin，两者混在一个大列表里等于没有日志。
//
// 数据来源分两种：
//  1. 系统日志（journalctl / syslog）：靠 unit 或关键字过滤
//  2. 审计文件（audit.jsonl）：本机所有管理操作，按 action 前缀过滤
//
// 后者在容器里也可能是唯一有数据的一类，所以不能只做 systemd 一条路。
type Category struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Unit     string `json:"unit,omitempty"`
	Grep     string `json:"grep,omitempty"`
	AuditAll bool   `json:"audit_all,omitempty"`
	// AuditActions 非空表示这类日志从审计文件取，按 action 前缀匹配
	AuditActions []string `json:"-"`
}

// Categories 是九个分类的完整定义，顺序即界面展示顺序。
var Categories = []Category{
	{Key: "arp", Name: "ARP 日志", Desc: "IP 与 MAC 的绑定变化、ARP 冲突/欺骗告警", Grep: "(?i)arp"},
	{Key: "ddns", Name: "DDNS 日志", Desc: "动态域名解析的更新与失败", Grep: "(?i)ddns|dyndns|dnspod"},
	{Key: "dial", Name: "拨号日志", Desc: "PPPoE / DHCP 上联的拨号过程和断线重连", Unit: "pppd", Grep: "(?i)ppp|wan|link"},
	{Key: "push", Name: "推送日志", Desc: "告警推送到手机/邮件/Webhook 的记录", Grep: "(?i)push|notify|webhook"},
	{Key: "auth", Name: "认证日志", Desc: "系统账号的认证与 sudo 提权", Unit: "sshd", Grep: "(?i)auth|password|sudo"},
	{Key: "dhcp", Name: "DHCP 日志", Desc: "地址分配、租约续期与冲突", Unit: "dnsmasq", Grep: "(?i)dhcp"},
	{Key: "system", Name: "系统事件", Desc: "内核与服务的重要事件（warning 及以上）"},
	{Key: "webadmin", Name: "webadmin 访问", Desc: "管理界面上的配置变更（谁、改了什么、结果）", AuditAll: true},
	{Key: "weblogin", Name: "weblogin 登录", Desc: "管理界面的登录与登出，含失败尝试",
		AuditActions: []string{"auth.login", "auth.logout"}},
}

// FindCategory 按 key 查分类。
func FindCategory(key string) (Category, bool) {
	for _, c := range Categories {
		if c.Key == key {
			return c, true
		}
	}
	return Category{}, false
}

// Result 是一次分类查询的结果，带上真实数据来源，便于界面说明
// "这些是命中的日志" 还是 "这一类在这个环境里没有日志源"。
type Result struct {
	Category string     `json:"category"`
	Name     string     `json:"name"`
	Source   string     `json:"source"`
	Entries  []LogEntry `json:"entries"`
	Note     string     `json:"note,omitempty"`
}

// DefaultAuditPath 是审计日志的默认位置，与 server 侧的默认值保持一致。
const DefaultAuditPath = "/var/log/router-re/audit.jsonl"

// QueryCategory 查询某一类日志。
//
// auditPath 传空时用 DefaultAuditPath；调用方应传入实际生效的路径
// （环境变量 MGM_AUDIT_LOG 可能改过），否则 webadmin/weblogin 会读空。
func QueryCategory(key string, limit int, since string, auditPath string) (Result, error) {
	cat, ok := FindCategory(key)
	if !ok {
		return Result{}, fmt.Errorf("未知的日志分类 %q", key)
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 5000 {
		limit = 5000
	}
	res := Result{Category: cat.Key, Name: cat.Name}

	if cat.AuditAll || len(cat.AuditActions) > 0 {
		path := auditPath
		if path == "" {
			path = DefaultAuditPath
		}
		entries, err := queryAudit(path, cat, limit, since)
		if err != nil {
			if os.IsNotExist(err) {
				res.Source = "audit:" + path
				res.Note = "审计日志文件不存在（尚无管理操作被记录）"
				res.Entries = []LogEntry{}
				return res, nil
			}
			return res, err
		}
		res.Source = "audit:" + path
		res.Entries = sortDesc(entries)
		if len(entries) == 0 {
			res.Note = "审计文件存在，但这一类还没有记录"
		}
		return res, nil
	}

	// 走系统日志
	opts := QueryOpts{Limit: limit, Since: since, Unit: cat.Unit, Grep: cat.Grep}
	entries, err := Query(opts)
	if err != nil {
		res.Source = "journalctl/syslog"
		res.Entries = []LogEntry{}
		res.Note = "系统日志不可用：" + err.Error()
		return res, nil
	}
	src := "journalctl"
	if cat.Unit != "" {
		src = "journalctl:" + cat.Unit
	}
	res.Source = src
	res.Entries = sortDesc(entries)
	if len(entries) == 0 {
		res.Note = "这一类在当前系统上没有匹配的日志"
	}
	return res, nil
}

// queryAudit 从审计文件里挑出属于该分类的记录。
//
// 文件是 append-only JSONL，可能很大，所以从尾部往前读，凑够 limit 就停。
func queryAudit(path string, cat Category, limit int, since string) ([]LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sinceTS int64
	if since != "" {
		if t, err := parseSince(since); err == nil {
			sinceTS = t
		}
	}

	// 一次性读全文：审计文件按天轮转，单文件通常在几十 MB 内；
	// 逐行倒读要处理不等长记录，复杂度换不来收益。
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	out := make([]LogEntry, 0, limit)
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var ev struct {
			TS       int64  `json:"ts"`
			Actor    string `json:"actor"`
			Action   string `json:"action"`
			Target   string `json:"target"`
			Result   string `json:"result"`
			Detail   string `json:"detail"`
			RemoteIP string `json:"remote_ip"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if sinceTS > 0 && ev.TS < sinceTS {
			continue
		}
		if !auditMatches(cat, ev.Action) {
			continue
		}
		prio := "info"
		if ev.Result != "ok" && ev.Result != "" {
			prio = "warning"
		}
		msg := fmt.Sprintf("%s %s → %s（%s）", ev.Actor, ev.Action, ev.Target, ev.Result)
		if ev.Detail != "" {
			msg += " " + ev.Detail
		}
		if ev.RemoteIP != "" {
			msg += " from " + ev.RemoteIP
		}
		out = append(out, LogEntry{
			TS:       time.Unix(ev.TS, 0).Format("2006-01-02 15:04:05"),
			Unit:     "mgmtd",
			Priority: prio,
			Message:  msg,
		})
	}
	// 倒读天然是"新→旧"，与界面展示顺序一致，不再翻转
	return out, nil
}

// sortDesc 把日志按时间从新到旧排序。
//
// 两种来源的原始顺序不同（journalctl 新→旧、审计文件我按倒读也是新→旧），
// 但排序这件事不应该依赖"恰好一致"——统一在出口排一次，界面顺序才稳定。
func sortDesc(in []LogEntry) []LogEntry {
	out := make([]LogEntry, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out
}

// auditMatches 判断一条审计记录是否属于该分类。
func auditMatches(cat Category, action string) bool {
	if cat.AuditAll {
		// webadmin 是"配置变更"，登录/登出归 weblogin，不重复计入
		return !strings.HasPrefix(action, "auth.login") && !strings.HasPrefix(action, "auth.logout")
	}
	for _, p := range cat.AuditActions {
		if strings.HasPrefix(action, p) {
			return true
		}
	}
	return false
}

// sinceRe 匹配 "30m" / "2h" / "1 day ago" / "10 minutes" 这类相对时间。
var sinceRe = regexp.MustCompile(`(?i)(\d+)\s*(seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d)\b`)

// unitSeconds 把单位首字母映射成秒。
var unitSeconds = map[byte]int64{'s': 1, 'm': 60, 'h': 3600, 'd': 86400}

// parseSince 解析相对时间（"1 hour ago" / "30m" / "2h" / "1d"）或绝对时间。
func parseSince(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.Unix(), nil
	}
	m := sinceRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("无法解析时间 %q（支持 \"30m\" \"2h\" \"1 day ago\" 或 RFC3339）", s)
	}
	var n int64
	if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil {
		return 0, err
	}
	unit := strings.ToLower(m[2])[0]
	mult, ok := unitSeconds[unit]
	if !ok {
		return 0, fmt.Errorf("不支持的时间单位 %q", m[2])
	}
	return time.Now().Add(-time.Duration(n*mult) * time.Second).Unix(), nil
}

// CategorySources 报告每个分类在当前环境是否有可用数据源，供界面标注。
// （与 logcenter.Sources 区分：那个返回 syslog 文件列表）
func CategorySources(auditPath string) []map[string]interface{} {
	if auditPath == "" {
		auditPath = DefaultAuditPath
	}
	_, auditErr := os.Stat(auditPath)
	out := make([]map[string]interface{}, 0, len(Categories))
	for _, c := range Categories {
		m := map[string]interface{}{"key": c.Key, "name": c.Name, "desc": c.Desc}
		if c.AuditAll || len(c.AuditActions) > 0 {
			m["source"] = "audit:" + auditPath
			m["available"] = auditErr == nil
			if auditErr != nil {
				m["note"] = "审计文件尚未创建"
			}
		} else {
			m["source"] = "journalctl/syslog"
			m["available"] = true // journalctl 不可用时 Query 会降级，实际可用性由查询结果反映
		}
		out = append(out, m)
	}
	return out
}
