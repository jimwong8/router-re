package ddns

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// sources to probe for public IP.
var publicIPSources = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://ipinfo.io/ip",
}

// isPublicIP rejects private/link-local/loopback/multicast ranges.
func isPublicIP(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	return true
}

// DetectPublicIP probes several services until one returns a public IPv4.
func DetectPublicIP(client *http.Client) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	var errs []string
	for _, src := range publicIPSources {
		resp, err := client.Get(src)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src, err))
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src, err))
			continue
		}
		ip := strings.TrimSpace(string(data))
		if !isPublicIP(ip) {
			errs = append(errs, fmt.Sprintf("%s: rejected non-public IP %q", src, ip))
			continue
		}
		return ip, nil
	}
	return "", fmt.Errorf("all public IP sources failed: %s", strings.Join(errs, "; "))
}
