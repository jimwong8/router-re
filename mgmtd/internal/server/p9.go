package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/accounting"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/ipv6"
	"github.com/jimwong/router-re/mgmtd/internal/mirror"
	"github.com/jimwong/router-re/mgmtd/internal/multidial"
)

const (
	ndStoreKey     = "ipv6/nd"
	pdStoreKey     = "ipv6/pd"
	mirrorStoreKey = "mirror/config"
	sessionsPath   = "/etc/ppp/peers/router-re-sessions.json"
	dnsc6cConfPath = "/etc/wide-dhcpv6/dhcp6c.conf"
	accountingKey  = "accounting/counter"
)

// ---------- IPv6 ND ----------

func (s *Server) handleGetIPv6ND(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("iface")
	if iface == "" {
		iface = "ens18"
	}
	raw, _ := s.store.Get("ipv6", ndStoreKey)
	var cfg ipv6.NDConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	live := map[string]interface{}{}
	if s.aclApplier != nil {
		if got, err := ipv6.ReadND(execx.DefaultRunner{}, iface); err == nil {
			live = map[string]interface{}{
				"iface": got.Iface, "hop_limit": got.HopLimit, "mtu": got.Mtu,
				"reachable_time_ms": got.ReachableTimeMs, "retrans_timer_ms": got.RetransTimerMs,
				"dad_transmits": got.DadTransmits, "accept_ra": got.AcceptRa, "forwarding": got.Forwarding,
			}
		} else {
			live = map[string]interface{}{"error": err.Error()}
		}
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "live": live})
}

func (s *Server) handleSetIPv6ND(w http.ResponseWriter, r *http.Request) {
	var cfg ipv6.NDConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10190, "invalid body", err.Error()))
		return
	}
	if err := ipv6.ValidateND(cfg); err != nil {
		writeJSON(w, 422, errBody(10191, "invalid nd config", err.Error()))
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10192, "exec runner not initialized", ""))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("ipv6", ndStoreKey, string(b))

	applyErr := ipv6.ApplyND(execx.DefaultRunner{}, cfg)
	detail := ""
	if applyErr != nil {
		detail = applyErr.Error()
	}
	writeJSON(w, 200, map[string]interface{}{"applied": applyErr == nil, "config": cfg, "detail": detail})
}

// ---------- IPv6 PD ----------

func (s *Server) handleGetIPv6PD(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("ipv6", pdStoreKey)
	var cfg ipv6.PDConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "dhcp6c": ipv6.RenderDhcp6c(cfg)})
}

func (s *Server) handleSetIPv6PD(w http.ResponseWriter, r *http.Request) {
	var cfg ipv6.PDConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10193, "invalid body", err.Error()))
		return
	}
	if err := ipv6.ValidatePD(cfg); err != nil {
		writeJSON(w, 422, errBody(10194, "invalid pd config", err.Error()))
		return
	}
	if s.aclApplier == nil {
		writeJSON(w, 500, errBody(10195, "exec runner not initialized", ""))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("ipv6", pdStoreKey, string(b))

	applyErr := ipv6.ApplyPD(execx.DefaultRunner{}, cfg, dnsc6cConfPath)
	detail := ""
	if applyErr != nil {
		detail = applyErr.Error()
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applyErr == nil,
		"config":  cfg,
		"dhcp6c":  ipv6.RenderDhcp6c(cfg),
		"detail":  detail,
	})
}

// ---------- 端口镜像 ----------

func (s *Server) handleGetMirror(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("mirror", mirrorStoreKey)
	var cfg mirror.Config
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"script":  mirror.RenderScript(cfg),
		"cleanup": mirror.Cleanup(cfg),
	})
}

func (s *Server) handleSetMirror(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool          `json:"confirm"`
		Config  mirror.Config `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 422, errBody(10196, "invalid body", err.Error()))
		return
	}
	if req.Config.SourceIface == "" {
		req.Config = mirror.Config{
			SourceIface: req.Config.SourceIface, TargetIface: req.Config.TargetIface,
			Direction: req.Config.Direction, Enabled: req.Config.Enabled,
		}
	}
	if err := mirror.Validate(req.Config); err != nil {
		writeJSON(w, 422, errBody(10197, "invalid mirror config", err.Error()))
		return
	}
	script := mirror.RenderScript(req.Config)
	if !req.Confirm {
		writeJSON(w, 200, map[string]interface{}{
			"applied": false, "dry_run": true, "script": script,
			"detail": "dry-run：未下发。端口镜像会重建该接口的 qdisc，确认请带 confirm=true。",
		})
		return
	}
	applied, detail := mirror.Apply(execx.DefaultRunner{}, req.Config)
	if applied {
		b, _ := json.Marshal(req.Config)
		s.store.Set("mirror", mirrorStoreKey, string(b))
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied, "dry_run": false, "script": script, "detail": detail,
	})
}

// ---------- PPPoE 多拨 ----------

func (s *Server) handleGetMultiDial(w http.ResponseWriter, r *http.Request) {
	ss, err := multidial.LoadSessions(sessionsPath)
	if err != nil {
		ss = []multidial.Session{}
	}
	// 密码绝不出现在响应里
	redacted := make([]map[string]interface{}, 0, len(ss))
	for _, x := range ss {
		redacted = append(redacted, map[string]interface{}{
			"name": x.Name, "iface": x.Iface, "username": x.Username,
			"password": "***", "service": x.Service, "ac": x.AC,
			"metric": x.Metric, "mru": x.MRU, "lcp_echo": x.LCPEcho, "enabled": x.Enabled,
			"peer": multidial.RedactPeer(x),
		})
	}
	writeJSON(w, 200, map[string]interface{}{"sessions": redacted, "path": sessionsPath})
}

func (s *Server) handleSetMultiDial(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sessions []multidial.Session `json:"sessions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 422, errBody(10198, "invalid body", err.Error()))
		return
	}
	if len(req.Sessions) == 0 {
		// 空数组是"假成功"：清空配置必须显式说明，不能静默接受
		writeJSON(w, 422, errBody(10199, "sessions 不能为空", "如需清空请显式传空数组并说明意图，或逐条停用"))
		return
	}
	for i, x := range req.Sessions {
		if err := multidial.ValidateSession(x); err != nil {
			writeJSON(w, 422, errBody(10200, "invalid session",
				"session["+strconv.Itoa(i)+"] "+x.Name+": "+err.Error()))
			return
		}
	}
	if err := multidial.SaveSessions(sessionsPath, req.Sessions); err != nil {
		writeJSON(w, 500, errBody(10201, "save sessions failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"count": len(req.Sessions), "path": sessionsPath,
		"detail": "已写入。启动拨号需要对每个会话执行 `pon " + req.Sessions[0].Name + "`（未自动执行）。",
	})
}

// ---------- 流量计费 ----------

// handleAccountingStatus 如实报告能否拿到真实的字节计数。
//
// 这台机器上 nf_conntrack_acct 默认是 0，此时 /proc/net/nf_conntrack 里
// 根本没有 bytes= 字段 —— 任何"已用流量"都只能是 0。返回 0 而不解释原因
// 就是在骗用户，所以这里必须把 acct_enabled 和启用方法一起返回。
func (s *Server) handleAccountingStatus(w http.ResponseWriter, r *http.Request) {
	acctOn := false
	if b, err := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct"); err == nil {
		acctOn = strings.TrimSpace(string(b)) == "1"
	}
	total, withBytes := 0, 0
	if b, err := os.ReadFile("/proc/net/nf_conntrack"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			total++
			if strings.Contains(line, "bytes=") {
				withBytes++
			}
		}
	}
	resp := map[string]interface{}{
		"acct_enabled":       acctOn,
		"conntrack_total":    total,
		"entries_with_bytes": withBytes,
	}
	if !acctOn || withBytes == 0 {
		resp["available"] = false
		resp["reason"] = "内核未启用连接计费（nf_conntrack_acct=0），/proc/net/nf_conntrack 没有 bytes= 字段，" +
			"无法得到真实已用流量。启用方法：sysctl -w net.netfilter.nf_conntrack_acct=1 " +
			"（注意：启用后只统计新连接，已有连接仍无字节数）。"
	} else {
		resp["available"] = true
	}
	writeJSON(w, 200, resp)
}

// handleAccountingReport 用量报表。用内存 counter（由采样接口累加）。
func (s *Server) handleAccountingReport(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("accounting", accountingKey)
	used := map[string]int{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &used)
	}
	// 配额从账号管理取
	quotas := map[string]int{}
	accts := s.accountMgr.List()
	for _, a := range accts {
		quotas[a.Username] = a.QuotaMB
	}
	report := accounting.BuildReport(used, quotas, map[string]int64{})
	writeJSON(w, 200, map[string]interface{}{"report": report, "count": len(report)})
}
