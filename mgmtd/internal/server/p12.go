package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/bond"
	"github.com/jimwong/router-re/mgmtd/internal/certmgr"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/online"
	"github.com/jimwong/router-re/mgmtd/internal/porttrigger"
)

const (
	bondStoreKey = "bond/config"
	bondTuneKey  = "bond/tune"
	trigStoreKey = "porttrigger/rules"
	algStoreKey  = "porttrigger/alg"
	mdnsStoreKey = "porttrigger/mdns"
	webCertPath  = "/etc/ssl/router-re/cert.pem"
	webKeyPath   = "/etc/ssl/router-re/key.pem"
)

// ---------- 链路聚合 ----------

func (s *Server) handleGetBond(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("bond", bondStoreKey)
	var cfg bond.BondConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	live := map[string]string{}
	if cfg.Name != "" {
		if m, err := bond.ReadBond(execx.DefaultRunner{}, cfg.Name); err == nil {
			live = m
		}
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "live": live, "script": bond.RenderScript(cfg)})
}

func (s *Server) handleSetBond(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool            `json:"confirm"`
		Config  bond.BondConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10330, "invalid body", err.Error()))
		return
	}
	// ValidateBond 会去查 slave 接口是否存在，所以需要 Runner
	if err := bond.ValidateBond(execx.DefaultRunner{}, body.Config); err != nil {
		writeJSON(w, 422, errBody(10331, "invalid bond config", err.Error()))
		return
	}
	script := bond.RenderScript(body.Config)
	if !body.Confirm {
		writeJSON(w, 200, map[string]interface{}{
			"applied": false, "dry_run": true, "script": script,
			"detail": "dry-run：建 bond 会先把 slave 接口 down 再 enslave，" +
				"如果这些接口承载管理流量会立刻断连。确认请带 confirm=true。",
		})
		return
	}
	ok, detail := bond.Apply(execx.DefaultRunner{}, body.Config)
	if ok {
		b, _ := json.Marshal(body.Config)
		s.store.Set("bond", bondStoreKey, string(b))
	}
	writeJSON(w, 200, map[string]interface{}{"applied": ok, "detail": detail, "script": script})
}

func (s *Server) handleGetIfaceTune(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("iface")
	if iface == "" {
		writeJSON(w, 422, errBody(10332, "缺少 iface 参数", ""))
		return
	}
	live, err := bond.ReadTune(execx.DefaultRunner{}, iface)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"iface": iface, "live": map[string]string{}, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"iface": iface, "live": live})
}

func (s *Server) handleSetIfaceTune(w http.ResponseWriter, r *http.Request) {
	var cfg bond.TuneConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10333, "invalid body", err.Error()))
		return
	}
	if err := bond.ValidateTune(cfg); err != nil {
		writeJSON(w, 422, errBody(10334, "invalid tune config", err.Error()))
		return
	}
	ok, detail := bond.ApplyTune(execx.DefaultRunner{}, cfg)
	if ok {
		b, _ := json.Marshal(cfg)
		s.store.Set("bond", bondTuneKey, string(b))
	}
	writeJSON(w, 200, map[string]interface{}{"applied": ok, "detail": detail})
}

// ---------- 证书 ----------

// GET /api/v1/cert/inspect —— 读当前 Web 证书（不返回私钥）。
func (s *Server) handleGetCert(w http.ResponseWriter, r *http.Request) {
	pemBytes, err := os.ReadFile(webCertPath)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{
			"exists": false, "path": webCertPath,
			"detail": "尚未配置证书，Web 管理可能用自签或明文。可用 POST /cert/self-signed 生成。",
		})
		return
	}
	info, err := certmgr.InspectPEM(pemBytes)
	if err != nil {
		writeJSON(w, 422, errBody(10335, "证书解析失败", err.Error()))
		return
	}
	now := time.Now().Unix()
	left := certmgr.DaysLeft(info, now)
	validErr := certmgr.Validate(info, now)
	resp := map[string]interface{}{
		"exists": true, "path": webCertPath, "info": info,
		"days_left": left, "valid": validErr == nil,
	}
	if validErr != nil {
		resp["warning"] = validErr.Error()
	}
	writeJSON(w, 200, resp)
}

// POST /api/v1/cert/self-signed —— 生成自签证书并写入，返回证书信息（绝不回私钥）。
func (s *Server) handleSelfSignedCert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hostnames []string `json:"hostnames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Hostnames = nil
	}
	certPEM, keyPEM, err := certmgr.GenerateSelfSigned(body.Hostnames, time.Now().Unix())
	if err != nil {
		writeJSON(w, 500, errBody(10336, "生成证书失败", err.Error()))
		return
	}
	if err := os.MkdirAll("/etc/ssl/router-re", 0o755); err != nil {
		writeJSON(w, 500, errBody(10337, "创建目录失败", err.Error()))
		return
	}
	// 私钥 0600：同机其他用户不能读
	if err := os.WriteFile(webKeyPath, keyPEM, 0o600); err != nil {
		writeJSON(w, 500, errBody(10338, "写私钥失败", err.Error()))
		return
	}
	if err := os.WriteFile(webCertPath, certPEM, 0o644); err != nil {
		writeJSON(w, 500, errBody(10339, "写证书失败", err.Error()))
		return
	}
	info, _ := certmgr.InspectPEM(certPEM)
	writeJSON(w, 200, map[string]interface{}{
		"info": info, "cert_path": webCertPath, "key_path": webKeyPath,
		// 只回长度，不回内容
		"cert_bytes": len(certPEM), "key_bytes": len(keyPEM),
		"detail": "已写入。Web 服务需要重新加载才会使用新证书（本接口不会重启服务）。",
	})
}

// ---------- 在线用户 ----------

func (s *Server) handleGetOnline(w http.ResponseWriter, r *http.Request) {
	sessions := map[string]online.Session{}
	if b, err := os.ReadFile("/proc/net/nf_conntrack"); err == nil {
		sessions = online.ParseConntrackSessions(string(b))
	}
	// 账号 -> 绑定 IP 的映射来自账号表
	accounts := map[string]string{}
	for _, a := range s.accountMgr.List() {
		accounts[a.Username] = ""
	}
	list := online.Merge(accounts, sessions)

	// 如实说明为什么字节数是 0
	acctOn := false
	if b, err := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct"); err == nil {
		acctOn = string(b) == "1\n" || string(b) == "1"
	}
	resp := map[string]interface{}{
		"sessions": list, "count": len(list),
		"conntrack_entries": len(sessions),
		"byte_counting":     acctOn,
	}
	if !acctOn {
		resp["byte_counting_note"] = "内核未启用 nf_conntrack_acct，" +
			"所有会话的 rx/tx 字节数都是 0（不是没流量，是没统计）。"
	}
	writeJSON(w, 200, resp)
}

// POST /api/v1/online/kick —— 断开某用户。只删 conntrack 表项，不杀进程。
func (s *Server) handleKickOnline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		IP       string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10340, "invalid body", err.Error()))
		return
	}
	if body.IP == "" {
		writeJSON(w, 422, errBody(10341, "必须提供 ip",
			"断开连接按 IP 删 conntrack 表项。没有 IP 就无法定位会话 —— "+
				"绝不按用户名去杀进程，那可能中断同名的其他用户。"))
		return
	}
	cmd, err := online.Kick(body.Username, body.IP)
	if err != nil {
		writeJSON(w, 422, errBody(10342, "生成断开命令失败", err.Error()))
		return
	}
	_, err = (execx.DefaultRunner{}).Run("conntrack", "-D", "-s", body.IP)
	if err != nil {
		writeJSON(w, 500, errBody(10343, "断开失败", "命令: "+cmd+" → "+err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"kicked": true, "command": cmd, "ip": body.IP,
		"detail": "已删除该 IP 的 conntrack 表项。如果对方是 PPPoE 拨号，重新拨号会立刻恢复连接。",
	})
}

// ---------- 端口触发 / ALG / mDNS ----------

func (s *Server) handleGetPortTrigger(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("porttrigger", trigStoreKey)
	rules := []porttrigger.Trigger{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": rules, "count": len(rules),
		"nft": porttrigger.RenderTable(rules),
	})
}

func (s *Server) handleSetPortTrigger(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rules   []porttrigger.Trigger `json:"rules"`
		Confirm bool                  `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10344, "invalid body", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空所有端口触发规则并返回 200。
	// 必须显式 confirm:true 才允许清空。
	if len(body.Rules) == 0 && !body.Confirm {
		writeJSON(w, 422, errBody(10347, "rules must not be empty",
			"to clear all port triggers, send {rules:[],confirm:true}"))
		return
	}
	for i, t := range body.Rules {
		if err := porttrigger.Validate(t); err != nil {
			writeJSON(w, 422, errBody(10345, "invalid trigger",
				"rules["+itoa(i)+"] "+t.Name+": "+err.Error()))
			return
		}
	}
	b, _ := json.Marshal(body.Rules)
	s.store.Set("porttrigger", trigStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{
		"rules": body.Rules, "count": len(body.Rules),
		"nft": porttrigger.RenderTable(body.Rules),
	})
}

func (s *Server) handleGetALG(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("porttrigger", algStoreKey)
	var cfg porttrigger.ALGConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg, "enabled_any": porttrigger.AlgsEnabled(cfg),
		"script": porttrigger.RenderALG(cfg),
	})
}

func (s *Server) handleSetALG(w http.ResponseWriter, r *http.Request) {
	var cfg porttrigger.ALGConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10346, "invalid body", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("porttrigger", algStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{
		"config": cfg, "enabled_any": porttrigger.AlgsEnabled(cfg),
		"script": porttrigger.RenderALG(cfg),
	})
}

func (s *Server) handleGetMDNS(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("porttrigger", mdnsStoreKey)
	var cfg porttrigger.MDNSRelay
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "avahi": porttrigger.RenderAvahi(cfg)})
}

func (s *Server) handleSetMDNS(w http.ResponseWriter, r *http.Request) {
	var cfg porttrigger.MDNSRelay
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10347, "invalid body", err.Error()))
		return
	}
	if err := porttrigger.ValidateMDNS(cfg); err != nil {
		writeJSON(w, 422, errBody(10348, "invalid mdns config", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("porttrigger", mdnsStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "avahi": porttrigger.RenderAvahi(cfg)})
}
