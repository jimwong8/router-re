package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/ddos"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/iperf"
	"github.com/jimwong/router-re/mgmtd/internal/nettools2"
	"github.com/jimwong/router-re/mgmtd/internal/procmon"
	"github.com/jimwong/router-re/mgmtd/internal/svcmgr"
	"github.com/jimwong/router-re/mgmtd/internal/timesched"
)

// ---------- 进程与服务 ----------

// handleListServices 列出 systemd 单元。
// 真机上没有 systemd（OpenWrt 用 procd）时，如实返回 available:false，
// 不要假装返回空列表 —— 空列表会被读成"没有服务在跑"。
func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	out, err := (execx.DefaultRunner{}).Run("systemctl",
		"list-units", "--all", "--no-pager", "--plain", "--no-legend")
	if err != nil {
		hasProcd := false
		if _, e2 := (execx.DefaultRunner{}).Run("ubus", "list"); e2 == nil {
			hasProcd = true
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"reason":    "systemd is not available on this system",
			"detail":    err.Error(),
			"procd":     hasProcd,
			"hint":      "OpenWrt uses procd; unit management is only supported on systemd hosts",
		})
		return
	}
	units := procmon.ParseUnits(out)

	// 分页（可选）：无 limit 参数时全量返回，保持向后兼容（e2e 无参调用）。
	total := len(units)
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	offset := 0
	if offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			offset = n
		}
	}
	hasMore := false
	if limitStr != "" {
		limit := 200
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
		if limit > 1000 {
			limit = 1000
		}
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		hasMore = offset+limit < total
		units = units[offset:end]
	}
	if units == nil {
		units = []procmon.Unit{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": true,
		"count":     len(units),
		"total":     total,
		"offset":    offset,
		// 与 /api/v1/logs 对齐：服务列表能拿到全量所以 total 是真值，
		// has_more 直接由"窗口有没有截断"得出，前端两处可以用同一套翻页逻辑。
		"has_more": hasMore,
		"units":    units,
	})
}

// handleServiceAction 启停/重启/重载一个单元。
//
// 安全边界（三道，缺一不可）：
//  1. 单元名过白名单字符集，挡命令注入
//  2. 动作过白名单，杜绝 mask/disable/kill
//  3. stop/restart 关键服务（sshd/networking/mgmtd）必须显式 confirm，
//     否则一条失误的 API 调用就能把自己锁在门外
func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Unit    string `json:"unit"`
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10380, "invalid JSON body", err.Error()))
		return
	}
	if err := procmon.ValidateUnitName(body.Unit); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10381, "invalid unit name", err.Error()))
		return
	}
	act := procmon.AllowedAction(body.Action)
	if err := procmon.ValidateAction(act); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10382, "invalid action", err.Error()))
		return
	}
	// 停或重启关键服务会让远程管理立刻失联
	if (act == procmon.ActionStop || act == procmon.ActionRestart) && procmon.IsProtected(body.Unit) && !body.Confirm {
		writeJSON(w, 422, errBody(10400, "refusing to stop/restart a protected unit without confirm",
			body.Unit+" carries remote management; stopping it locks you out of this router. "+
				"Re-send with confirm:true if that is really intended"))
		return
	}

	argv, err := procmon.RenderCmd(act, body.Unit)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10383, "cannot build command", err.Error()))
		return
	}
	out, err := (execx.DefaultRunner{}).Run("systemctl", argv...)
	resp := map[string]interface{}{
		"unit":    body.Unit,
		"action":  string(act),
		"argv":    argv,
		"output":  strings.TrimSpace(out),
		"applied": err == nil,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	// 执行后回读真实状态，而不是假设成功
	if st, e2 := (execx.DefaultRunner{}).Run("systemctl", "is-active", body.Unit); e2 == nil {
		resp["active_after"] = strings.TrimSpace(st)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListProcesses 返回按 CPU 排序的前 N 个进程。
func (s *Server) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	n := 15
	if v := r.URL.Query().Get("n"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p <= 200 {
			n = p
		}
	}
	out, err := (execx.DefaultRunner{}).Run("ps", "-eo", "pid,user,pcpu,pmem,rss,args", "--no-headers")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(10384, "ps failed", err.Error()))
		return
	}
	procs := procmon.ParsePS(out)
	top := procmon.TopN(procs, n)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total": len(procs),
		"n":     n,
		"top":   top,
	})
}

// handleKillProcess 杀掉一个进程。
// 只接受 PID，不接受进程名 —— 按名字杀会连带杀掉别人的同名进程。
func (s *Server) handleKillProcess(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PID     int  `json:"pid"`
		Signal  int  `json:"signal"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10385, "invalid JSON body", err.Error()))
		return
	}
	if err := procmon.ValidatePID(body.PID); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10386, "invalid pid", err.Error()))
		return
	}
	if !body.Confirm {
		writeJSON(w, 422, errBody(10401, "confirm required",
			"sending a signal to a process cannot be undone; re-send with confirm:true"))
		return
	}
	sig := body.Signal
	if sig == 0 {
		sig = 15 // SIGTERM 默认，先礼后兵
	}
	if sig != 15 && sig != 9 && sig != 1 && sig != 2 {
		writeJSON(w, 422, errBody(10402, "signal not allowed",
			"only 1(SIGHUP)/2(SIGINT)/9(SIGKILL)/15(SIGTERM) are accepted"))
		return
	}
	out, err := (execx.DefaultRunner{}).Run("kill", "-"+strconv.Itoa(sig), strconv.Itoa(body.PID))
	resp := map[string]interface{}{
		"pid": body.PID, "signal": sig, "applied": err == nil,
		"output": strings.TrimSpace(out),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- DDoS 防护 ----------

func (s *Server) handleGetDDoS(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadDDoS()
	src := "config store"
	if err != nil {
		cfg = ddos.Config{PortScanWindow: 10, BlockSeconds: 600}
		src = "defaults (no config saved yet)"
	}
	// nil slice 会序列化成 JSON 的 null，而不是 []。前端 Object.assign 之后
	// 拿到 whitelist:null，模板里 whitelist.length 直接抛 TypeError，
	// 整个 tab-pane 渲染失败（页面上只看到空白，控制台才有一行错）。
	// API 契约要求数组字段永远是数组。
	if cfg.Whitelist == nil {
		cfg.Whitelist = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"config": cfg, "source": src})
}

func (s *Server) handleSetDDoS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config  ddos.Config `json:"config"`
		Confirm bool        `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10387, "invalid JSON body", err.Error()))
		return
	}
	if err := ddos.Validate(body.Config); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10388, "invalid ddos config", err.Error()))
		return
	}
	rules := ddos.Render(body.Config)
	resp := map[string]interface{}{"nft": rules, "applied": false}
	if !body.Confirm {
		resp["dry_run"] = true
		resp["hint"] = "re-send with \"confirm\":true to apply"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	// nft -f - 从 stdin 读，避免临时文件竞争
	out, err := (execx.DefaultRunner{}).RunWithInput(rules, "nft", "-f", "-")
	resp["applied"] = err == nil
	resp["output"] = strings.TrimSpace(out)
	if err != nil {
		resp["error"] = err.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if err := s.saveDDoS(body.Config); err != nil {
		resp["save_error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) loadDDoS() (ddos.Config, error) {
	var c ddos.Config
	raw, err := s.store.Get("ddos", "config")
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(raw), &c)
	return c, err
}

func (s *Server) saveDDoS(c ddos.Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.store.Set("ddos", "config", string(b))
}

// ---------- 时间段策略 ----------

func (s *Server) handleGetTimeSchedule(w http.ResponseWriter, r *http.Request) {
	rules := s.loadTimeRules()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().Unix()
	type view struct {
		timesched.Rule
		Active bool `json:"active_now"`
	}
	out := make([]view, 0, len(rules))
	for _, ru := range rules {
		act, _ := timesched.Active(ru, now, loc)
		out = append(out, view{Rule: ru, Active: act})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(out),
		"timezone": "Asia/Shanghai",
		"now":      time.Unix(now, 0).In(loc).Format("2006-01-02 15:04:05 Mon"),
		"rules":    out,
	})
}

func (s *Server) handleSetTimeSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rules   []timesched.Rule `json:"rules"`
		Confirm bool             `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10389, "invalid JSON body", err.Error()))
		return
	}
	if len(body.Rules) == 0 {
		writeJSON(w, 422, errBody(10403, "rules must not be empty",
			"to disable all time-based policies, send one rule with enabled:false instead of an empty list"))
		return
	}
	for i, ru := range body.Rules {
		if err := timesched.Validate(ru); err != nil {
			writeJSON(w, 422, errBody(10404, "rule "+strconv.Itoa(i)+" invalid", ru.Name+": "+err.Error()))
			return
		}
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	nft := timesched.Render(body.Rules, time.Now().Unix(), loc)
	resp := map[string]interface{}{"nft": nft, "applied": false, "count": len(body.Rules)}
	if !body.Confirm {
		resp["dry_run"] = true
		writeJSON(w, http.StatusOK, resp)
		return
	}
	// p95: 走 s.aclApplier（生产即 execx.DefaultRunner{}，行为不变；测试可注入
	// fake runner，见 rmw_concurrency_test.go）。
	// nft 下发与保存包进 timeschedMu：delete 的「读→过滤→写」也持同一把锁，
	// 否则 delete 在下发前读到的旧表会在保存后写回，把刚下发的规则抹掉。
	s.timeschedMu.Lock()
	runner := s.aclApplier
	if runner == nil {
		runner = execx.DefaultRunner{}
	}
	out, err := runner.RunWithInput(nft, "nft", "-f", "-")
	resp["applied"] = err == nil
	resp["output"] = strings.TrimSpace(out)
	if err != nil {
		resp["error"] = err.Error()
	} else if err := s.saveTimeRules(body.Rules); err != nil {
		resp["save_error"] = err.Error()
	}
	s.timeschedMu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteTimeSchedule deletes a time-schedule rule by id.
func (s *Server) handleDeleteTimeSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// p95: 「读整份→改一条→写整份」须与并发 delete / set 串行化：不加锁时两个并发
	// 删除读同一份快照，后写回者盖掉先写回者的删除（并发用例实测 30/32 丢失）。
	s.timeschedMu.Lock()
	rules := s.loadTimeRules()
	filtered := make([]timesched.Rule, 0, len(rules))
	removed := false
	for _, ru := range rules {
		if ru.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, ru)
	}
	var saveErr error
	if removed {
		saveErr = s.saveTimeRules(filtered)
	}
	s.timeschedMu.Unlock()
	if !removed {
		writeJSON(w, 200, map[string]interface{}{"deleted": false, "rules": rules})
		return
	}
	if saveErr != nil {
		writeJSON(w, 500, errBody(10405, "save failed", saveErr.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": true, "rules": filtered})
}

func (s *Server) loadTimeRules() []timesched.Rule {
	raw, err := s.store.Get("timesched", "rules")
	if err != nil {
		return []timesched.Rule{}
	}
	var rules []timesched.Rule
	if json.Unmarshal([]byte(raw), &rules) != nil {
		return []timesched.Rule{}
	}
	return rules
}

func (s *Server) saveTimeRules(rules []timesched.Rule) error {
	b, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	return s.store.Set("timesched", "rules", string(b))
}

// ---------- 网络工具 ----------

// runNetTool 是 ping/traceroute/portprobe 的共同骨架。
// 参数一律以 []string 传入 execx（绝不拼 shell 字符串），
// 目标地址先过 ValidateTarget 挡参数注入。
func (s *Server) runNetTool(w http.ResponseWriter, tool, bin string, args []string, timeoutMsg string) {
	start := time.Now()
	out, err := (execx.DefaultRunner{}).Run(bin, args...)
	lines := []string{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	res := nettools2.Result{
		OK:      err == nil,
		Command: bin + " " + strings.Join(args, " "),
		Lines:   lines,
		Summary: nettools2.Summarize(tool, lines),
	}
	if err != nil {
		res.Error = err.Error()
		// 命令不存在和"目标不可达"是两件事，必须分开说
		if strings.Contains(err.Error(), "executable file not found") {
			res.Summary = bin + " 未安装（不是网络不通）"
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"result":     res,
		"elapsed_ms": time.Since(start).Milliseconds(),
		"note":       timeoutMsg,
	})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target string `json:"target"`
		Count  int    `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10390, "invalid JSON body", err.Error()))
		return
	}
	if err := nettools2.ValidateTarget(body.Target); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10391, "invalid target", err.Error()))
		return
	}
	if body.Count == 0 {
		body.Count = 4
	}
	argv, err := nettools2.PingArgs(body.Target, body.Count)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10392, "invalid count", err.Error()))
		return
	}
	s.runNetTool(w, "ping", "ping", argv, "ICMP 被防火墙丢弃时 ping 不通不代表主机不在线")
}

func (s *Server) handleTraceroute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target  string `json:"target"`
		MaxHops int    `json:"max_hops"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10393, "invalid JSON body", err.Error()))
		return
	}
	if err := nettools2.ValidateTarget(body.Target); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10394, "invalid target", err.Error()))
		return
	}
	if body.MaxHops == 0 {
		body.MaxHops = 15
	}
	argv, err := nettools2.TracerouteArgs(body.Target, body.MaxHops)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10395, "invalid max_hops", err.Error()))
		return
	}
	s.runNetTool(w, "traceroute", "traceroute", argv, "中间跳路由器可能不回 ICMP，出现 * 不代表路径断开")
}

func (s *Server) handlePortProbe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target  string `json:"target"`
		Port    int    `json:"port"`
		Timeout int    `json:"timeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10396, "invalid JSON body", err.Error()))
		return
	}
	if err := nettools2.ValidateTarget(body.Target); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10397, "invalid target", err.Error()))
		return
	}
	if err := nettools2.ValidatePort(body.Port); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10398, "invalid port", err.Error()))
		return
	}
	if body.Timeout == 0 {
		body.Timeout = 3
	}
	argv, err := nettools2.PortProbeArgs(body.Target, body.Port, body.Timeout)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10399, "invalid timeout", err.Error()))
		return
	}
	// nc 的 exit code 语义：0=开放，1=关闭；这比看输出文本可靠
	start := time.Now()
	// PortProbeArgs 返回的是参数，命令名单独给 —— 之前误用 argv[0] 当命令名，
	// 导致 "nc -z -w 2 host port" 被当成 "-z ..." 执行，永远报"未安装"。
	out, runErr := (execx.DefaultRunner{}).Run("nc", argv...)
	state := "unknown"
	if runErr == nil {
		state = "open"
	} else if strings.Contains(runErr.Error(), "executable file not found") {
		state = "tool_missing"
	} else {
		state = "closed_or_filtered"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"target":     body.Target,
		"port":       body.Port,
		"state":      state,
		"command":    "nc " + strings.Join(argv, " "),
		"output":     strings.TrimSpace(out),
		"elapsed_ms": time.Since(start).Milliseconds(),
		"note":       "closed_or_filtered 表示端口没有响应，无法区分是被防火墙 DROP 还是服务未启动",
	})
}

// handleServicesStatus returns the enabled/running status of ssh/telnet/ftp/dnsmasq.
//
// GET /api/v1/services/status
func (s *Server) handleServicesStatus(w http.ResponseWriter, r *http.Request) {
	mgr := s.svcMgr
	if mgr == nil {
		mgr = svcmgr.New(execx.DefaultRunner{})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"services": mgr.StatusAll(),
	})
}

// handleServiceSwitch toggles a service's enabled state.
//
// POST /api/v1/services/{name}  body {"enabled":bool}
func (s *Server) handleServiceSwitch(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !svcmgrValid(name) {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10505, "unknown service",
			"service must be one of: ssh, telnet, ftp, dnsmasq"))
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10506, "invalid JSON body", err.Error()))
		return
	}
	mgr := s.svcMgr
	if mgr == nil {
		mgr = svcmgr.New(execx.DefaultRunner{})
	}
	if err := mgr.SetEnabled(name, body.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(10507, "service switch failed", err.Error()))
		return
	}
	st, _ := mgr.Status(name)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":    name,
		"enabled": body.Enabled,
		"status":  st,
	})
}

// svcmgrValid mirrors svcmgr.validService without exporting it.
func svcmgrValid(name string) bool {
	for _, s := range svcmgr.Services {
		if s == name {
			return true
		}
	}
	return false
}

// handleIperfRun runs an iperf3 client test.
//
// POST /api/v1/tools/iperf/run  body {"host","port","duration","protocol"}
func (s *Server) handleIperfRun(w http.ResponseWriter, r *http.Request) {
	var cfg iperf.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10508, "invalid JSON body", err.Error()))
		return
	}
	if err := iperf.Validate(&cfg); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10509, "invalid iperf config", err.Error()))
		return
	}
	runner := s.iperfRunner
	if runner == nil {
		runner = execx.DefaultRunner{}
	}
	res := iperf.Run(runner, cfg)
	s.iperfMu.Lock()
	s.iperfLast = res
	s.iperfMu.Unlock()
	if !res.OK {
		writeJSON(w, http.StatusInternalServerError, errBody(10510, "iperf run failed", res.Error))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleIperfStatus returns the most recent iperf result.
//
// GET /api/v1/tools/iperf/status
func (s *Server) handleIperfStatus(w http.ResponseWriter, r *http.Request) {
	s.iperfMu.Lock()
	res := s.iperfLast
	s.iperfMu.Unlock()
	writeJSON(w, http.StatusOK, res)
}

// handleTaskSubmit submits a new background task.
//
// POST /api/v1/tasks  body {"type":..., "params":{...}}
func (s *Server) handleTaskSubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(10513, "invalid JSON body", err.Error()))
		return
	}
	if body.Type == "" {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10514, "type required", "task type is required"))
		return
	}
	if body.Params == nil {
		body.Params = json.RawMessage("{}")
	}
	task, err := s.taskQ.Submit(body.Type, body.Params)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errBody(10515, "submit failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

// handleTaskList lists tasks with optional state/type filters and pagination.
//
// GET /api/v1/tasks?state=...&type=...&offset=...&limit=...
func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	typ := q.Get("type")
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	tasks, total := s.taskQ.List(state, typ, offset, limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tasks": tasks,
		"total": total,
	})
}

// handleTaskGet returns a single task.
//
// GET /api/v1/tasks/{id}
func (s *Server) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, ok := s.taskQ.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody(10516, "task not found", "no task with id "+id))
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// handleTaskCancel cancels a pending/running task.
//
// POST /api/v1/tasks/{id}/cancel
func (s *Server) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.taskQ.Cancel(id); err != nil {
		writeJSON(w, http.StatusConflict, errBody(10517, "cancel failed", err.Error()))
		return
	}
	task, _ := s.taskQ.Get(id)
	writeJSON(w, http.StatusOK, task)
}

// netSplitHostPort 仅供测试用，拆分 host:port
func netSplitHostPort(s string) (string, string, error) { return net.SplitHostPort(s) }
