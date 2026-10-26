package wireless

import "testing"

func TestRenderHostapd(t *testing.T) {
	s := SSID{Name: "router-net", Security: "wpa2", Password: "secret123", VLAN: 10, Enabled: true}
	out := RenderHostapd(s)
	if !containsString(out, "ssid=") {
		t.Errorf("expected 'ssid=' in hostapd config; got:\n%s", out)
	}
	if !containsString(out, "wpa_passphrase=") {
		t.Errorf("expected 'wpa_passphrase=' in hostapd config; got:\n%s", out)
	}
}

func TestRenderWpaSupplicant(t *testing.T) {
	s := SSID{Name: "router-net", Security: "wpa2", Password: "secret123", Enabled: true}
	out := RenderWpaSupplicant(s)
	if !containsString(out, "ssid=") {
		t.Errorf("expected 'ssid=' in wpa_supplicant config; got:\n%s", out)
	}
	if !containsString(out, "psk=") {
		t.Errorf("expected 'psk=' in wpa_supplicant config; got:\n%s", out)
	}
}

func containsString(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > len(sub) && (s[:len(sub)] == sub || s[len(s)-len(sub):] == sub || (len(s) > len(sub) && findIn(s, sub))))
}

func findIn(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
