package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/auth"
	"github.com/jimwong/router-re/mgmtd/internal/config"
	"github.com/jimwong/router-re/mgmtd/internal/firewall"
)

type Config struct {
	Addr      string
	SecretKey []byte
}

type Server struct {
	cfg   Config
	mux   *http.ServeMux
	store config.Store
	tm    *auth.TokenManager
}

func New(cfg Config) *Server {
	s := &Server{
		cfg:   cfg,
		mux:   http.NewServeMux(),
		store: config.NewMockStore(),
		tm:    auth.NewTokenManager(cfg.SecretKey, 15*time.Minute),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/v1/system/info", s.auth(s.handleSystemInfo))
	s.mux.HandleFunc("GET /api/v1/interfaces", s.auth(s.handleInterfaces))
	s.mux.HandleFunc("GET /api/v1/firewall/rules", s.auth(s.handleGetFirewallRules))
	s.mux.HandleFunc("POST /api/v1/firewall/rules", s.auth(s.handleSetFirewallRules))
}

// handleLogin authenticates a fixed dev account and issues a JWT session token.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	// Dev default account; production reads from store/DB.
	if req.Username != "admin" || req.Password != "admin123" {
		writeJSON(w, 401, errBody(10001, "invalid credentials", ""))
		return
	}
	tok, err := s.tm.Issue(req.Username)
	if err != nil {
		writeJSON(w, 500, errBody(10000, "token issue fail", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{"token": tok})
}

// handleSystemInfo returns basic router identity.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	hostname, _ := s.store.Get("system", "hostname")
	if hostname == "" {
		hostname = "router"
	}
	writeJSON(w, 200, map[string]interface{}{
		"hostname": hostname,
		"version":  "0.1.0",
	})
}

// handleInterfaces returns a mock list of network interfaces from the store.
func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces := []map[string]string{
		{"name": "lan", "ipaddr": "192.168.1.1"},
		{"name": "wan", "ipaddr": "dynamic"},
	}
	writeJSON(w, 200, map[string]interface{}{"interfaces": ifaces})
}

// handleGetFirewallRules returns the current firewall rules as rendered nftables.
func (s *Server) handleGetFirewallRules(w http.ResponseWriter, r *http.Request) {
	var rules []firewall.Rule
	for i := 0; ; i++ {
		chain, err := s.store.Get("firewall", itoa(i)+".chain")
		if err != nil || chain == "" {
			break
		}
		proto, _ := s.store.Get("firewall", itoa(i)+".proto")
		dport, _ := s.store.Get("firewall", itoa(i)+".dport")
		action, _ := s.store.Get("firewall", itoa(i)+".action")
		src, _ := s.store.Get("firewall", itoa(i)+".src")
		dst, _ := s.store.Get("firewall", itoa(i)+".dst")
		rules = append(rules, firewall.Rule{
			Chain: chain, Src: src, Dst: dst, Proto: proto, DPort: dport, Action: action,
		})
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": rules,
		"nft":   firewall.RenderNftables(rules),
	})
}

// handleSetFirewallRules stores the submitted rules in the store.
func (s *Server) handleSetFirewallRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rules []firewall.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
		return
	}
	for i, rule := range req.Rules {
		key := itoa(i)
		s.store.Set("firewall", key+".chain", rule.Chain)
		s.store.Set("firewall", key+".src", rule.Src)
		s.store.Set("firewall", key+".dst", rule.Dst)
		s.store.Set("firewall", key+".proto", rule.Proto)
		s.store.Set("firewall", key+".dport", rule.DPort)
		s.store.Set("firewall", key+".action", rule.Action)
	}
	writeJSON(w, 200, map[string]interface{}{
		"rules": req.Rules,
		"nft":   firewall.RenderNftables(req.Rules),
	})
}

// auth is a Bearer-token middleware guarding all endpoints except health/login.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(h, "Bearer ")
		if tok == "" {
			writeJSON(w, 401, errBody(10003, "unauthorized", "missing token"))
			return
		}
		if _, err := s.tm.Verify(tok); err != nil {
			writeJSON(w, 401, errBody(10003, "unauthorized", "invalid token"))
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func errBody(code int, msg, detail string) map[string]string {
	return map[string]string{"code": itoa(code), "message": msg, "detail": detail}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Run() error {
	return http.ListenAndServe(s.cfg.Addr, s.mux)
}
