package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/diagreport"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/firmware"
	"github.com/jimwong/router-re/mgmtd/internal/notify"
	"github.com/jimwong/router-re/mgmtd/internal/policyroute"
)

const (
	smtpStoreKey = "notify/smtp"
	trapStoreKey = "notify/trap"
	prTablesKey  = "policyroute/tables"
	prRulesKey   = "policyroute/rules"
)

// ---------- 固件升级 ----------

func (s *Server) handleFirmwareInspect(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, 422, errBody(10300, "缺少 path 参数", "path= 指向服务器上的固件文件"))
		return
	}
	img, err := firmware.InspectImage(path)
	if err != nil {
		writeJSON(w, 422, errBody(10301, "读取镜像失败", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"image": img})
}

// POST /api/v1/firmware/plan —— 只生成计划，不执行。
func (s *Server) handleFirmwarePlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path       string `json:"path"`
		LocalBoard string `json:"local_board"`
		HasBackup  bool   `json:"has_backup"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10302, "invalid body", err.Error()))
		return
	}
	img, err := firmware.InspectImage(body.Path)
	if err != nil {
		writeJSON(w, 422, errBody(10303, "读取镜像失败", err.Error()))
		return
	}
	if err := firmware.ValidateImage(img, body.LocalBoard); err != nil {
		writeJSON(w, 422, errBody(10304, "镜像不可用于本机", err.Error()))
		return
	}
	plan, err := firmware.BuildPlan(img, body.LocalBoard, body.HasBackup)
	if err != nil {
		writeJSON(w, 500, errBody(10305, "生成计划失败", err.Error()))
		return
	}
	writeJSON(w, 200, plan)
}

// POST /api/v1/firmware/apply —— 真正升级。必须 confirm=true。
func (s *Server) handleFirmwareApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm    bool   `json:"confirm"`
		Path       string `json:"path"`
		LocalBoard string `json:"local_board"`
		KeepConfig bool   `json:"keep_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10306, "invalid body", err.Error()))
		return
	}
	if !body.Confirm {
		writeJSON(w, 422, errBody(10307, "升级需要 confirm=true",
			"升级会重启设备并中断网络。如果通过 ssh 远程操作，升级后连接会断开。"+
				"请先用 /firmware/plan 检查计划，并确保已做配置备份。"))
		return
	}
	img, err := firmware.InspectImage(body.Path)
	if err != nil {
		writeJSON(w, 422, errBody(10308, "读取镜像失败", err.Error()))
		return
	}
	if err := firmware.ValidateImage(img, body.LocalBoard); err != nil {
		writeJSON(w, 422, errBody(10309, "镜像不可用于本机", err.Error()))
		return
	}
	ok, detail := firmware.Apply(execx.DefaultRunner{}, img, body.KeepConfig)
	writeJSON(w, 200, map[string]interface{}{
		"applied": ok, "detail": detail,
		"note": "sysupgrade 已提交，设备即将重启，网络会中断。",
	})
}

// ---------- 系统诊断报告 ----------

func (s *Server) handleDiagReportJSON(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	cmds := map[string][]string{}
	for _, sec := range diagreport.DefaultCommands() {
		// DefaultCommands 的顺序由 slice 保证，这里转成 map 只用于执行
		_ = sec
	}
	// 逐条执行；顺序与 DefaultCommands 一致
	var ordered []diagreport.Section
	for _, sec := range diagreport.DefaultCommands() {
		argv := diagReportArgv(sec.Name)
		if argv == nil {
			continue
		}
		cmds[sec.Name] = argv
		ordered = append(ordered, sec)
	}
	rep := diagreport.Collect(host, time.Now().Unix(), execx.DefaultRunner{}, cmds)
	writeJSON(w, 200, rep)
}

func (s *Server) handleDiagReportText(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	cmds := map[string][]string{}
	for _, sec := range diagreport.DefaultCommands() {
		if argv := diagReportArgv(sec.Name); argv != nil {
			cmds[sec.Name] = argv
		}
	}
	rep := diagreport.Collect(host, time.Now().Unix(), execx.DefaultRunner{}, cmds)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(diagreport.RenderText(rep)))
}

// diagReportArgv 把章节名映射到真实命令。
//
// 为什么不直接让 DefaultCommands 返回命令：那个函数签名是给前端列章节用的，
// 命令清单放在这里，避免把"要执行什么"泄露给调用方（防止有人篡改请求让它跑别的命令）。
func diagReportArgv(name string) []string {
	switch name {
	case "interfaces":
		return []string{"ip", "-br", "addr"}
	case "routes":
		return []string{"ip", "route"}
	case "neighbors":
		return []string{"ip", "neigh"}
	case "nft":
		return []string{"nft", "list", "ruleset"}
	case "disk":
		return []string{"df", "-h"}
	case "memory":
		return []string{"free", "-m"}
	case "loadavg":
		return []string{"cat", "/proc/loadavg"}
	case "failed_units":
		return []string{"systemctl", "--failed", "--no-pager", "--plain"}
	}
	return nil
}

// ---------- 邮件告警 / SNMP Trap ----------

func (s *Server) handleGetSMTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("notify", smtpStoreKey)
	var cfg notify.SMTPConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg.Masked()})
}

func (s *Server) handleSetSMTP(w http.ResponseWriter, r *http.Request) {
	var cfg notify.SMTPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 422, errBody(10310, "invalid body", err.Error()))
		return
	}
	if err := notify.ValidateSMTP(cfg); err != nil {
		writeJSON(w, 422, errBody(10311, "invalid smtp config", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	s.store.Set("notify", smtpStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"config": cfg.Masked()})
}

// POST /api/v1/notify/smtp/preview —— 只生成邮件内容，不发信。
func (s *Server) handleSMTPPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Subject string             `json:"subject"`
		Body    string             `json:"body"`
		Config  *notify.SMTPConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10312, "invalid body", err.Error()))
		return
	}
	cfg := notify.SMTPConfig{}
	if body.Config != nil {
		cfg = *body.Config
	} else {
		raw, _ := s.store.Get("notify", smtpStoreKey)
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	msg, err := notify.BuildMessage(cfg, body.Subject, body.Body)
	if err != nil {
		writeJSON(w, 422, errBody(10313, "生成邮件失败", err.Error()))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(msg))
}

func (s *Server) handleGetTrap(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("notify", trapStoreKey)
	var t notify.TrapTarget
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &t)
	}
	writeJSON(w, 200, map[string]interface{}{"target": t.Masked(), "sink": notify.RenderSink(t)})
}

func (s *Server) handleSetTrap(w http.ResponseWriter, r *http.Request) {
	var t notify.TrapTarget
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, 422, errBody(10314, "invalid body", err.Error()))
		return
	}
	if err := notify.ValidateTrap(t); err != nil {
		writeJSON(w, 422, errBody(10315, "invalid trap target", err.Error()))
		return
	}
	b, _ := json.Marshal(t)
	s.store.Set("notify", trapStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"target": t.Masked(), "sink": notify.RenderSink(t)})
}

// ---------- 策略路由 ----------

func (s *Server) handleGetPolicyTables(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("policyroute", prTablesKey)
	tables := []policyroute.Table{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &tables)
	}
	rendered := map[int][]string{}
	for _, t := range tables {
		rendered[t.ID] = policyroute.RenderTableRoutes(t)
	}
	writeJSON(w, 200, map[string]interface{}{"tables": tables, "count": len(tables), "rendered": rendered})
}

func (s *Server) handleSetPolicyTables(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tables  []policyroute.Table `json:"tables"`
		Confirm bool                `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10316, "invalid body", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空所有既有策略路由表并返回 200。
	// 误发空 body 就静默清空配置的风险太高，跟 timeschedule 对齐：
	// 必须显式 confirm:true 才允许清空。
	if len(body.Tables) == 0 && !body.Confirm {
		writeJSON(w, 422, errBody(10318, "tables must not be empty",
			"to clear all policy routing tables, send {\"tables\":[],\"confirm\":true}"))
		return
	}
	for i, t := range body.Tables {
		if err := policyroute.ValidateTable(t); err != nil {
			writeJSON(w, 422, errBody(10317, "invalid table", "tables["+itoa(i)+"] "+t.Name+": "+err.Error()))
			return
		}
	}
	b, _ := json.Marshal(body.Tables)
	s.store.Set("policyroute", prTablesKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"tables": body.Tables, "count": len(body.Tables)})
}

func (s *Server) handleGetPolicyRules(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("policyroute", prRulesKey)
	rules := []policyroute.Rule{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	rendered := make([]string, 0, len(rules))
	for _, rl := range rules {
		rendered = append(rendered, policyroute.RenderRule(rl))
	}
	writeJSON(w, 200, map[string]interface{}{"rules": rules, "count": len(rules), "rendered": rendered})
}

func (s *Server) handleSetPolicyRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rules   []policyroute.Rule `json:"rules"`
		Confirm bool               `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10318, "invalid body", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空所有策略路由规则并返回 200。
	// 必须显式 confirm:true 才允许清空（10318 已被 invalid body 占用，这里用 10320）。
	if len(body.Rules) == 0 && !body.Confirm {
		writeJSON(w, 422, errBody(10320, "rules must not be empty",
			"to clear all policy rules, send {rules:[],confirm:true}"))
		return
	}
	for i, rl := range body.Rules {
		if err := policyroute.ValidateRule(rl); err != nil {
			writeJSON(w, 422, errBody(10319, "invalid rule", "rules["+itoa(i)+"]: "+err.Error()))
			return
		}
	}
	b, _ := json.Marshal(body.Rules)
	s.store.Set("policyroute", prRulesKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"rules": body.Rules, "count": len(body.Rules)})
}


func (s *Server) handleGetPolicyRouteHits(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("policyroute", prRulesKey)
	rules := []policyroute.Rule{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	// Return each rule's hits keyed by priority string for simplicity
	out := []map[string]interface{}{}
	for _, rl := range rules {
		out = append(out, map[string]interface{}{"rule": rl, "hits": rl.Hits})
	}
	writeJSON(w, 200, map[string]interface{}{"hits": out, "count": len(out)})
}
