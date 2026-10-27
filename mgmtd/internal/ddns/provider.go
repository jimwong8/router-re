package ddns

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Provider updates a DNS record to point at ip.
type Provider interface {
	Name() string
	Update(cfg Config, ip string) error
}

// Config describes DDNS provider settings.
type Config struct {
	Provider  string `json:"provider"`
	Domain    string `json:"domain"`
	Subdomain string `json:"subdomain"`
	Token     string `json:"token"`
	RecordID  string `json:"record_id"`
	ZoneID    string `json:"zone_id"`
	Enabled   bool   `json:"enabled"`
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// CloudflareProvider implements the Cloudflare v4 DNS API.
type CloudflareProvider struct {
	// baseURL overrides the API endpoint (tests). Default: official.
	baseURL string
}

func (CloudflareProvider) Name() string { return "cloudflare" }

func (p CloudflareProvider) Update(cfg Config, ip string) error {
	if cfg.ZoneID == "" || cfg.RecordID == "" || cfg.Token == "" {
		return fmt.Errorf("cloudflare requires zone_id, record_id and token")
	}
	base := p.baseURL
	if base == "" {
		base = "https://api.cloudflare.com/client/v4"
	}
	body, _ := json.Marshal(map[string]interface{}{
		"type":    "A",
		"name":    cfg.Subdomain,
		"content": ip,
		"ttl":     120,
	})
	u := fmt.Sprintf("%s/zones/%s/dns_records/%s", base, url.PathEscape(cfg.ZoneID), url.PathEscape(cfg.RecordID))
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Success bool          `json:"success"`
		Errors  []interface{} `json:"errors"`
	}
	_ = json.Unmarshal(data, &parsed)
	if !parsed.Success {
		errs, _ := json.Marshal(parsed.Errors)
		return fmt.Errorf("cloudflare update failed: %s", string(errs))
	}
	return nil
}

// AliDNSProvider implements Aliyun DNS via signed HTTP API (simplified).
type AliDNSProvider struct{}

func (AliDNSProvider) Name() string { return "alidns" }

// AliDNSSign signs Aliyun RPC-style params per RFC 2104 (HMAC-SHA1).
func AliDNSSign(secret, method string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(url.QueryEscape(k))
		b.WriteString("=")
		b.WriteString(url.QueryEscape(params[k]))
		b.WriteString("&")
	}
	qs := strings.TrimSuffix(b.String(), "&")
	// canonical: HTTPMethod & percentEncode("/") & percentEncode(canonicalizedQueryString)
	canonical := method + "&" + url.QueryEscape("/") + "&" + url.QueryEscape(qs)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(canonical))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (p AliDNSProvider) Update(cfg Config, ip string) error {
	return alidnsUpdate(aliDNSEndpoint, cfg, ip)
}

// aliDNSEndpoint is overrideable for tests (default official).
var aliDNSEndpoint = "https://alidns.aliyuncs.com/"

func alidnsUpdate(base string, cfg Config, ip string) error {
	if cfg.Domain == "" || cfg.Token == "" || cfg.RecordID == "" {
		return fmt.Errorf("alidns requires domain, token (AccessKeySecret) and record_id")
	}
	params := map[string]string{
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      cfg.Subdomain, // reuse subdomain as AKID placeholder
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"Action":           "UpdateDomainRecord",
		"RecordId":         cfg.RecordID,
		"RR":               cfg.Subdomain,
		"Type":             "A",
		"Value":            ip,
	}
	sig := AliDNSSign(cfg.Token, "POST", params)
	params["Signature"] = sig
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequest(http.MethodPost, base, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	_ = json.Unmarshal(data, &parsed)
	if parsed.Code != "" && parsed.Code != "OK" {
		return fmt.Errorf("alidns update failed: %s %s", parsed.Code, parsed.Message)
	}
	return nil
}
