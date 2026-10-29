package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleGetLBStatusPerMemberTraffic(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
	cfg := `{"mode":"weighted","members":[{"name":"m1","iface":"eth0","gateway":"1.1.1.1","weight":1,"enabled":true}],"health":{"target":"8.8.8.8","interval_sec":5,"timeout_sec":2,"fail_threshold":2,"rise_threshold":2}}`
	_ = s.store.Set("lb", "config", cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lb/status", nil)
	rec := httptest.NewRecorder()
	s.handleGetLBStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["per_member_rx_tx"]; !ok {
		t.Fatalf("missing per_member_rx_tx: %s", rec.Body.String())
	}
	// field present (null or object) - not assertion failure
	_ = strings.Contains
}
