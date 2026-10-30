package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/firmware"
)

// makeFirmwareFile creates a >=1MB file whose magic is "ustar" so that
// firmware.InspectImage + firmware.ValidateImage both succeed (size>=1MB,
// magic==tar). Returns (path, sha256).
func makeFirmwareFile(t *testing.T) (string, string) {
	t.Helper()
	f, err := os.CreateTemp("", "p38-firmware-")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	buf := make([]byte, 1024*1024+512)
	copy(buf[257:], []byte("ustar"))
	if _, err := f.Write(buf); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	img, err := firmware.InspectImage(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatalf("InspectImage: %v", err)
	}
	if img.SHA256 == "" {
		os.Remove(f.Name())
		t.Fatal("InspectImage returned empty SHA256")
	}
	return f.Name(), img.SHA256
}

// TestP38FirmwareConfirmShaMatch：正向路径 —— sha256 匹配时应返回 200 且响应含 verified 字段
func TestP38FirmwareConfirmShaMatch(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)

	path, sha := makeFirmwareFile(t)
	defer os.Remove(path)

	body := `{"path":"` + path + `","sha256":"` + sha + `","board":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/firmware/confirm", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (sha match), got %d: %s", rr.Code, rr.Body.String())
	}
	dec := json.NewDecoder(rr.Body)
	dec.UseNumber()
	var resp map[string]interface{}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["verified"]; !ok {
		t.Fatalf("response missing 'verified' field: %v", resp)
	}
	if c, ok := resp["code"]; ok {
		if strings.TrimSpace(toStr(c)) == "10500" {
			t.Fatalf("sha mismatch code 10500 leaked into success path: %v", resp)
		}
	}
}

// TestP38FirmwareConfirmBoardChecked：board_checked 字段必须是 bool 类型
func TestP38FirmwareConfirmBoardChecked(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)

	path, sha := makeFirmwareFile(t)
	defer os.Remove(path)

	body := `{"path":"` + path + `","sha256":"` + sha + `","board":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/firmware/confirm", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	dec := json.NewDecoder(rr.Body)
	dec.UseNumber()
	var resp map[string]interface{}
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	v, ok := resp["board_checked"]
	if !ok {
		t.Fatalf("response missing 'board_checked' field: %v", resp)
	}
	if _, isBool := v.(bool); !isBool {
		t.Fatalf("board_checked must be bool, got %T (%v)", v, v)
	}
}

// TestP38ConntrackAcctGetFields：GET /api/v1/conntrack/acct 结构完整性
func TestP38ConntrackAcctGetFields(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)
	req := httptest.NewRequest("GET", "/api/v1/conntrack/acct", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	dec := json.NewDecoder(rr.Body)
	dec.UseNumber()
	var body map[string]interface{}
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["on"].(bool); !ok {
		t.Fatalf("'on' must be bool, got %T (%v)", body["on"], body["on"])
	}
	if _, ok := body["path"].(string); !ok {
		t.Fatalf("'path' must be string, got %T (%v)", body["path"], body["path"])
	}
	if _, ok := body["persisted"].(bool); !ok {
		t.Fatalf("'persisted' must be bool, got %T (%v)", body["persisted"], body["persisted"])
	}
}

// TestP38TerminalsFields：GET /api/v1/terminals 结构完整性
func TestP38TerminalsFields(t *testing.T) {
	s := New(Config{Addr: ":8080", SecretKey: []byte("0123456789abcdef0123456789abcdef")})
	token := getAdminToken(t, s)
	req := httptest.NewRequest("GET", "/api/v1/terminals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	dec := json.NewDecoder(rr.Body)
	dec.UseNumber()
	var body map[string]interface{}
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	cn, ok := body["count"].(json.Number)
	if !ok {
		t.Fatalf("'count' must be number, got %T (%v)", body["count"], body["count"])
	}
	if _, err := cn.Int64(); err != nil {
		t.Fatalf("'count' must be int, got %s: %v", cn, err)
	}
	terminals, ok := body["terminals"].([]interface{})
	if !ok {
		t.Fatalf("'terminals' must be array, got %T (%v)", body["terminals"], body["terminals"])
	}
	for i, e := range terminals {
		m, ok := e.(map[string]interface{})
		if !ok {
			t.Fatalf("terminals[%d] not object: %T", i, e)
		}
		for _, k := range []string{"ip", "mac", "hostname", "iface", "source", "online"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("terminals[%d] missing key %s: %v", i, k, m)
			}
		}
	}
}
