package server

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/alerting"
	"github.com/jimwong/router-re/mgmtd/internal/billing"
	"github.com/jimwong/router-re/mgmtd/internal/cfgsync"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/logship"
)

const (
	alertRulesKey  = "alerts/rules"
	rotateStoreKey = "logs/rotate"
	syslogStoreKey = "logs/syslog"
	billsStoreKey  = "billing/bills"
)

var alertEngine = alerting.NewEngine()

// ---------- 告警规则 ----------

func (s *Server) handleGetAlertRules(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("alerts", alertRulesKey)
	rules := []alerting.Rule{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &rules)
	}
	writeJSON(w, 200, map[string]interface{}{"rules": rules, "count": len(rules)})
}

func (s *Server) handleSetAlertRules(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rules   []alerting.Rule `json:"rules"`
		Confirm bool            `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10210, "invalid body", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空所有既有告警规则并返回 200。
	// 误发空 body 就静默清空配置的风险太高，跟 timeschedule 对齐：
	// 必须显式 confirm:true 才允许清空。
	if len(body.Rules) == 0 && !body.Confirm {
		writeJSON(w, 422, errBody(10212, "rules must not be empty",
			"to clear all alert rules, send {\"rules\":[],\"confirm\":true}"))
		return
	}
	for i, rl := range body.Rules {
		if err := alerting.Validate(rl); err != nil {
			writeJSON(w, 422, errBody(10211, "invalid rule",
				"rule["+strconv.Itoa(i)+"] "+rl.Name+": "+err.Error()))
			return
		}
	}
	b, _ := json.Marshal(body.Rules)
	s.store.Set("alerts", alertRulesKey, string(b))
	alertEngine.Reset()
	writeJSON(w, 200, map[string]interface{}{"rules": body.Rules, "count": len(body.Rules)})
}

// POST /api/v1/alerts/test —— 用给定消息试跑规则，看会不会告警（调试用）。
func (s *Server) handleTestAlerts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Unit    string `json:"unit"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10212, "invalid body", err.Error()))
		return
	}
	raw, _ := s.store.Get("alerts", alertRulesKey)
	var rules []alerting.Rule
	_ = json.Unmarshal([]byte(raw), &rules)

	matched := []map[string]string{}
	for _, rl := range rules {
		if alerting.Match(rl, body.Unit, body.Message) {
			matched = append(matched, map[string]string{
				"id": rl.ID, "name": rl.Name, "severity": rl.Severity,
			})
		}
	}
	writeJSON(w, 200, map[string]interface{}{"matched": matched, "count": len(matched)})
}

// ---------- 日志轮转与 syslog ----------

func (s *Server) handleGetRotate(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("logs", rotateStoreKey)
	var cfg logship.RotateConfig
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "logrotate": logship.RenderLogrotate(cfg)})
}

func (s *Server) handleSetRotate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool                 `json:"confirm"`
		Config  logship.RotateConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10213, "invalid body", err.Error()))
		return
	}
	if err := logship.ValidateRotate(body.Config); err != nil {
		writeJSON(w, 422, errBody(10214, "invalid rotate config", err.Error()))
		return
	}
	conf := logship.RenderLogrotate(body.Config)
	if !body.Confirm {
		writeJSON(w, 200, map[string]interface{}{
			"applied": false, "dry_run": true, "logrotate": conf,
			"detail": "dry-run：未写入。写入会执行一次强制轮转验证，确认请带 confirm=true。",
		})
		return
	}
	path := "/etc/logrotate.d/router-re-generated"
	b, _ := json.Marshal(body.Config)
	s.store.Set("logs", rotateStoreKey, string(b))
	if err := logship.ApplyRotate(execx.DefaultRunner{}, body.Config, path); err != nil {
		writeJSON(w, 500, errBody(10215, "apply rotate failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"applied": true, "path": path, "logrotate": conf})
}

func (s *Server) handleGetSyslog(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("logs", syslogStoreKey)
	var t logship.SyslogTarget
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &t)
	}
	writeJSON(w, 200, map[string]interface{}{"target": t, "rsyslog": logship.RenderRsyslog(t)})
}

func (s *Server) handleSetSyslog(w http.ResponseWriter, r *http.Request) {
	var t logship.SyslogTarget
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, 422, errBody(10216, "invalid body", err.Error()))
		return
	}
	if err := logship.ValidateTarget(t); err != nil {
		writeJSON(w, 422, errBody(10217, "invalid syslog target", err.Error()))
		return
	}
	b, _ := json.Marshal(t)
	s.store.Set("logs", syslogStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{"target": t, "rsyslog": logship.RenderRsyslog(t)})
}

// POST /api/v1/logs/syslog/test —— 真实尝试发一条测试日志。
func (s *Server) handleTestSyslog(w http.ResponseWriter, r *http.Request) {
	var t logship.SyslogTarget
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, 422, errBody(10218, "invalid body", err.Error()))
		return
	}
	if err := logship.ValidateTarget(t); err != nil {
		writeJSON(w, 422, errBody(10219, "invalid syslog target", err.Error()))
		return
	}
	ok, detail := logship.TestTarget(execx.DefaultRunner{}, t)
	writeJSON(w, 200, map[string]interface{}{"ok": ok, "detail": detail})
}

// ---------- 账单 ----------

func (s *Server) handleGetBills(w http.ResponseWriter, r *http.Request) {
	raw, _ := s.store.Get("billing", billsStoreKey)
	bills := []billing.Bill{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &bills)
	}
	sort.Slice(bills, func(i, j int) bool { return bills[i].CreatedAt > bills[j].CreatedAt })
	writeJSON(w, 200, map[string]interface{}{"bills": bills, "count": len(bills)})
}

// POST /api/v1/billing/bills/generate —— 按套餐和用量给账号出账。
func (s *Server) handleGenerateBills(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PeriodDays int `json:"period_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.PeriodDays = 30
	}
	if body.PeriodDays <= 0 {
		body.PeriodDays = 30
	}
	now := time.Now().Unix()
	plans := s.billingPlans
	planByID := map[string]billing.Plan{}
	for _, p := range plans {
		planByID[p.ID] = billing.Plan{
			ID: p.ID, Name: p.Name, PriceYuan: p.PriceYuan,
			PeriodDays: body.PeriodDays, QuotaMB: p.QuotaMB,
		}
	}

	accts := s.accountMgr.List()
	generated := []billing.Bill{}
	var skipped []string
	for _, a := range accts {
		// Account 里表示套餐的是 Profile 字段
		p, ok := planByID[a.Profile]
		if !ok {
			// 套餐不存在就没法算钱 —— 如实报告而不是给个 0 元账单
			skipped = append(skipped, a.Username+"(套餐 "+a.Profile+" 不存在)")
			continue
		}
		used := a.UsedMB
		b := billing.NewBill(
			"bill-"+a.Username+"-"+strconv.FormatInt(now, 10),
			a.Username, p, used, now-int64(body.PeriodDays)*86400, now)
		if err := billing.ValidateBill(b); err != nil {
			skipped = append(skipped, a.Username+"("+err.Error()+")")
			continue
		}
		generated = append(generated, b)
	}

	b, _ := json.Marshal(generated)
	s.store.Set("billing", billsStoreKey, string(b))
	writeJSON(w, 200, map[string]interface{}{
		"generated": generated, "count": len(generated), "skipped": skipped,
	})
}

// ---------- 配置导出 / 差异 / 恢复 ----------

// sectionKeys 是参与导出的 store 命名空间（顺序无关，checksum 内部会排序）。
var sectionKeys = []string{
	"interfaces", "vlan", "routes", "firewall", "nat", "dhcp", "dns", "ddns",
	"ntp", "qos", "acl", "vpn", "vlan", "ipv6", "snmp", "upnp", "ips",
	"l7filter", "nac", "threat", "quota", "htb", "lb", "mirror", "alerts", "logs",
}

// storeExporter 是可选能力：FileStore 能一次导出全部配置项。
// 不能逐个 (section, option) 去 Get —— 那要求事先知道所有 option 名，
// 而 option 名分散在各模块里（"config"/"rules"/"proxy"/"access"...），
// 逐个猜必然漏（实测导出结果 sections=0）。
type storeExporter interface {
	Export() map[string]string
}

// snapshotSections 取当前全部配置。key 是 "section.option"。
func (s *Server) snapshotSections() map[string]json.RawMessage {
	sections := map[string]json.RawMessage{}
	ex, ok := s.store.(storeExporter)
	if !ok {
		return sections
	}
	for k, v := range ex.Export() {
		if v == "" {
			continue
		}
		if json.Valid([]byte(v)) {
			sections[k] = json.RawMessage(v)
		}
	}
	return sections
}

func (s *Server) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	sections := s.snapshotSections()
	host, _ := os.Hostname()
	b := cfgsync.NewBundle(host, time.Now().Unix(), sections)
	writeJSON(w, 200, b)
}

// POST /api/v1/config/preview —— 传入 bundle，返回与当前配置的差异 + 危险项。
func (s *Server) handlePreviewRestore(w http.ResponseWriter, r *http.Request) {
	var incoming cfgsync.Bundle
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeJSON(w, 422, errBody(10220, "invalid bundle", err.Error()))
		return
	}
	if err := cfgsync.ValidateBundle(incoming); err != nil {
		writeJSON(w, 422, errBody(10221, "bundle 校验失败", err.Error()))
		return
	}
	host, _ := os.Hostname()
	current := cfgsync.NewBundle(host, time.Now().Unix(), s.snapshotSections())

	plan, err := cfgsync.BuildRestorePlan(current, incoming)
	if err != nil {
		writeJSON(w, 500, errBody(10222, "build plan failed", err.Error()))
		return
	}
	writeJSON(w, 200, plan)
}

// POST /api/v1/config/restore —— 真正恢复。必须 confirm=true。
func (s *Server) handleRestoreConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool           `json:"confirm"`
		Bundle  cfgsync.Bundle `json:"bundle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10223, "invalid body", err.Error()))
		return
	}
	if err := cfgsync.ValidateBundle(body.Bundle); err != nil {
		writeJSON(w, 422, errBody(10224, "bundle 校验失败", err.Error()))
		return
	}
	if !body.Confirm {
		writeJSON(w, 422, errBody(10225, "restore requires confirm=true",
			"恢复会覆盖当前配置，且部分 section（网络/防火墙）改错会导致失联。先用 /config/preview 看差异。"))
		return
	}
	restored := []string{}
	for ns, raw := range body.Bundle.Sections {
		if !json.Valid(raw) {
			continue
		}
		// ns 形如 "section.option"（Export 的 key 格式），拆回 (section, option)。
		section, option := ns, ""
		if i := strings.IndexByte(ns, '.'); i >= 0 {
			section = ns[:i]
			option = ns[i+1:]
		}
		if option == "" {
			option = "config"
		}
		if err := s.store.Set(section, option, string(raw)); err != nil {
			continue
		}
		restored = append(restored, ns)
	}
	sort.Strings(restored)
	writeJSON(w, 200, map[string]interface{}{
		"restored": restored, "count": len(restored),
		"detail": "已写回 store。注意：部分 section 需要重启对应服务或重新下发才会真正生效。",
	})
}

// 保证 strings 被用到（避免 import 漂移）
var _ = strings.TrimSpace
