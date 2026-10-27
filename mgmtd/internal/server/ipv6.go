package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/ipv6"
)

func (s *Server) handleGetIPv6Addresses(w http.ResponseWriter, r *http.Request) {
	addrs, err := ipv6.ListAddr(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10130, "ipv6 addr list failed", err.Error()))
		return
	}
	if addrs == nil {
		addrs = []ipv6.Address{}
	}
	writeJSON(w, 200, map[string]interface{}{"addresses": addrs})
}

func (s *Server) handleSetIPv6Addresses(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Addresses []ipv6.Address `json:"addresses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10131, "invalid body", err.Error()))
		return
	}
	if len(req.Addresses) == 0 {
		writeJSON(w, 422, errBody(10132, "empty list", "POST /ipv6/addresses must carry at least one address"))
		return
	}
	for _, a := range req.Addresses {
		if err := ipv6.ValidateAddr(a); err != nil {
			writeJSON(w, 422, errBody(10133, "validation failed", err.Error()))
			return
		}
		if err := ipv6.ApplyAddr(execx.DefaultRunner{}, a); err != nil {
			writeJSON(w, 500, errBody(10134, "ipv6 addr apply failed", err.Error()))
			return
		}
	}
	// read back actual state to confirm the addresses really exist
	actual, _ := ipv6.ListAddr(execx.DefaultRunner{})
	if actual == nil {
		actual = []ipv6.Address{}
	}
	writeJSON(w, 200, map[string]interface{}{"applied": true, "addresses": actual})
}

func (s *Server) handleDeleteIPv6Address(w http.ResponseWriter, r *http.Request) {
	var a ipv6.Address
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, 400, errBody(10131, "invalid body", err.Error()))
		return
	}
	if err := ipv6.ValidateAddr(a); err != nil {
		writeJSON(w, 422, errBody(10133, "validation failed", err.Error()))
		return
	}
	if err := ipv6.DeleteAddr(execx.DefaultRunner{}, a); err != nil {
		writeJSON(w, 500, errBody(10135, "ipv6 addr delete failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"deleted": true, "addr": a.Addr, "iface": a.Iface})
}

func (s *Server) handleGetIPv6RA(w http.ResponseWriter, r *http.Request) {
	cfg := ipv6.RAConfig{}
	if raw, _ := s.store.Get("ipv6", "ra"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg, "radvd": ipv6.RenderRadvd(cfg)})
}

func (s *Server) handleSetIPv6RA(w http.ResponseWriter, r *http.Request) {
	var cfg ipv6.RAConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10131, "invalid body", err.Error()))
		return
	}
	if err := ipv6.ValidateRA(cfg); err != nil {
		writeJSON(w, 422, errBody(10136, "validation failed", err.Error()))
		return
	}
	b, _ := json.Marshal(cfg)
	_ = s.store.Set("ipv6", "ra", string(b))
	applied := false
	detail := ""
	if s.aclApplier != nil {
		if err := ipv6.ApplyRA(execx.DefaultRunner{}, cfg, "/etc/radvd.conf"); err != nil {
			detail = err.Error()
		} else {
			applied = true
			detail = "radvd restarted with new config"
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"applied": applied,
		"config":  cfg,
		"radvd":   ipv6.RenderRadvd(cfg),
		"detail":  detail,
	})
}

func (s *Server) handleGetIPv6Status(w http.ResponseWriter, r *http.Request) {
	st, err := ipv6.Status(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10137, "ipv6 status failed", err.Error()))
		return
	}
	writeJSON(w, 200, st)
}
