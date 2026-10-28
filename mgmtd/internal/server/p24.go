package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/sysctl"
)

// ---------- conntrack 字节统计开关 ----------

// handleGetConntrackAcct 返回当前 nf_conntrack_acct 状态及持久化情况。
// GET /api/v1/conntrack/acct
func (s *Server) handleGetConntrackAcct(w http.ResponseWriter, r *http.Request) {
	on, err := sysctl.IsAcctOn()
	if err != nil {
		// proc 路径不可读（模块未加载等），如实返回 off + 原因，不报错
		writeJSON(w, 200, map[string]interface{}{
			"on":        false,
			"path":      sysctl.AcctPath(),
			"persisted": false,
			"reason":    "无法读取 nf_conntrack_acct：" + err.Error(),
		})
		return
	}

	persisted := false
	if b, err := os.ReadFile("/etc/sysctl.d/90-router-re-conntrack.conf"); err == nil {
		persisted = strings.Contains(string(b), "=1")
	}

	writeJSON(w, 200, map[string]interface{}{
		"on":        on,
		"path":      sysctl.AcctPath(),
		"persisted": persisted,
	})
}

// handleSetConntrackAcct 一键开启/关闭 conntrack 字节统计。
// POST /api/v1/conntrack/acct  body: {"on":bool, "persist":bool}
func (s *Server) handleSetConntrackAcct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On      bool `json:"on"`
		Persist bool `json:"persist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10480, "invalid body", err.Error()))
		return
	}

	// 即时生效：写 /proc/sys/net/netfilter/nf_conntrack_acct
	if err := sysctl.SetAcct(body.On); err != nil {
		writeJSON(w, 422, errBody(10480, "即时开启失败",
			"写 "+sysctl.AcctPath()+" 失败："+err.Error()+
				"。mgmtd 需以 root 运行，且 nf_conntrack 模块须已加载。"))
		return
	}

	persisted := false
	if body.Persist {
		if err := sysctl.PersistAcctConf("", body.On); err != nil {
			writeJSON(w, 422, errBody(10481, "持久化失败",
				"写 /etc/sysctl.d/90-router-re-conntrack.conf 失败："+err.Error()))
			return
		}
		persisted = true
	}

	writeJSON(w, 200, map[string]interface{}{
		"on":        body.On,
		"applied":   true,
		"persisted": persisted,
	})
}
