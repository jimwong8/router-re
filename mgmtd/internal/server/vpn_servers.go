package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/ovpn"
	"github.com/jimwong/router-re/mgmtd/internal/vpn"
)

// PPTP / L2TP / OpenVPN 服务端。
//
// 路由路径沿用 iKuai 的命名（vpn/pptp/services、vpn/l2tp/services、
// vpn/openvpn/services），字段也逐个对齐，方便从爱快迁配置、也方便
// 两边对照排查。与 iKuai 不同的是：每次保存都先 Validate 再落盘，
// 校验不过不写文件、不重启服务，避免把路由器配成"服务起不来的状态"。

const (
	pptpConfPath    = "/etc/pptpd.conf"
	pptpOptionsPath = "/etc/ppp/pptpd-options"
	chapSecretsPath = "/etc/ppp/chap-secrets"
	xl2tpdConfPath  = "/etc/xl2tpd/xl2tpd.conf"
	xl2tpdOptsPath  = "/etc/ppp/options.xl2tpd"
	ipsecConfPath   = "/etc/ipsec.d/router-re-l2tp.conf"
	openvpnConfPath = "/etc/openvpn/server/router-re.conf"
)

// writeFileSafe 写文件前建目录；内容为空时删除文件（表示"不启用该片段"）。
func writeFileSafe(path, content string) error {
	if strings.TrimSpace(content) == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// ensureOpenVPNPKI 保证 server.conf 引用的四份文件真实存在。
//
// 之前没有任何代码负责生成它们：conf 里写着 rr-ca.crt / rr-server.crt /
// rr-server.key / rr-tls-crypt.key，实际一个都没有，openvpn 起来就
// "Cannot pre-load keyfile ... Exiting due to fatal error" —— 而接口只回
// applied=false + 一段 systemctl 文本，看不出真正原因。
// 这里在渲染配置前用 ovpn.LoadOrCreate 生成 CA 与服务端证书（已存在则复用），
// tls-crypt 用一份本地随机静态密钥，写完再渲染。
func ensureOpenVPNPKI() error {
	const dir = "/etc/openvpn/server"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ca, err := ovpn.LoadOrCreate(dir + "/pki")
	if err != nil {
		return err
	}
	cert, key, err := ca.IssueServer("vpn-server", nil, []string{"10.8.0.1"})
	if err != nil {
		return err
	}
	files := map[string]struct {
		data []byte
		mode os.FileMode
	}{
		dir + "/rr-ca.crt":     {ca.CACertPEM(), 0o644},
		dir + "/rr-server.crt": {cert, 0o644},
		dir + "/rr-server.key": {key, 0o600},
	}
	for name, f := range files {
		// 已存在就不覆盖：换了服务端证书，已签发的客户端会全部失联
		if _, statErr := os.Stat(name); statErr == nil {
			continue
		}
		if err := os.WriteFile(name, f.data, f.mode); err != nil {
			return err
		}
	}
	tc := dir + "/rr-tls-crypt.key"
	if _, statErr := os.Stat(tc); statErr != nil {
		buf := make([]byte, 256)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		var sb strings.Builder
		sb.WriteString("-----BEGIN OpenVPN Static key V1-----\n")
		for i := 0; i < len(buf); i += 16 {
			fmt.Fprintf(&sb, "%x\n", buf[i:i+16])
		}
		sb.WriteString("-----END OpenVPN Static key V1-----\n")
		if err := os.WriteFile(tc, []byte(sb.String()), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// openvpnStartError 把 systemd 的 is-active 结果翻成一句能看懂的话，
// 并补上日志里最关键的一行（例如 tun 打不开、证书读不到）。
func openvpnStartError(r execx.Runner, state string) string {
	msg := "openvpn 服务状态为 " + state + "，未能启动"
	if out, err := r.Run("journalctl", "-u", "openvpn-server@router-re", "-n", "20", "--no-pager"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			l := strings.ToLower(line)
			if strings.Contains(l, "cannot open tun") || strings.Contains(l, "cannot pre-load keyfile") ||
				strings.Contains(l, "error:") || strings.Contains(l, "fatal") {
				msg += "；" + strings.TrimSpace(line)
				break
			}
		}
	}
	return msg
}

func runCommands(r execx.Runner, cmds [][]string) (bool, string) {
	for _, c := range cmds {
		if len(c) == 0 {
			continue
		}
		if _, err := r.Run(c[0], c[1:]...); err != nil {
			return false, c[0] + " " + strings.Join(c[1:], " ") + ": " + err.Error()
		}
	}
	return true, ""
}

func serviceActive(r execx.Runner, name string) string {
	out, err := r.Run("systemctl", "is-active", name)
	out = strings.TrimSpace(out)
	if err != nil && out == "" {
		return "unknown"
	}
	return out
}

// vpnMaskUsers 返回一份密码被清空的用户列表。
//
// 口令一律不回显：既是"凭据绝不回显"的要求，也避免有人截图配置页就把
// 所有拨号账号密码一起发出去。前端只能看到 username/ip/comment。
func vpnMaskUsers(in []vpn.VPNUser) []vpn.VPNUser {
	if in == nil {
		return nil
	}
	out := make([]vpn.VPNUser, 0, len(in))
	for _, u := range in {
		c := u
		c.Password = ""
		out = append(out, c)
	}
	return out
}

// vpnMergeUsers 把提交上来的用户列表与已保存的合并。
//
// 规则：password 留空 = 沿用同 user 名的旧密码；新用户留空则校验阶段会拒绝。
// 这样前端不回显密码也不会把已有账号的密码清掉。
func vpnMergeUsers(newUsers, oldUsers []vpn.VPNUser) []vpn.VPNUser {
	old := make(map[string]string, len(oldUsers))
	for _, u := range oldUsers {
		old[u.Username] = u.Password
	}
	out := make([]vpn.VPNUser, 0, len(newUsers))
	for _, u := range newUsers {
		c := u
		if c.Password == "" {
			if pw, ok := old[c.Username]; ok {
				c.Password = pw
			}
		}
		out = append(out, c)
	}
	return out
}

// ---------- PPTP ----------

func (s *Server) handleGetPPTP(w http.ResponseWriter, r *http.Request) {
	cfg := vpn.PPTPConfig{}
	if raw, _ := s.store.Get("vpn", "pptp"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.OpenMPPE == 0 && cfg.MTU == 0 {
		cfg.OpenMPPE = 2 // 展示默认值，避免界面显示"不加密"但其实下发了加密
	}
	out := cfg
	out.Users = vpnMaskUsers(cfg.Users)
	writeJSON(w, 200, map[string]interface{}{
		"config":   out,
		"status":   serviceActive(execx.DefaultRunner{}, "pptpd"),
		"writable": dirWritable(filepath.Dir(pptpConfPath)),
	})
}

func (s *Server) handleSetPPTP(w http.ResponseWriter, r *http.Request) {
	var cfg vpn.PPTPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(11000, "json decode fail", err.Error()))
		return
	}
	// 前端不回显密码，留空表示沿用已保存的密码
	var old vpn.PPTPConfig
	if raw, _ := s.store.Get("vpn", "pptp"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &old)
	}
	cfg.Users = vpnMergeUsers(cfg.Users, old.Users)
	if err := vpn.ValidatePPTP(cfg); err != nil {
		writeJSON(w, 422, errBody(11001, "pptp config invalid", err.Error()))
		return
	}
	runner := execx.DefaultRunner{}
	// 先落盘配置，再启停；任何一步失败都如实返回，不谎报 applied
	if err := writeFileSafe(pptpConfPath, vpn.RenderPptpdConf(cfg)); err != nil {
		writeJSON(w, 500, errBody(11002, "write pptpd.conf failed", err.Error()))
		return
	}
	if err := writeFileSafe(pptpOptionsPath, vpn.RenderPptpdOptions(cfg)); err != nil {
		writeJSON(w, 500, errBody(11003, "write pptpd-options failed", err.Error()))
		return
	}
	if err := writeFileSafe(chapSecretsPath, vpn.RenderChapSecrets(cfg.Users)); err != nil {
		writeJSON(w, 500, errBody(11004, "write chap-secrets failed", err.Error()))
		return
	}
	ok, detail := runCommands(runner, vpn.RenderPPTPCommands(cfg.Enabled))
	if b, err := json.Marshal(cfg); err == nil {
		_ = s.store.Set("vpn", "pptp", string(b))
	}
	outCfg := cfg
	outCfg.Users = vpnMaskUsers(cfg.Users)
	writeJSON(w, 200, map[string]interface{}{
		"config":  outCfg,
		"applied": ok,
		"error":   detail,
		"status":  serviceActive(runner, "pptpd"),
	})
}

// ---------- L2TP ----------

func (s *Server) handleGetL2TP(w http.ResponseWriter, r *http.Request) {
	cfg := vpn.L2TPConfig{}
	if raw, _ := s.store.Get("vpn", "l2tp"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	out := cfg
	// PSK 不回显（连首尾字符也不给）：这是预共享密钥，泄露首尾就缩小了爆破空间
	out.IPSecSecret = ""
	out.Users = vpnMaskUsers(cfg.Users)
	writeJSON(w, 200, map[string]interface{}{
		"config":  out,
		"psk_set": cfg.IPSecSecret != "",
		"status":  serviceActive(execx.DefaultRunner{}, "xl2tpd"),
	})
}

func (s *Server) handleSetL2TP(w http.ResponseWriter, r *http.Request) {
	var cfg vpn.L2TPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(11005, "json decode fail", err.Error()))
		return
	}
	// PSK 与账号密码都不回显：留空（或带 *** 的旧掩码）表示沿用已保存的值
	old := vpn.L2TPConfig{}
	if raw, _ := s.store.Get("vpn", "l2tp"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &old)
	}
	if cfg.IPSecSecret == "" || strings.Contains(cfg.IPSecSecret, "***") {
		cfg.IPSecSecret = old.IPSecSecret
	}
	cfg.Users = vpnMergeUsers(cfg.Users, old.Users)
	if err := vpn.ValidateL2TP(cfg); err != nil {
		writeJSON(w, 422, errBody(11006, "l2tp config invalid", err.Error()))
		return
	}
	runner := execx.DefaultRunner{}
	if err := writeFileSafe(xl2tpdConfPath, vpn.RenderXl2tpdConf(cfg)); err != nil {
		writeJSON(w, 500, errBody(11007, "write xl2tpd.conf failed", err.Error()))
		return
	}
	if err := writeFileSafe(xl2tpdOptsPath, vpn.RenderXl2tpdOptions(cfg)); err != nil {
		writeJSON(w, 500, errBody(11008, "write options.xl2tpd failed", err.Error()))
		return
	}
	if cfg.ForceIPSec == 1 {
		if err := writeFileSafe(ipsecConfPath, vpn.RenderIPSecConf(cfg)); err != nil {
			writeJSON(w, 500, errBody(11009, "write ipsec conf failed", err.Error()))
			return
		}
		if err := writeFileSafe("/etc/ipsec.d/router-re-l2tp.secrets", vpn.RenderIPSecSecrets(cfg)); err != nil {
			writeJSON(w, 500, errBody(11010, "write ipsec secrets failed", err.Error()))
			return
		}
	}
	if err := writeFileSafe(chapSecretsPath, vpn.RenderChapSecrets(cfg.Users)); err != nil {
		writeJSON(w, 500, errBody(11011, "write chap-secrets failed", err.Error()))
		return
	}
	ok, detail := runCommands(runner, vpn.RenderL2TPCommands(cfg))
	if b, err := json.Marshal(cfg); err == nil {
		_ = s.store.Set("vpn", "l2tp", string(b))
	}
	out := cfg
	out.IPSecSecret = ""
	out.Users = vpnMaskUsers(cfg.Users)
	writeJSON(w, 200, map[string]interface{}{
		"config":  out,
		"psk_set": cfg.IPSecSecret != "",
		"applied": ok,
		"error":   detail,
		"status":  serviceActive(runner, "xl2tpd"),
	})
}

// ---------- OpenVPN ----------

func (s *Server) handleGetOpenVPN(w http.ResponseWriter, r *http.Request) {
	cfg := vpn.OpenVPNConfig{}
	if raw, _ := s.store.Get("vpn", "openvpn"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	out := cfg
	out.Users = vpnMaskUsers(cfg.Users)
	writeJSON(w, 200, map[string]interface{}{
		"config": out,
		"status": serviceActive(execx.DefaultRunner{}, "openvpn-server@router-re"),
	})
}

func (s *Server) handleSetOpenVPN(w http.ResponseWriter, r *http.Request) {
	var cfg vpn.OpenVPNConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(11012, "json decode fail", err.Error()))
		return
	}
	// 密码不回显，留空即沿用已保存的
	var oldOvpn vpn.OpenVPNConfig
	if raw, _ := s.store.Get("vpn", "openvpn"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &oldOvpn)
	}
	cfg.Users = vpnMergeUsers(cfg.Users, oldOvpn.Users)
	if err := vpn.ValidateOpenVPN(cfg); err != nil {
		writeJSON(w, 422, errBody(11013, "openvpn config invalid", err.Error()))
		return
	}
	runner := execx.DefaultRunner{}
	// 证书与 tls-crypt 必须先生成，否则写出来的 conf 引用了不存在的文件
	if err := ensureOpenVPNPKI(); err != nil {
		writeJSON(w, 500, errBody(11015, "openvpn pki init failed", err.Error()))
		return
	}
	conf := vpn.RenderOpenVPNServerConf(cfg)
	if err := writeFileSafe(openvpnConfPath, conf); err != nil {
		writeJSON(w, 500, errBody(11014, "write openvpn conf failed", err.Error()))
		return
	}
	ok, detail := runCommands(runner, vpn.RenderOpenVPNCommands(cfg.Enabled))
	// systemctl restart 返回 0 只代表"提交了重启"，进程秒退时它一样返回 0。
	// 必须回读服务状态，否则接口会把崩溃循环报成 applied=true —— 这是这类
	// 功能最常见的假成功。等最多 3 秒让 systemd 把状态落定。
	if cfg.Enabled && ok {
		for i := 0; i < 6; i++ {
			if serviceActive(runner, "openvpn-server@router-re") == "active" {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if st := serviceActive(runner, "openvpn-server@router-re"); st != "active" {
			ok = false
			detail = openvpnStartError(runner, st)
		}
	}
	if b, err := json.Marshal(cfg); err == nil {
		_ = s.store.Set("vpn", "openvpn", string(b))
	}
	outCfg := cfg
	outCfg.Users = vpnMaskUsers(cfg.Users)
	writeJSON(w, 200, map[string]interface{}{
		"config":  outCfg,
		"conf":    conf,
		"applied": ok,
		"error":   detail,
		"status":  serviceActive(runner, "openvpn-server@router-re"),
	})
}

// handleOpenVPNClientConf 生成某个客户端的 .ovpn（证书由 certmgr 提供）。
func (s *Server) handleOpenVPNClientConf(w http.ResponseWriter, r *http.Request) {
	cfg := vpn.OpenVPNConfig{}
	if raw, _ := s.store.Get("vpn", "openvpn"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	var req struct {
		Name   string `json:"name"`
		Remote string `json:"remote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11015, "json decode fail", err.Error()))
		return
	}
	if req.Remote == "" {
		writeJSON(w, 422, errBody(11016, "remote required", "请填写客户端要连接的服务器地址"))
		return
	}
	// 每个客户端必须有自己的证书。
	// 旧实现把 /etc/openvpn/server/rr-client.crt|key 这对「共享证书」直接塞给任何请求，
	// 结果是所有人同一个身份：无法按人吊销、审计日志里分不清谁是谁、
	// 且任何拿到一份 .ovpn 的人都能拿去连。这里改成按 name 从 CA 现签。
	if err := ovpn.ValidateCN(req.Name); err != nil {
		writeJSON(w, 422, errBody(11017, "invalid client name", err.Error()))
		return
	}
	ca, err := ovpn.LoadOrCreate("/var/lib/router-re/ovpn/pki")
	if err != nil {
		writeJSON(w, 500, errBody(11018, "pki unavailable", err.Error()))
		return
	}
	cliCert, cliKey, err := ca.IssueClient(req.Name)
	if err != nil {
		writeJSON(w, 500, errBody(11019, "issue client cert failed", err.Error()))
		return
	}
	tlsCrypt := readOptional("/etc/openvpn/server/rr-tls-crypt.key")
	conf := vpn.RenderOpenVPNClientConf(cfg, req.Name,
		string(ca.CACertPEM()), string(cliCert), string(cliKey), tlsCrypt, req.Remote)
	writeJSON(w, 200, map[string]interface{}{"name": req.Name, "conf": conf})
}

// readOptional 读一个可选文件，不存在就返回空串。
func readOptional(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func dirWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".rr-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// handleGetOpenVPNStatus returns the parsed OpenVPN management status.
// It prefers a "text" query parameter (raw management output) and falls
// back to reading the conventional status log file via execx (slice args).
func (s *Server) handleGetOpenVPNStatus(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if strings.TrimSpace(text) == "" {
		out, err := execx.DefaultRunner{}.Run("cat", "/var/log/openvpn/status.log")
		if err == nil {
			text = out
		}
	}
	// ParseClientList 直接返回汇总 Status（客户端列表 + 累计流量 + 在线起始时间）
	st := ovpn.ParseClientList(text)
	if st.Clients == nil {
		st.Clients = []ovpn.Client{}
	}
	writeJSON(w, 200, st)
}
