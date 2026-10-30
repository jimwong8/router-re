package server

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/filesvc"
)

// FTP / Samba / HTTP 文件服务。
//
// 三个服务共用一套流程：校验 → 渲染配置文本 → 落盘 → （可选）重启服务。
// 密码只在写入时使用，GET 一律脱敏成 password_set 布尔值，正文里不出现。

func (s *Server) filesvcConfig(kind filesvc.Kind) (interface{}, string, bool) {
	key := "filesvc_" + string(kind)
	raw, _ := s.store.Get("filesvc", key)
	def := filesvc.Defaults(kind)
	if raw == "" {
		return def, "", false
	}
	switch kind {
	case filesvc.KindFTP:
		var c filesvc.FTPConfig
		if json.Unmarshal([]byte(raw), &c) == nil {
			return c, c.Password, c.Password != ""
		}
	case filesvc.KindSamba:
		var c filesvc.SambaConfig
		if json.Unmarshal([]byte(raw), &c) == nil {
			return c, c.Password, c.Password != ""
		}
	default:
		var c filesvc.HTTPConfig
		if json.Unmarshal([]byte(raw), &c) == nil {
			return c, "", false
		}
	}
	return def, "", false
}

// filesvcStatus 探测服务是否安装、是否运行。
//
// 不谎报：没装就说 not_installed，装了但没起就说 stopped。
func filesvcStatus(kind filesvc.Kind) map[string]interface{} {
	bin := filesvc.BinaryName(kind)
	path, err := exec.LookPath(bin)
	installed := err == nil
	running := false
	if installed {
		// pgrep -x 精确匹配进程名，避免把 vsftpd-wrapper 之类算进来
		if err := exec.Command("pgrep", "-x", bin).Run(); err == nil {
			running = true
		}
	}
	return map[string]interface{}{
		"installed": installed,
		"binary":    bin,
		"path":      path,
		"running":   running,
	}
}

func (s *Server) handleGetFileService(w http.ResponseWriter, r *http.Request) {
	kind := filesvc.Kind(r.PathValue("kind"))
	switch kind {
	case filesvc.KindFTP, filesvc.KindSamba, filesvc.KindHTTP:
	default:
		writeJSON(w, 422, errBody(11300, "unknown kind", "只支持 ftp / samba / http"))
		return
	}
	cfg, pw, pwSet := s.filesvcConfig(kind)

	var conf string
	switch kind {
	case filesvc.KindFTP:
		c := cfg.(filesvc.FTPConfig)
		c.Password = pw
		conf = filesvc.RenderFTP(c)
		// 脱敏后再返回
		c.Password = ""
		cfg = c
	case filesvc.KindSamba:
		c := cfg.(filesvc.SambaConfig)
		c.Password = pw
		conf = filesvc.RenderSamba(c)
		c.Password = ""
		cfg = c
	default:
		conf = filesvc.RenderHTTP(cfg.(filesvc.HTTPConfig))
	}

	writeJSON(w, 200, map[string]interface{}{
		"kind":         kind,
		"config":       cfg,
		"password_set": pwSet,
		"conf":         conf,
		"conf_path":    filesvc.ConfPath(kind),
		"status":       filesvcStatus(kind),
		"storage_root": filesvc.StorageRoot,
	})
}

func (s *Server) handleSetFileService(w http.ResponseWriter, r *http.Request) {
	kind := filesvc.Kind(r.PathValue("kind"))
	// 先读出旧配置，用于"没填密码就沿用旧密码"
	oldCfg, oldPw, _ := s.filesvcConfig(kind)

	var apply struct {
		Apply bool `json:"apply"`
	}
	bodyRaw := json.NewDecoder(r.Body)
	var payload map[string]json.RawMessage
	if err := bodyRaw.Decode(&payload); err != nil {
		writeJSON(w, 400, errBody(11301, "json decode fail", err.Error()))
		return
	}
	if raw, ok := payload["apply"]; ok {
		_ = json.Unmarshal(raw, &apply.Apply)
	}

	var conf string
	var save interface{}
	switch kind {
	case filesvc.KindFTP:
		c := oldCfg.(filesvc.FTPConfig)
		if err := remarshal(payload, &c); err != nil {
			writeJSON(w, 400, errBody(11302, "bad ftp config", err.Error()))
			return
		}
		if c.Password == "" {
			c.Password = oldPw // 前端不回显密码，留空表示不修改
		}
		if err := filesvc.ValidateFTP(c); err != nil {
			writeJSON(w, 422, errBody(11303, "ftp config rejected", err.Error()))
			return
		}
		conf = filesvc.RenderFTP(c)
		save = c
	case filesvc.KindSamba:
		c := oldCfg.(filesvc.SambaConfig)
		if err := remarshal(payload, &c); err != nil {
			writeJSON(w, 400, errBody(11304, "bad samba config", err.Error()))
			return
		}
		if c.Password == "" {
			c.Password = oldPw
		}
		if err := filesvc.ValidateSamba(c); err != nil {
			writeJSON(w, 422, errBody(11305, "samba config rejected", err.Error()))
			return
		}
		conf = filesvc.RenderSamba(c)
		save = c
	case filesvc.KindHTTP:
		c := oldCfg.(filesvc.HTTPConfig)
		if err := remarshal(payload, &c); err != nil {
			writeJSON(w, 400, errBody(11306, "bad http config", err.Error()))
			return
		}
		if err := filesvc.ValidateHTTP(c); err != nil {
			writeJSON(w, 422, errBody(11307, "http config rejected", err.Error()))
			return
		}
		conf = filesvc.RenderHTTP(c)
		save = c
	default:
		writeJSON(w, 422, errBody(11308, "unknown kind", "只支持 ftp / samba / http"))
		return
	}

	path := filesvc.ConfPath(kind)
	if err := writeFileSafe(path, conf); err != nil {
		// 目标目录可能不存在（服务没装过），如实报错而不是假装保存成功
		writeJSON(w, 500, errBody(11309, "write conf failed",
			err.Error()+"（服务可能未安装，配置目录不存在）"))
		return
	}
	b, _ := json.Marshal(save)
	_ = s.store.Set("filesvc", "filesvc_"+string(kind), string(b))

	resp := map[string]interface{}{
		"kind":      kind,
		"saved":     true,
		"conf":      conf,
		"conf_path": path,
		"applied":   false,
		"status":    filesvcStatus(kind),
	}
	if apply.Apply {
		st := filesvcStatus(kind)
		if !st["installed"].(bool) {
			resp["note"] = "配置已保存，但 " + filesvc.BinaryName(kind) + " 未安装，无法启动服务"
		} else {
			if out, err := restartService(kind); err != nil {
				resp["note"] = "配置已保存，重启服务失败：" + strings.TrimSpace(out)
			} else {
				resp["applied"] = true
				resp["status"] = filesvcStatus(kind)
			}
		}
	}
	writeJSON(w, 200, resp)
}

// restartService 重启服务：优先 init.d，其次 systemctl。两条都不存在时返回原因。
func restartService(kind filesvc.Kind) (string, error) {
	name := string(kind)
	if kind == filesvc.KindFTP {
		name = "vsftpd"
	} else if kind == filesvc.KindSamba {
		name = "samba"
	} else {
		name = "lighttpd"
	}
	initd := "/etc/init.d/" + name
	if _, err := os.Stat(initd); err == nil {
		out, err := exec.Command(initd, "restart").CombinedOutput()
		return string(out), err
	}
	if p, err := exec.LookPath("systemctl"); err == nil {
		out, err := exec.Command(p, "restart", name).CombinedOutput()
		return string(out), err
	}
	return "没有 /etc/init.d/" + name + "，也没有 systemctl", os.ErrNotExist
}

// remarshal 把 payload 合并进 dst。只覆盖请求里出现的字段，
// 避免"前端少传一个字段就把后端配置清空"。
func remarshal(payload map[string]json.RawMessage, dst interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
