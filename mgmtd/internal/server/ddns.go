package server

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/ddns"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

const ddnsConfigKey = "ddns/config"
const ntpConfigKey = "ntp/config"

func (s *Server) handleGetDDNSConfig(w http.ResponseWriter, r *http.Request) {
	cfg := ddns.Config{}
	raw, _ := s.store.Get("ddns", "config")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	// mask token: never echo the full token back
	out := cfg
	if out.Token != "" {
		out.Token = maskToken(out.Token)
	}
	writeJSON(w, 200, map[string]interface{}{"config": out})
}

func (s *Server) handleSetDDNSConfig(w http.ResponseWriter, r *http.Request) {
	var cfg ddns.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10110, "json decode fail", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("ddns", "config", string(b))
	out := cfg
	if out.Token != "" {
		out.Token = maskToken(out.Token)
	}
	writeJSON(w, 200, map[string]interface{}{"config": out, "saved": true})
}

func (s *Server) handleDDNSUpdate(w http.ResponseWriter, r *http.Request) {
	cfg := ddns.Config{}
	raw, _ := s.store.Get("ddns", "config")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Provider == "" {
		writeJSON(w, 400, errBody(10111, "ddns not configured", "configure a provider first"))
		return
	}
	ip, err := ddns.DetectPublicIP(nil)
	if err != nil {
		writeJSON(w, 500, errBody(10112, "public ip detect failed", err.Error()))
		return
	}
	var p ddns.Provider
	switch cfg.Provider {
	case "cloudflare", "Cloudflare":
		p = ddns.CloudflareProvider{}
	case "alidns", "AliDNS", "aliyun", "Alibaba":
		p = ddns.AliDNSProvider{}
	default:
		writeJSON(w, 400, errBody(10113, "unsupported provider", cfg.Provider))
		return
	}
	res := map[string]interface{}{"ip": ip, "provider": cfg.Provider}
	if err := p.Update(cfg, ip); err != nil {
		res["ok"] = false
		res["detail"] = err.Error()
		writeJSON(w, 200, res)
		return
	}
	res["ok"] = true
	res["detail"] = "DNS record updated"
	writeJSON(w, 200, res)
}

func (s *Server) handleGETDDNSIP(w http.ResponseWriter, r *http.Request) {
	ip, err := ddns.DetectPublicIP(nil)
	if err != nil {
		writeJSON(w, 500, errBody(10114, "public ip detect failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ip": ip})
}

func (s *Server) handleGetNTPConfig(w http.ResponseWriter, r *http.Request) {
	cfg := ddns.NTPConfig{}
	raw, _ := s.store.Get("ntp", "config")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg})
}

func (s *Server) handleSetNTPConfig(w http.ResponseWriter, r *http.Request) {
	var cfg ddns.NTPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10115, "json decode fail", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("ntp", "config", string(b))
	res := map[string]interface{}{
		"config":  cfg,
		"text":    ddns.RenderTimesyncd(cfg),
		"applied": false,
	}
	if cfg.Enabled {
		err := ddns.ApplyTimesyncd(execx.DefaultRunner{}, cfg)
		res["applied"] = err == nil
		if err != nil {
			res["reason"] = err.Error()
		} else {
			res["reason"] = "timesyncd restarted with new config"
		}
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleNTPStatus(w http.ResponseWriter, r *http.Request) {
	st, err := ddns.Status(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10116, "ntp status failed", err.Error()))
		return
	}
	writeJSON(w, 200, st)
}

func maskToken(t string) string {
	if len(t) <= 4 {
		return "****"
	}
	return t[:2] + "****" + t[len(t)-2:]
}

var _ = net.ParseIP
var _ = time.Now
