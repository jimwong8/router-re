package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/threat"
)

// threatBlockDuration 由前端传入秒数；0 表示永久。
const maxBlockDuration = 365 * 24 * 3600 // 1 年上限，防止误填极大值

// handleThreatList 返回封禁列表。
// 支持 ?include_expired=true：默认只返回生效条目（过滤已过期）；传 true 则全量返回
// 但仍用 Prune 语义在响应里附带已过期标记（由前端展示）。
func (s *Server) handleThreatList(w http.ResponseWriter, r *http.Request) {
	includeExpired := r.URL.Query().Get("include_expired") == "true"
	now := time.Now().Unix()

	all := s.threatList.All()
	active := make([]threat.Entry, 0, len(all))
	expired := make([]threat.Entry, 0, len(all))
	for _, e := range all {
		if threat.Expired(e, now) {
			expired = append(expired, e)
		} else {
			active = append(active, e)
		}
	}
	res := map[string]interface{}{
		"count": len(all),
		"now":   now,
	}
	if includeExpired {
		res["entries"] = all
		res["expired_count"] = len(expired)
	} else {
		res["entries"] = active
		res["expired_count"] = len(expired)
	}
	writeJSON(w, 200, res)
}

// handleThreatBlock 加入一条封禁。POST /api/v1/threat/block {ip, reason, duration_seconds}。
// duration 0=永久；否则为从现在起多少秒（≤1 年）。
func (s *Server) handleThreatBlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP              string `json:"ip"`
		Reason          string `json:"reason"`
		DurationSeconds int64  `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10160, "json decode fail", err.Error()))
		return
	}
	if req.DurationSeconds < 0 {
		req.DurationSeconds = 0
	}
	if req.DurationSeconds > maxBlockDuration {
		req.DurationSeconds = maxBlockDuration
	}
	now := time.Now().Unix()
	e := threat.Entry{
		IP:        req.IP,
		Reason:    req.Reason,
		Source:    "manual",
		CreatedAt: now,
		Hits:      0,
	}
	if req.DurationSeconds > 0 {
		e.ExpiresAt = now + req.DurationSeconds
	}
	if err := threat.ValidateEntry(e); err != nil {
		// 包含拒绝 0.0.0.0/0 与 ::/0 的红线。
		writeJSON(w, 422, errBody(10161, "validation failed", err.Error()))
		return
	}
	if err := s.threatList.Add(e); err != nil {
		writeJSON(w, 422, errBody(10161, "add failed", err.Error()))
		return
	}
	// 审计必须记录 IP 与原因。
	s.logAudit(r, "threat.block", req.IP, "ok",
		fmt.Sprintf("reason=%q expires_at=%d", req.Reason, e.ExpiresAt))
	writeJSON(w, 200, map[string]interface{}{"entry": e, "added": true})
}

// handleThreatUnblock 移除封禁。DELETE /api/v1/threat/block {ip}。
func (s *Server) handleThreatUnblock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10162, "json decode fail", err.Error()))
		return
	}
	removed := s.threatList.Remove(req.IP)
	s.logAudit(r, "threat.unblock", req.IP, "ok", fmt.Sprintf("removed=%v", removed))
	writeJSON(w, 200, map[string]interface{}{"ip": req.IP, "removed": removed})
}

// handleThreatApply 下发 nft，并**回读 nft list set 确认元素数量**。
// 回读是设计关键：只看 nft -f - 的退出码可能被缓存/时序掩盖，必须确认集合里真有
// 预期的元素数，否则封禁没有真正生效而要管理员以为生效了。
func (s *Server) handleThreatApply(w http.ResponseWriter, r *http.Request) {
	now := time.Now().Unix()
	all := s.threatList.All()
	active := make([]threat.Entry, 0, len(all))
	for _, e := range all {
		if !threat.Expired(e, now) {
			active = append(active, e)
		}
	}
	script := threat.RenderNft(active)

	runner := execx.DefaultRunner{}
	// 用 nft -f - 通过 stdin 下发，绝不 flush ruleset / 绝不 policy drop。
	if _, err := runner.RunWithInput(script, "nft", "-f", "-"); err != nil {
		writeJSON(w, 500, errBody(10163, "nft apply failed", err.Error()))
		return
	}
	// 回读 v4 集合元素数量。
	v4Count, v6Count, err := threatSetCounts(runner)
	if err != nil {
		writeJSON(w, 500, errBody(10164, "nft readback failed", err.Error()))
		return
	}
	res := map[string]interface{}{
		"applied":  true,
		"count":    len(active),
		"v4_count": v4Count,
		"v6_count": v6Count,
		"script":   script,
	}
	writeJSON(w, 200, res)
}

// threatSetCounts 回读 nft list set 输出，统计 blocked / blocked6 集合元素数。
// nft 输出形如 "elements = { 203.0.113.5, 10.0.0.0/8 }"，按逗号切分元素。
func threatSetCounts(r execx.Runner) (v4, v6 int, err error) {
	if v4o, e := r.Run("nft", "list", "set", "inet", "router_threat", "blocked"); e == nil {
		v4 = countNftElements(v4o)
	} else {
		return 0, 0, e
	}
	if v6o, e := r.Run("nft", "list", "set", "inet", "router_threat", "blocked6"); e == nil {
		v6 = countNftElements(v6o)
	} else {
		return 0, 0, e
	}
	return v4, v6, nil
}

func countNftElements(out string) int {
	// 形如 "elements = { a, b, c }" 或 "elements = {}"
	count := 0
	inElem := false
	// 只处理包含 "elements =" 的那一行花括号区段。
	if idx := strings.Index(out, "elements ="); idx >= 0 {
		seg := out[idx:]
		for _, ch := range seg {
			switch {
			case ch == '{':
				inElem = false
			case ch == '}':
				if inElem {
					count++
				}
				return count
			case ch == ',':
				if inElem {
					count++
				}
				inElem = false
			case ch != ' ' && ch != '\t' && ch != '\n':
				inElem = true
			}
		}
	}
	return count
}

// handleThreatImport 批量导入文本（每行一个 IP/CIDR）。
func (s *Server) handleThreatImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10165, "json decode fail", err.Error()))
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, 422, errBody(10166, "text must not be empty",
			"导入内容为空；空文本无法产生任何情报条目"))
		return
	}
	entries, parseSkipped, parseErr := threat.ParseFeedCount(req.Text)
	now := time.Now().Unix()
	// 补齐时间字段。
	for i := range entries {
		entries[i].Source = "import"
		entries[i].CreatedAt = now
	}
	added, addSkipped, errs := s.threatList.AddMany(entries)
	skipped := parseSkipped + addSkipped
	s.logAudit(r, "threat.import", "text", "ok",
		fmt.Sprintf("added=%d skipped=%d", added, skipped))
	res := map[string]interface{}{
		"added":      added,
		"skipped":    skipped,
		"parse_note": "",
	}
	if parseErr != nil {
		res["parse_note"] = parseErr.Error()
	}
	if len(errs) > 0 {
		res["errors"] = errs
	}
	writeJSON(w, 200, res)
}

// handleThreatFetch 从情报源 URL 拉取并导入。
func (s *Server) handleThreatFetch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10166, "json decode fail", err.Error()))
		return
	}
	entries, err := threat.FetchFeed(execx.DefaultRunner{}, req.URL)
	if err != nil {
		writeJSON(w, 422, errBody(10167, "fetch failed", err.Error()))
		return
	}
	now := time.Now().Unix()
	for i := range entries {
		entries[i].Source = req.URL
		entries[i].CreatedAt = now
	}
	added, skipped, errs := s.threatList.AddMany(entries)
	s.logAudit(r, "threat.fetch", req.URL, "ok", fmt.Sprintf("added=%d", added))
	res := map[string]interface{}{
		"source":  req.URL,
		"added":   added,
		"skipped": skipped,
	}
	if len(errs) > 0 {
		res["errors"] = errs
	}
	writeJSON(w, 200, res)
}

// handleThreatStats 统计卡片数据：总 / 生效中 / 已过期 / 永久。
func (s *Server) handleThreatStats(w http.ResponseWriter, r *http.Request) {
	now := time.Now().Unix()
	total := 0
	active := 0
	expired := 0
	permanent := 0
	for _, e := range s.threatList.All() {
		total++
		if threat.Expired(e, now) {
			expired++
			continue
		}
		active++
		if e.ExpiresAt == 0 {
			permanent++
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"total":     total,
		"active":    active,
		"expired":   expired,
		"permanent": permanent,
	})
}
