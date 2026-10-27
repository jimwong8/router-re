package server

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/jimwong/router-re/mgmtd/internal/dnsproxy"
)

const dnsConfigKey = "dns/proxy"

// loadDNSConfig reads the persisted DNS proxy config (empty when unset).
func (s *Server) loadDNSConfig() dnsproxy.Config {
	cfg := dnsproxy.Config{}
	raw, _ := s.store.Get("dns", "proxy")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

// handleGetDNSConfig returns the config plus the rendered dnsmasq text so the
// UI can show exactly what would be written to disk.
func (s *Server) handleGetDNSConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadDNSConfig()
	writeJSON(w, 200, map[string]interface{}{
		"config":  cfg,
		"dnsmasq": dnsproxy.RenderDnsmasq(cfg),
		"hosts":   dnsproxy.RenderHosts(cfg),
	})
}

// handleSetDNSConfig validates, persists, then applies the config.
//
// Persist-before-apply: the operator's edit must survive even when the live
// apply fails (no CAP_NET_ADMIN in a dev box, dnsmasq not installed, a
// transient error). `applied` is reported separately so the UI can say
// "saved but not active" instead of silently losing the change.
func (s *Server) handleSetDNSConfig(w http.ResponseWriter, r *http.Request) {
	var cfg dnsproxy.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10130, "invalid body", err.Error()))
		return
	}
	if err := dnsproxy.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10131, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("dns", "proxy", string(b))

	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := dnsproxy.Apply(s.aclApplier, cfg, dnsproxy.DefaultConfPath); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "dnsmasq restarted with new config"
		}
	} else {
		detail = "no runner configured"
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"dnsmasq": dnsproxy.RenderDnsmasq(cfg),
		"detail":  detail,
	})
}

// handleDNSResolve resolves a name through the local resolver.
// The name is validated inside dnsproxy.Resolve before any command runs.
func (s *Server) handleDNSResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10132, "invalid body", err.Error()))
		return
	}
	ips, err := dnsproxy.Resolve(s.aclApplier, req.Name)
	if err != nil {
		writeJSON(w, 404, errBody(10133, "resolve failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"name": req.Name,
		"ips":  ips,
	})
}

// handleGetDNSStatus reports dnsmasq's running state, whether the rendered
// config file exists, and the configured upstreams.
func (s *Server) handleGetDNSStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadDNSConfig()
	running := false
	statusDetail := ""
	if s.aclApplier != nil {
		if out, err := s.aclApplier.Run("systemctl", "is-active", "dnsmasq"); err == nil {
			running = out == "active"
			statusDetail = out
		} else {
			statusDetail = err.Error()
		}
	}
	confExists := false
	if _, err := os.Stat(dnsproxy.DefaultConfPath); err == nil {
		confExists = true
	}
	writeJSON(w, 200, map[string]interface{}{
		"running":     running,
		"status":      statusDetail,
		"conf_exists": confExists,
		"conf_path":   dnsproxy.DefaultConfPath,
		"enabled":     cfg.Enabled,
		"upstreams":   cfg.Upstreams,
		"cache_size":  cfg.CacheSize,
	})
}
