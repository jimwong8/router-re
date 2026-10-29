package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHTBSmartApplyNoPlan(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/htb/smart-apply", bytes.NewBufferString(`{"confirm":true}`))
	rec := httptest.NewRecorder()
	s.handleHTBSmartApply(rec, req)
	if rec.Code != 422 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleHTBSmartApplyConfirmFalse(t *testing.T) {
	s := New(Config{
		Addr:          ":0",
		SecretKey:     []byte("0123456789abcdef0123456789abcdef"),
		AdminUser:     "admin",
		AdminPassword: "pw",
	})
	s.store.Set("htb", "smart-plan", `{"iface":"eth0","root_rate_mbit":100,"classes":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/htb/smart-apply", bytes.NewBufferString(`{"confirm":false}`))
	rec := httptest.NewRecorder()
	s.handleHTBSmartApply(rec, req)
	if rec.Code != 422 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Marshal
}
