package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/ddnsup"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/vpnclient"
)

// ---------- WireGuard 客户端（本机连出去） ----------

// handleGetVPNClient 返回配置 + 真实运行状态。
// 私钥不下发：配置里存的私钥只用于渲染 wg0.conf，读接口一律不给。
func (s *Server) handleGetVPNClient(w http.ResponseWriter, r *http.Request) {
	cfg, hasCfg := s.loadVPNClient()
	status := []vpnclient.PeerStatus{}
	live := false
	iface := cfg.Interface
	if iface == "" {
		iface = "wg0"
	}
	out, err := (execx.DefaultRunner{}).Run("wg", "show", iface)
	if err == nil {
		live = true
		status = vpnclient.ParseWGShow(out)
	}
	// 每个 peer 算握手龄：看不到握手就是断的，别只看配置在不在
	now := time.Now().Unix()
	type peerView struct {
		vpnclient.PeerStatus
		Alive  bool   `json:"alive"`
		Reason string `json:"reason"`
	}
	views := make([]peerView, 0, len(status))
	for _, st := range status {
		alive, reason := vpnclient.HandshakeAge(now, st.LatestHandshake, 180)
		views = append(views, peerView{PeerStatus: st, Alive: alive, Reason: reason})
	}

	resp := map[string]interface{}{
		"has_config": hasCfg,
		"interface":  iface,
		"live":       live,
		"peers":      views,
		"peer_count": len(views),
	}
	if hasCfg {
		// 把私钥剥掉再返回
		safe := cfg
		safe.PrivateKey = ""
		for i := range safe.Peers {
			safe.Peers[i].Preshared = ""
		}
		resp["config"] = safe
	}
	if !live {
		resp["live_error"] = "wg show 不可用（wireguard-tools 未安装，或接口未启动）"
		resp["hint"] = "配置存在不代表隧道通；peers 为空说明接口没起来"
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleSetVPNClient(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config  vpnclient.Config `json:"config"`
		Confirm bool             `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(10420, "invalid JSON body", err.Error()))
		return
	}
	// 私钥为空表示"沿用已保存的"，不是错误
	if body.Config.PrivateKey == "" {
		if old, ok := s.loadVPNClient(); ok {
			body.Config.PrivateKey = old.PrivateKey
		}
	}
	if err := vpnclient.Validate(body.Config); err != nil {
		writeJSON(w, 422, errBody(10421, "invalid wireguard client config", err.Error()))
		return
	}
	conf := vpnclient.Render(body.Config)
	cmds := vpnclient.RenderCommands(body.Config)
	resp := map[string]interface{}{
		"conf":     conf,
		"commands": cmds,
		"applied":  false,
	}
	if !body.Confirm {
		resp["dry_run"] = true
		resp["hint"] = "re-send with confirm:true to write /etc/wireguard/<iface>.conf and bring it up"
		writeJSON(w, 200, resp)
		return
	}
	// 落盘 0600 后按 RenderCommands 逐条执行（每条是 []string，不拼 shell）
	path := "/etc/wireguard/" + body.Config.Interface + ".conf"
	if err := s.writeSecretFile(path, conf); err != nil {
		resp["error"] = "写入配置失败: " + err.Error()
		writeJSON(w, 200, resp)
		return
	}
	resp["config_path"] = path
	results := []map[string]interface{}{}
	allOK := true
	for _, argv := range cmds {
		if len(argv) == 0 {
			continue
		}
		out, err := (execx.DefaultRunner{}).Run(argv[0], argv[1:]...)
		item := map[string]interface{}{
			"cmd": strings.Join(argv, " "), "ok": err == nil,
			"output": strings.TrimSpace(out),
		}
		if err != nil {
			item["error"] = err.Error()
			allOK = false
		}
		results = append(results, item)
	}
	resp["commands_run"] = results
	resp["applied"] = allOK
	if allOK {
		if err := s.saveVPNClient(body.Config); err != nil {
			resp["save_error"] = err.Error()
		}
	}
	writeJSON(w, 200, resp)
}

func (s *Server) loadVPNClient() (vpnclient.Config, bool) {
	var c vpnclient.Config
	raw, err := s.store.Get("vpnclient", "config")
	if err != nil || raw == "" {
		return c, false
	}
	if json.Unmarshal([]byte(raw), &c) != nil {
		return c, false
	}
	return c, true
}

func (s *Server) saveVPNClient(c vpnclient.Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.store.Set("vpnclient", "config", string(b))
}

// ---------- DDNS ----------

func (s *Server) handleListDDNS(w http.ResponseWriter, r *http.Request) {
	records := s.loadDDNS()
	out := make([]ddnsup.Record, 0, len(records))
	for _, rec := range records {
		out = append(out, ddnsup.Masked(rec)) // 一律打码，原文不出后端
	}
	writeJSON(w, 200, map[string]interface{}{
		"count":     len(out),
		"records":   out,
		"providers": ddnsup.Providers(),
	})
}

func (s *Server) handleSetDDNS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Records []ddnsup.Record `json:"records"`
		Confirm bool            `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(10422, "invalid JSON body", err.Error()))
		return
	}
	// 空数组是全量替换语义：会清空所有 DDNS 记录并返回 200。
	// 必须显式 confirm:true 才允许清空。
	if len(body.Records) == 0 && !body.Confirm {
		writeJSON(w, 422, errBody(10425, "records must not be empty",
			"to clear all ddns records, send {records:[],confirm:true}"))
		return
	}
	// 打码值原样提交回来时不能覆盖真 token
	old := s.loadDDNS()
	byID := map[string]ddnsup.Record{}
	for _, r0 := range old {
		byID[r0.ID] = r0
	}
	for i := range body.Records {
		rec := &body.Records[i]
		if prev, ok := byID[rec.ID]; ok {
			// 前端拿到的是打码值；若提交的仍是打码值（或空），沿用旧的
			if rec.Token == "" || rec.Token == ddnsup.Masked(prev).Token {
				rec.Token = prev.Token
			}
			if rec.Password == "" || rec.Password == ddnsup.Masked(prev).Password {
				rec.Password = prev.Password
			}
		}
		if err := ddnsup.Validate(*rec); err != nil {
			writeJSON(w, 422, errBody(10423, "record "+strconv.Itoa(i)+" invalid", rec.Domain+": "+err.Error()))
			return
		}
	}
	if err := s.saveDDNS(body.Records); err != nil {
		writeJSON(w, 500, errBody(10424, "save failed", err.Error()))
		return
	}
	// 回读并打码
	out := make([]ddnsup.Record, 0, len(body.Records))
	for _, rec := range body.Records {
		out = append(out, ddnsup.Masked(rec))
	}
	writeJSON(w, 200, map[string]interface{}{"count": len(out), "records": out})
}

// handleDDNSCheck 对比"当前公网 IP"和"记录里存的 IP"，只回答是否需要更新。
// 真正提交给服务商需要各自的 API，这里不假装做到了 —— 只做本地判断。
func (s *Server) handleDDNSCheck(w http.ResponseWriter, r *http.Request) {
	current := strings.TrimSpace(r.URL.Query().Get("ip"))
	source := "query param"
	if current == "" {
		out, err := (execx.DefaultRunner{}).Run("curl", "-s", "-m", "5", "https://api.ipify.org")
		if err != nil {
			writeJSON(w, 200, map[string]interface{}{
				"available": false,
				"reason":    "无法获取公网 IP（curl 失败）",
				"detail":    err.Error(),
			})
			return
		}
		current = strings.TrimSpace(out)
		source = "api.ipify.org"
	}
	if current == "" {
		writeJSON(w, 200, map[string]interface{}{
			"available": false,
			"reason":    "获取到的公网 IP 是空值",
		})
		return
	}
	records := s.loadDDNS()
	type item struct {
		ID      string `json:"id"`
		Domain  string `json:"domain"`
		LastIP  string `json:"last_ip"`
		Changed bool   `json:"needs_update"`
	}
	items := make([]item, 0, len(records))
	var firstChanged *item
	for _, rec := range records {
		ch := ddnsup.NeedsUpdate(rec.LastIP, current)
		it := item{ID: rec.ID, Domain: rec.Subdomain + "." + rec.Domain, LastIP: rec.LastIP, Changed: ch}
		items = append(items, it)
		if ch && firstChanged == nil {
			firstChanged = &it
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"available":   true,
		"current_ip":  current,
		"source":      source,
		"records":     items,
		"need_update": firstChanged != nil,
		"note":        "这里只判断是否需要更新，不代为调用各服务商 API",
	})
}

func (s *Server) loadDDNS() []ddnsup.Record {
	raw, err := s.store.Get("ddns", "records")
	if err != nil || raw == "" {
		return []ddnsup.Record{}
	}
	var recs []ddnsup.Record
	if json.Unmarshal([]byte(raw), &recs) != nil {
		return []ddnsup.Record{}
	}
	return recs
}

func (s *Server) saveDDNS(records []ddnsup.Record) error {
	b, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return s.store.Set("ddns", "records", string(b))
}

// writeSecretFile 以 0600 写私钥文件。
//
// 不能直接 os.WriteFile(path, data, 0600)：那会在 umask 宽松时先以默认权限
// 创建再 chmod，中间有一个短暂窗口文件是可读的。这里用 O_CREATE|O_EXCL 试写，
// 已存在时先删再建，并显式 chmod 兜底。
func (s *Server) writeSecretFile(path, content string) error {
	dir := "/etc/wireguard"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// 已存在则先删除，避免旧权限或旧内容残留
	_ = os.Remove(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// 兜底：确认最终权限真的是 0600
	return os.Chmod(path, 0o600)
}

// handleDeleteDDNSRecord DELETE /api/v1/ddns/records/{id}
// 从存储删除指定 id 的 record，id 不存在返回 200 deleted=false。
func (s *Server) handleDeleteDDNSRecord(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	records := s.loadDDNS()
	found := false
	filtered := make([]ddnsup.Record, 0, len(records))
	for _, rec := range records {
		if rec.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, rec)
	}
	if found {
		if err := s.saveDDNS(filtered); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(10483, "save failed", err.Error()))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": found})
}
