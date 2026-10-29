package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/htb"
)

func (s *Server) handleGetHTBSmartPlan(w http.ResponseWriter, r *http.Request) {
	iface := r.URL.Query().Get("iface")
	rootStr := r.URL.Query().Get("root_mbit")
	root, err := strconv.ParseFloat(rootStr, 64)
	if err != nil {
		writeJSON(w, 422, errBody(11017, "invalid root_mbit", err.Error()))
		return
	}
	// collect terminals from arp
	terminals := []string{}
	data, err := os.ReadFile("/proc/net/arp")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 6 {
				continue
			}
			ip := fields[0]
			// basic validity
			if ip == "" {
				continue
			}
			terminals = append(terminals, ip)
		}
	}
	if len(terminals) == 0 {
		writeJSON(w, 422, errBody(11017, "no terminals", "no terminals found"))
		return
	}
	cfg, err := htb.PlanSmart(root, iface, terminals)
	if err != nil {
		writeJSON(w, 422, errBody(11017, "plan failed", err.Error()))
		return
	}
	if b, merr := json.Marshal(cfg); merr == nil {
		s.store.Set("htb", "smart-plan", string(b))
	}
	writeJSON(w, 200, map[string]interface{}{"config": cfg})
}
