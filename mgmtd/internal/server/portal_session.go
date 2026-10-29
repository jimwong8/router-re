package server

// 门户（captive portal）自助下线与在线状态。
//
// 这两个端点是 portal_flow_test 第 6 步要求的：用户点"下线"后 nft 放行集合
// 里要真的没有他，再查 status 必须显示不在线。
//
// 安全边界（重要）：logout **不允许**无差别踢掉所有在线设备。
// 只接受两种目标：
//   1. body 里显式给 mac；
//   2. 不给 mac 时，按请求来源 IP 匹配（用户只能下线自己那台机器）。
// 一个空 body 就把全网踢下线，是很容易写出来、也很容易被扫到的洞。

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/portal"
)

// handlePortalLogout 让用户（或管理员按 mac）下线。POST /api/v1/portal/logout
func (s *Server) handlePortalLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC string `json:"mac"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // 空 body 是合法的：按来源 IP 匹配
	}

	now := time.Now()
	authorized := s.portalMgr.Authorized(now)

	var targets []string
	if mac := portal.NormalizeMAC(strings.TrimSpace(req.MAC)); mac != "" {
		targets = append(targets, mac)
	} else {
		src := clientIP(r.RemoteAddr)
		for _, c := range authorized {
			if c.IP != "" && c.IP == src {
				targets = append(targets, c.MAC)
			}
		}
	}

	revoked := 0
	for _, mac := range targets {
		if s.portalMgr.Revoke(mac) {
			revoked++
		}
	}

	remaining := s.portalMgr.Authorized(now)
	if revoked > 0 {
		// 真正把 nft 放行集合收紧，否则"下线"只改了内存，流量照样通。
		_ = portal.Apply(execx.DefaultRunner{}, remaining, s.portalPort())
	}

	writeJSON(w, 200, map[string]interface{}{
		"revoked": revoked,
		"online":  len(remaining) > 0,
		"online_count": len(remaining),
	})
}

// handlePortalStatus 返回当前在线情况。GET /api/v1/portal/status
func (s *Server) handlePortalStatus(w http.ResponseWriter, r *http.Request) {
	authorized := s.portalMgr.Authorized(time.Now())
	writeJSON(w, 200, map[string]interface{}{
		"online":       len(authorized) > 0,
		"online_count": len(authorized),
		"clients":      authorized,
	})
}
