package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/cfgsync"
	"github.com/jimwong/router-re/mgmtd/internal/config"
)

// failingStore wraps a MockStore and fails Set on a specific section.
type failingStore struct {
	config.Store
	failSection string
}

func (f *failingStore) Set(section, option, value string) error {
	if section == f.failSection {
		return &storeError{"injected failure for section " + section}
	}
	return f.Store.Set(section, option, value)
}

type storeError struct{ msg string }

func (e *storeError) Error() string { return e.msg }

func TestRestoreRollback(t *testing.T) {
	// 1. Create server with MockStore
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})

	// 2. Replace store with failing store (fails on "firewall" section)
	mock := config.NewMockStore()
	// Pre-populate some config
	mock.Set("network", "config", `{"iface":"eth0"}`)
	mock.Set("firewall", "config", `{"rules":[]}`)
	s.store = &failingStore{Store: mock, failSection: "firewall"}

	// 3. Record baseline
	baselineNet, _ := s.store.Get("network", "config")
	baselineFW, _ := s.store.Get("firewall", "config")

	// 4. Build a bundle with network + firewall sections
	sections := map[string]json.RawMessage{
		"network.config":  json.RawMessage(`{"iface":"eth1"}`),
		"firewall.config": json.RawMessage(`{"rules":[{"id":1}]}`),
	}
	bundle := cfgsync.NewBundle("testhost", 12345, sections)

	// 5. Call handleRestoreConfig
	token := getAdminToken(t, s)
	body := `{"confirm":true,"bundle":` + mustMarshal(bundle) + `}`
	req := httptest.NewRequest("POST", "/api/v1/config/restore", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	// 6. Assert non-200
	if rr.Code == http.StatusOK {
		t.Fatalf("expected non-200, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("HTTP status: %d, body: %s", rr.Code, rr.Body.String())

	// 7. Assert config rolled back to baseline
	afterNet, _ := s.store.Get("network", "config")
	afterFW, _ := s.store.Get("firewall", "config")

	if afterNet != baselineNet {
		t.Errorf("network config not rolled back: before=%s after=%s", baselineNet, afterNet)
	}
	if afterFW != baselineFW {
		t.Errorf("firewall config not rolled back: before=%s after=%s", baselineFW, afterFW)
	}

	// 8. Assert response body does not contain "restored"
	if strings.Contains(rr.Body.String(), `"restored"`) {
		t.Errorf("response body should not contain 'restored' on failure: %s", rr.Body.String())
	}
}

func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}