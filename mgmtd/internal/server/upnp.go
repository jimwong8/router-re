package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/upnp"
)

const upnpConfigKey = "upnp/config"
const upnpMappingsKey = "upnp/mappings"

// loadUPNPConfig reads the persisted UPnP service config.
func (s *Server) loadUPNPConfig() upnp.Config {
	cfg := upnp.Config{}
	raw, _ := s.store.Get("upnp", "config")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	return cfg
}

// loadUPNPMappings reads the persisted hand-added mappings.
func (s *Server) loadUPNPMappings() []upnp.Mapping {
	ms := []upnp.Mapping{}
	s.loadJSON("upnp", "mappings", &ms)
	return ms
}

// instUpnpTable drops and recreates the UPnP nft table from the persisted
// mappings. It uses delete-table-then-create (never `flush ruleset`) so other
// tables and chains are never touched.
func (s *Server) applyUPNPNft(ms []upnp.Mapping) error {
	_, _ = s.aclApplier.Run("nft", "delete", "table", "inet", upnp.UpnpTable)
	_, err := s.aclApplier.RunWithInput(upnp.RenderNftMappings(ms), "nft", "-f", "-")
	return err
}

// handleGetUPNPConfig returns the config plus the rendered miniupnpd text.
func (s *Server) handleGetUPNPConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadUPNPConfig()
	writeJSON(w, 200, map[string]interface{}{
		"config":    cfg,
		"miniupnpd": upnp.RenderMiniupnpd(cfg),
	})
}

// handleSetUPNPConfig validates, persists and applies the service config.
func (s *Server) handleSetUPNPConfig(w http.ResponseWriter, r *http.Request) {
	var cfg upnp.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10140, "invalid body", err.Error()))
		return
	}
	if err := upnp.Validate(cfg); err != nil {
		writeJSON(w, 422, errBody(10141, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("upnp", "config", string(b))

	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := upnp.Apply(s.aclApplier, cfg, upnp.DefaultConfPath); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "miniupnpd restarted with new config"
		}
	} else {
		detail = "no runner configured"
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"detail":  detail,
	})
}

// handleGetUPNPMappings returns the stored mappings plus the current nft state.
func (s *Server) handleGetUPNPMappings(w http.ResponseWriter, r *http.Request) {
	ms := s.loadUPNPMappings()
	nft := ""
	if s.aclApplier != nil {
		if out, err := s.aclApplier.Run("nft", "list", "table", "inet", upnp.UpnpTable); err == nil {
			nft = out
		}
	}
	writeJSON(w, 200, map[string]interface{}{"mappings": ms, "nft": nft})
}

// handleSetUPNPMappings adds a single hand mapping: validate -> store ->
// apply nft -> read back the nft table.
func (s *Server) handleAddUPNPMapping(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadUPNPConfig()
	var m upnp.Mapping
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, 400, errBody(10142, "invalid body", err.Error()))
		return
	}
	if err := upnp.ValidateMapping(m, cfg); err != nil {
		writeJSON(w, 422, errBody(10143, "validation failed", err.Error()))
		return
	}
	ms := s.loadUPNPMappings()
	// replace existing entry with same proto+ext_port
	found := false
	for i := range ms {
		if ms[i].Protocol == m.Protocol && ms[i].ExtPort == m.ExtPort {
			ms[i] = m
			found = true
			break
		}
	}
	if !found {
		ms = append(ms, m)
	}
	s.storeJSON("upnp", "mappings", ms)

	applied := false
	detail := ""
	nftOut := upnp.RenderNftMappings(ms)
	if s.aclApplier != nil {
		if err := s.applyUPNPNft(ms); err != nil {
			detail = err.Error()
		} else {
			applied = true
			if out, err := s.aclApplier.Run("nft", "list", "table", "inet", upnp.UpnpTable); err == nil {
				nftOut = out
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied":  applied,
		"mappings": ms,
		"nft":      nftOut,
		"detail":   detail,
	})
}

// handleDeleteUPNPMapping removes a mapping by ext_port + protocol.
func (s *Server) handleDeleteUPNPMapping(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadUPNPConfig()
	_ = cfg
	var req struct {
		Proto   string `json:"protocol"`
		ExtPort int    `json:"ext_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10144, "invalid body", err.Error()))
		return
	}
	ms := s.loadUPNPMappings()
	kept := ms[:0]
	removed := false
	for _, m := range ms {
		if m.Protocol == strings.ToLower(req.Proto) && m.ExtPort == req.ExtPort {
			removed = true
			continue
		}
		kept = append(kept, m)
	}
	ms = kept
	s.storeJSON("upnp", "mappings", ms)

	applied := false
	detail := ""
	nftOut := upnp.RenderNftMappings(ms)
	if s.aclApplier != nil {
		if err := s.applyUPNPNft(ms); err != nil {
			detail = err.Error()
		} else {
			applied = true
			if out, err := s.aclApplier.Run("nft", "list", "table", "inet", upnp.UpnpTable); err == nil {
				nftOut = out
			}
		}
	}
	if !removed {
		writeJSON(w, 404, errBody(10145, "mapping not found", "no mapping with proto "+req.Proto+" and ext_port "+itoa(req.ExtPort)))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied":  applied,
		"mappings": ms,
		"nft":      nftOut,
		"detail":   detail,
	})
}

// handleGetUPNPStatus reports install/running state and the active mapping count
// from the live `upnpc -l` output (when upnpc is installed).
func (s *Server) handleGetUPNPStatus(w http.ResponseWriter, r *http.Request) {
	installed := false
	running := false
	statusDetail := ""
	if s.aclApplier != nil {
		if out, err := s.aclApplier.Run("systemctl", "is-active", "miniupnpd"); err == nil {
			running = out == "active"
			statusDetail = out
		} else {
			statusDetail = err.Error()
		}
		// upnpc presence = the client tooling is installed
		if _, err := s.aclApplier.Run("upnpc", "--version"); err == nil {
			installed = true
		}
	}
	active := 0
	if installed {
		if ms, err := upnp.ActiveMappings(s.aclApplier); err == nil {
			active = len(ms)
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"installed":    installed,
		"running":      running,
		"status":       statusDetail,
		"active_count": active,
	})
}
