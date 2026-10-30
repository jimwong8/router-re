package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/ha"
)

// haState holds runtime HA state.
type haState struct {
	mu         sync.Mutex
	PeerURL    string
	Secret     string
	SyncKeys   []string
	LastSync   time.Time
	LastResult string
	LastErr    string
}

var globalHAState = &haState{}

// getHAConfig returns the current HA config.
func getHAConfig() ha.Config {
	globalHAState.mu.Lock()
	defer globalHAState.mu.Unlock()
	return ha.Config{
		PeerURL:  globalHAState.PeerURL,
		Secret:   globalHAState.Secret,
		SyncKeys: globalHAState.SyncKeys,
	}
}

// handleGetHAConfig returns HA configuration.
func (s *Server) handleGetHAConfig(w http.ResponseWriter, r *http.Request) {
	cfg := getHAConfig()
	writeJSON(w, 200, map[string]interface{}{
		"peer_url":   cfg.PeerURL,
		"has_secret": cfg.Secret != "",
		"secret":     ha.MaskedSecret(cfg.Secret),
		"sync_keys":  cfg.SyncKeys,
	})
}

// handleSetHAConfig saves HA configuration.
func (s *Server) handleSetHAConfig(w http.ResponseWriter, r *http.Request) {
	var cfg ha.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, errBody(10995, "invalid body", err.Error()))
		return
	}
	globalHAState.mu.Lock()
	globalHAState.PeerURL = cfg.PeerURL
	if cfg.Secret != "" {
		globalHAState.Secret = cfg.Secret
	}
	if cfg.SyncKeys != nil {
		globalHAState.SyncKeys = cfg.SyncKeys
	}
	globalHAState.mu.Unlock()
	writeJSON(w, 200, map[string]interface{}{"status": "ok"})
}

// handleHASync pushes local config to peer.
func (s *Server) handleHASync(w http.ResponseWriter, r *http.Request) {
	cfg := getHAConfig()
	if cfg.PeerURL == "" {
		writeJSON(w, 400, errBody(10996, "no peer", "peer_url not configured"))
		return
	}
	// Export local config
	data, err := ha.Export(s.store, cfg.SyncKeys)
	if err != nil {
		writeJSON(w, 500, errBody(10997, "export failed", err.Error()))
		return
	}
	// Push to peer
	body, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", cfg.PeerURL+"/api/v1/ha/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HA-Secret", cfg.Secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		globalHAState.mu.Lock()
		globalHAState.LastSync = time.Now()
		globalHAState.LastResult = "failed"
		globalHAState.LastErr = err.Error()
		globalHAState.mu.Unlock()
		writeJSON(w, 502, errBody(10998, "peer unreachable", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		globalHAState.mu.Lock()
		globalHAState.LastSync = time.Now()
		globalHAState.LastResult = "failed"
		globalHAState.LastErr = "peer returned " + resp.Status
		globalHAState.mu.Unlock()
		writeJSON(w, 502, errBody(10999, "peer error", resp.Status))
		return
	}
	globalHAState.mu.Lock()
	globalHAState.LastSync = time.Now()
	globalHAState.LastResult = "ok"
	globalHAState.LastErr = ""
	globalHAState.mu.Unlock()
	writeJSON(w, 200, map[string]interface{}{"status": "ok"})
}

// handleHAImport receives config from peer.
func (s *Server) handleHAImport(w http.ResponseWriter, r *http.Request) {
	secret := r.Header.Get("X-HA-Secret")
	cfg := getHAConfig()
	if !ha.ValidateSecret(cfg.Secret, secret) {
		writeJSON(w, 401, errBody(11000, "unauthorized", "secret mismatch"))
		return
	}
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, 400, errBody(11001, "invalid body", err.Error()))
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "merge"
	}
	if err := ha.Import(s.store, payload, mode, cfg.SyncKeys); err != nil {
		writeJSON(w, 400, errBody(11002, "import failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"status": "ok"})
}

// handleHAStatus returns sync status.
func (s *Server) handleHAStatus(w http.ResponseWriter, r *http.Request) {
	globalHAState.mu.Lock()
	defer globalHAState.mu.Unlock()
	writeJSON(w, 200, map[string]interface{}{
		"peer_url":    globalHAState.PeerURL,
		"last_sync":   globalHAState.LastSync,
		"last_result": globalHAState.LastResult,
		"last_error":  globalHAState.LastErr,
	})
}
