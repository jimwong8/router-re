package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/vpnclient"
)

// VPN 客户端：多种协议并存的多连接管理。
//
// 之前只有一条 WireGuard 配置，保存会互相覆盖；实际运维里经常是
// "一条 WireGuard 连总部 + 一条 L2TP 连分支 + 一份 OpenVPN 应急"，
// 所以这里改成按名字索引的列表。
//
// 密码一律不回显：GET 只给出 has_password 布尔值，POST 时留空表示沿用旧值。
// 这与 VPN 服务端那批的处理保持一致——一次截图不该泄露所有拨号凭据。

// vpnClientEntry 是存储里的一条记录。
type vpnClientEntry struct {
	Kind      string             `json:"kind"` // wireguard / pptp / l2tp / openvpn
	Name      string             `json:"name"`
	PPP       *vpnclient.PPPSpec `json:"ppp,omitempty"`
	Ovpn      string             `json:"ovpn,omitempty"`
	Enabled   bool               `json:"enabled"`
	Connected bool               `json:"connected,omitempty"`
	Comment   string             `json:"comment,omitempty"`
}

// vpnClientView 是返回给前端的形态（脱敏）。
type vpnClientView struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Server      string `json:"server,omitempty"`
	Username    string `json:"username,omitempty"`
	Enabled     bool   `json:"enabled"`
	HasPassword bool   `json:"has_password"`
	Comment     string `json:"comment,omitempty"`
	Remote      string `json:"remote,omitempty"` // openvpn: host:port
	Note        string `json:"note,omitempty"`
}

const vpnClientsKey = "clients"

func (s *Server) loadVPNClients() []vpnClientEntry {
	var list []vpnClientEntry
	raw, err := s.store.Get("vpnclient", vpnClientsKey)
	if err != nil || raw == "" {
		return nil
	}
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

func (s *Server) saveVPNClients(list []vpnClientEntry) error {
	b, _ := json.Marshal(list)
	return s.store.Set("vpnclient", vpnClientsKey, string(b))
}

func toView(e vpnClientEntry) vpnClientView {
	v := vpnClientView{Kind: e.Kind, Name: e.Name, Enabled: e.Enabled, Comment: e.Comment}
	switch e.Kind {
	case "pptp", "l2tp":
		if e.PPP != nil {
			v.Server = e.PPP.Server
			v.Username = e.PPP.Username
			v.HasPassword = e.PPP.Password != ""
			if e.PPP.Comment != "" {
				v.Comment = e.PPP.Comment
			}
		}
	case "openvpn":
		host, port := vpnclient.OvpnRemote(e.Ovpn)
		if host != "" {
			v.Remote = host + ":" + itoa(port)
		}
		v.HasPassword = false
	case "wireguard":
		v.Note = "WireGuard 配置在「虚拟专网 / WireGuard」页维护"
	}
	return v
}

func (s *Server) handleListVPNClients(w http.ResponseWriter, r *http.Request) {
	list := s.loadVPNClients()
	out := make([]vpnClientView, 0, len(list))
	for _, e := range list {
		out = append(out, toView(e))
	}
	writeJSON(w, 200, map[string]interface{}{
		"clients": out,
		"note": "密码不回显：编辑时留空表示沿用已保存的值。" +
			"只有「全局走 VPN」打开时才会替换默认路由；否则需要你自己在静态路由里指定走隧道的网段。",
		"split_note": vpnclient.SplitTunnelNote(),
	})
}

func (s *Server) handleSaveVPNClientEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
		vpnclient.PPPSpec
		Ovpn    string `json:"ovpn"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(11800, "json decode fail", err.Error()))
		return
	}

	list := s.loadVPNClients()
	idx := -1
	for i, e := range list {
		if e.Kind == body.Kind && e.Name == body.Name {
			idx = i
			break
		}
	}

	switch body.Kind {
	case "pptp", "l2tp":
		spec := body.PPPSpec
		spec.Kind = body.Kind
		spec.Enabled = body.Enabled
		// 密码留空表示沿用旧值：把它填回来再校验，避免"改个 MTU 就要重输密码"
		if spec.Password == "" && idx >= 0 && list[idx].PPP != nil {
			spec.Password = list[idx].PPP.Password
		}
		if err := vpnclient.ValidatePPP(spec); err != nil {
			writeJSON(w, 422, errBody(11801, "vpn client invalid", err.Error()))
			return
		}
		entry := vpnClientEntry{Kind: body.Kind, Name: spec.Name, PPP: &spec, Enabled: body.Enabled, Comment: spec.Comment}
		if idx >= 0 {
			list[idx] = entry
		} else {
			list = append(list, entry)
		}
	case "openvpn":
		if err := vpnclient.ValidateOvpn(body.Ovpn); err != nil {
			writeJSON(w, 422, errBody(11802, "ovpn rejected", err.Error()))
			return
		}
		if body.Name == "" {
			// 名字从配置里推不出就用 remote 主机名，方便区分多条
			host, _ := vpnclient.OvpnRemote(body.Ovpn)
			body.Name = strings.ReplaceAll(host, ".", "-")
		}
		if err := validEntryName(body.Name); err != nil {
			writeJSON(w, 422, errBody(11803, "name invalid", err.Error()))
			return
		}
		entry := vpnClientEntry{Kind: "openvpn", Name: body.Name, Ovpn: body.Ovpn, Enabled: body.Enabled}
		if idx >= 0 {
			list[idx] = entry
		} else {
			list = append(list, entry)
		}
	default:
		writeJSON(w, 422, errBody(11804, "unsupported kind", "只支持 pptp / l2tp / openvpn（WireGuard 请用 WireGuard 页）"))
		return
	}

	if err := s.saveVPNClients(list); err != nil {
		writeJSON(w, 500, errBody(11805, "save failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"saved": true, "name": body.Name, "kind": body.Kind})
}

func validEntryName(n string) error {
	if n == "" {
		return errStr("名称不能为空")
	}
	if len(n) > 32 {
		return errStr("名称最长 32 字符")
	}
	for _, c := range n {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
		if !ok {
			return errStr("名称只能用字母、数字、点、下划线、短横线")
		}
	}
	return nil
}

type errStr string

func (e errStr) Error() string { return string(e) }

// handleVPNClientPlan 生成一份"要写哪些文件、跑哪些命令"的计划，不执行。
//
// 路由器上改 VPN 配置的代价很高（写错就断网），所以先给计划再执行，
// 和磁盘管理页的做法一致。
func (s *Server) handleVPNClientPlan(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	kind := r.URL.Query().Get("kind")
	var found *vpnClientEntry
	for _, e := range s.loadVPNClients() {
		if e.Name == name && e.Kind == kind {
			c := e
			found = &c
			break
		}
	}
	if found == nil {
		writeJSON(w, 404, errBody(11806, "not found", "没有这条 VPN 客户端配置"))
		return
	}

	var files []map[string]string
	var cmds []string
	switch found.Kind {
	case "pptp", "l2tp":
		peers, err := vpnclient.RenderPeers(*found.PPP)
		if err != nil {
			writeJSON(w, 422, errBody(11807, "render fail", err.Error()))
			return
		}
		files = append(files, map[string]string{
			"path": "/etc/ppp/peers/" + found.Name, "content": peers, "mode": "600"})
		secret, _ := vpnclient.ChapSecretLine(*found.PPP)
		files = append(files, map[string]string{
			"path": "/etc/ppp/chap-secrets", "content": "# 追加一行（内容含密码，不回显）\n" +
				strings.SplitN(secret, "\t", 2)[0] + "\t<redacted>", "mode": "600"})
		if found.Kind == "l2tp" {
			xl, _ := vpnclient.RenderXl2tpd(*found.PPP)
			files = append(files, map[string]string{"path": "/etc/xl2tpd/xl2tpd.conf", "content": xl, "mode": "600"})
			cmds = append(cmds, "systemctl enable --now xl2tpd")
			cmds = append(cmds, "systemctl restart rr-l2tp-"+found.Name)
		} else {
			cmds = append(cmds, "systemctl enable --now rr-pptp-"+found.Name)
			cmds = append(cmds, "pon "+found.Name)
		}
		unit := "[Unit]\nDescription=router-re " + found.Kind + " client " + found.Name +
			"\n\n[Service]\nType=forking\nExecStart=/usr/bin/pon " + found.Name +
			"\nExecStop=/usr/bin/poff " + found.Name + "\nRestart=on-failure\n\n[Install]\nWantedBy=multi-user.target\n"
		files = append(files, map[string]string{
			"path": "/etc/systemd/system/" + vpnclient.ServiceName(*found.PPP) + ".service", "content": unit, "mode": "644"})
	case "openvpn":
		unit, err := vpnclient.RenderOvpnUnit(found.Name, "/etc/router-re/openvpn/"+found.Name+".ovpn")
		if err != nil {
			writeJSON(w, 422, errBody(11808, "render fail", err.Error()))
			return
		}
		files = append(files, map[string]string{
			"path":    "/etc/router-re/openvpn/" + found.Name + ".ovpn",
			"content": "# 内容不回显，写入后权限 600\n", "mode": "600"})
		files = append(files, map[string]string{
			"path": "/etc/systemd/system/rr-ovpn-" + found.Name + ".service", "content": unit, "mode": "644"})
		cmds = append(cmds, "systemctl daemon-reload")
		cmds = append(cmds, "systemctl enable --now rr-ovpn-"+found.Name)
	}

	// 依赖检查：缺什么先告诉用户，而不是执行到一半才发现
	var missing []string
	if found.Kind == "pptp" {
		if _, err := os.Stat("/usr/sbin/pptp"); err != nil {
			missing = append(missing, "pptp（pptp-linux 包）")
		}
	}
	if found.Kind == "l2tp" {
		if _, err := os.Stat("/usr/sbin/xl2tpd"); err != nil {
			missing = append(missing, "xl2tpd")
		}
	}
	if found.Kind == "openvpn" {
		if _, err := os.Stat("/usr/sbin/openvpn"); err != nil {
			missing = append(missing, "openvpn")
		}
	}
	if _, err := os.Stat("/usr/sbin/pppd"); err != nil && found.Kind != "openvpn" {
		missing = append(missing, "pppd（ppp 包）")
	}

	writeJSON(w, 200, map[string]interface{}{
		"kind": found.Kind, "name": found.Name,
		"files": files, "commands": cmds, "missing": missing,
		"note": "计划不会自动执行。缺依赖时先装包，否则服务会起不来。",
	})
}

func (s *Server) handleDeleteVPNClientEntry(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	kind := r.URL.Query().Get("kind")
	list := s.loadVPNClients()
	next := make([]vpnClientEntry, 0, len(list))
	removed := false
	for _, e := range list {
		if e.Name == name && e.Kind == kind {
			removed = true
			continue
		}
		next = append(next, e)
	}
	if !removed {
		writeJSON(w, 404, errBody(11809, "not found", "没有这条 VPN 客户端配置"))
		return
	}
	if err := s.saveVPNClients(next); err != nil {
		writeJSON(w, 500, errBody(11810, "save failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": true, "name": name})
}
