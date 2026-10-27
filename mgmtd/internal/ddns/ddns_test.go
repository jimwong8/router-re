package ddns

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareUpdateSuccess(t *testing.T) {
	var gotMethod, gotAuth, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.URL.Path, "/zones/zone1/dns_records/rec1") {
			t.Errorf("wrong URL path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"result":{}}`))
	}))
	defer ts.Close()
	p := CloudflareProvider{baseURL: ts.URL}
	cfg := Config{ZoneID: "zone1", RecordID: "rec1", Token: "tok123", Subdomain: "home.example.com"}
	if err := p.Update(cfg, "203.0.113.5"); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if gotMethod != "PUT" {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("expected Bearer tok123, got %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"content":"203.0.113.5"`) || !strings.Contains(gotBody, `"type":"A"`) {
		t.Errorf("bad body: %s", gotBody)
	}
}

func TestCloudflareUpdateFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"errors":[{"code":1003,"message":"Invalid zone"}]}`))
	}))
	defer ts.Close()
	p := CloudflareProvider{baseURL: ts.URL}
	cfg := Config{ZoneID: "z", RecordID: "r", Token: "t", Subdomain: "x.example.com"}
	if err := p.Update(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected error on success=false")
	} else {
		if !strings.Contains(err.Error(), "Invalid zone") {
			t.Errorf("error should mention provider message, got %v", err)
		}
	}
}

func TestAliDNSSign(t *testing.T) {
	params := map[string]string{"Action": "UpdateDomainRecord", "Format": "JSON"}
	sig := AliDNSSign("mysecret", "POST", params)
	if sig == "" {
		t.Fatal("signature must be non-empty")
	}
	// deterministic: re-run yields same
	if sig != AliDNSSign("mysecret", "POST", params) {
		t.Fatal("signature not deterministic")
	}
}

func TestAliDNSUpdateSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("Action") != "UpdateDomainRecord" {
			t.Errorf("wrong Action: %s", r.Form.Get("Action"))
		}
		if r.Form.Get("Value") != "203.0.113.5" {
			t.Errorf("wrong Value: %s", r.Form.Get("Value"))
		}
		if r.Form.Get("Signature") == "" {
			t.Error("missing Signature param")
		}
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	old := aliDNSEndpoint
	aliDNSEndpoint = ts.URL
	defer func() { aliDNSEndpoint = old }()
	cfg := Config{Domain: "example.com", Token: "secret", RecordID: "rid", Subdomain: "home"}
	if err := (AliDNSProvider{}).Update(cfg, "203.0.113.5"); err != nil {
		t.Fatalf("alidns update failed: %v", err)
	}
}

func TestAliDNSUpdateFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Code":"InvalidAccessKeyId","Message":"specified access key is not found"}`))
	}))
	defer ts.Close()
	old := aliDNSEndpoint
	aliDNSEndpoint = ts.URL
	defer func() { aliDNSEndpoint = old }()
	cfg := Config{Domain: "example.com", Token: "secret", RecordID: "rid", Subdomain: "home"}
	if err := (AliDNSProvider{}).Update(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected error on failure Code")
	} else if !strings.Contains(err.Error(), "InvalidAccessKeyId") {
		t.Errorf("error should include Code, got %v", err)
	}
}

func TestDetectRejectPrivateIP(t *testing.T) {
	if isPublicIP("192.168.1.1") {
		t.Fatal("private IP must be rejected")
	}
	if isPublicIP("127.0.0.1") {
		t.Fatal("loopback must be rejected")
	}
	if !isPublicIP("203.0.113.1") {
		t.Fatal("public IP must be accepted")
	}
}

func TestRenderTimesyncd(t *testing.T) {
	cfg := NTPConfig{Servers: []string{"time.google.com", "pool.ntp.org"}, Enabled: true}
	r := RenderTimesyncd(cfg)
	if !strings.Contains(r, "NTP=time.google.com pool.ntp.org") {
		t.Errorf("rendered conf missing NTP line: %q", r)
	}
}
