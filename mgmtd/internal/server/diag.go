package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/diag"
	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

func (s *Server) handleDiagPing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host  string `json:"host"`
		Count int    `json:"count,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10140, "invalid body", err.Error()))
		return
	}
	if err := diag.ValidateHost(req.Host); err != nil {
		writeJSON(w, 422, errBody(10141, "invalid host", err.Error()))
		return
	}
	res, err := diag.Ping(execx.DefaultRunner{}, req.Host, req.Count)
	if err != nil {
		writeJSON(w, 500, errBody(10142, "ping failed", err.Error()))
		return
	}
	if rec, ok := w.(*statusRecorder); ok {
		rec.setAuditTarget(req.Host)
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleDiagTraceroute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host    string `json:"host"`
		MaxHops int    `json:"max_hops,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10140, "invalid body", err.Error()))
		return
	}
	if err := diag.ValidateHost(req.Host); err != nil {
		writeJSON(w, 422, errBody(10141, "invalid host", err.Error()))
		return
	}
	res, err := diag.Traceroute(execx.DefaultRunner{}, req.Host, req.MaxHops)
	if err != nil {
		writeJSON(w, 500, errBody(10143, "traceroute failed", err.Error()))
		return
	}
	if rec, ok := w.(*statusRecorder); ok {
		rec.setAuditTarget(req.Host)
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleDiagDNS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Server string `json:"server,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10140, "invalid body", err.Error()))
		return
	}
	if req.Server != "" {
		if err := diag.ValidateHost(req.Server); err != nil {
			writeJSON(w, 422, errBody(10141, "invalid server", err.Error()))
			return
		}
	}
	res, err := diag.DNSLookup(execx.DefaultRunner{}, req.Name, req.Server)
	if err != nil {
		writeJSON(w, 500, errBody(10144, "dns lookup failed", err.Error()))
		return
	}
	if rec, ok := w.(*statusRecorder); ok {
		rec.setAuditTarget(req.Name)
	}
	writeJSON(w, 200, res)
}

func (s *Server) handleDiagInterfaces(w http.ResponseWriter, r *http.Request) {
	rows, err := diag.InterfaceStats(execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(10145, "interface stats failed", err.Error()))
		return
	}
	if rows == nil {
		rows = []map[string]string{}
	}
	writeJSON(w, 200, map[string]interface{}{"interfaces": rows, "count": len(rows)})
}
