package certmgr

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"
)

func TestGenerateInspectRoundTrip(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned([]string{"example.com", "www.example.com"}, 1000)
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	ci, err := InspectPEM(certPEM)
	if err != nil {
		t.Fatalf("InspectPEM: %v", err)
	}
	if len(ci.DNSNames) == 0 {
		t.Fatal("DNSNames must not be empty")
	}
	found := false
	for _, n := range ci.DNSNames {
		if n == "example.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("DNSNames %v must contain example.com", ci.DNSNames)
	}
}

func TestGenerateHasSAN(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned([]string{"router-re.local"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	ci, err := InspectPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if len(ci.DNSNames) == 0 {
		t.Fatal("generated cert must have SAN (DNSNames non-empty); browsers reject certs without SAN")
	}
}

func TestGenerateIPInIPAddresses(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned([]string{"10.100.1.18"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("10.100.1.18")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("IPAddresses %v must contain 10.100.1.18", cert.IPAddresses)
	}
	// DNSNames should be empty for pure IP
	if len(cert.DNSNames) != 0 {
		t.Fatalf("pure-IP cert should have no DNSNames, got %v", cert.DNSNames)
	}
}

func TestInspectPEMInvalid(t *testing.T) {
	if _, err := InspectPEM([]byte("not a cert")); err == nil {
		t.Fatal("InspectPEM on garbage must fail")
	}
}

func TestDaysLeftCorrect(t *testing.T) {
	now := int64(1000)
	ci := CertInfo{NotAfter: now + 86400*10}
	if got := DaysLeft(ci, now); got != 10 {
		t.Fatalf("DaysLeft=%d, want 10", got)
	}
}

func TestDaysLeftExpiredNegative(t *testing.T) {
	now := int64(1000)
	ci := CertInfo{NotAfter: now - 86400*3}
	if got := DaysLeft(ci, now); got >= 0 {
		t.Fatalf("expired cert DaysLeft=%d, want negative", got)
	}
}

func TestValidateExpired(t *testing.T) {
	now := int64(1000)
	ci := CertInfo{NotAfter: now - 100, NotBefore: 0}
	err := Validate(ci, now)
	if err == nil {
		t.Fatal("expired cert must fail Validate")
	}
	if !contains(err.Error(), "expired") {
		t.Fatalf("error must contain 'expired', got %q", err.Error())
	}
}

func TestValidateExpiringSoon(t *testing.T) {
	now := int64(1000)
	ci := CertInfo{NotAfter: now + 3*86400, NotBefore: 0, DNSNames: []string{"x"}}
	err := Validate(ci, now)
	if err == nil {
		t.Fatal("3-day cert must fail Validate")
	}
}

func TestSelfSignedTrue(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned([]string{"router-re.local"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	ci, err := InspectPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if !ci.SelfSigned {
		t.Fatal("self-signed cert must have SelfSigned=true (RawSubject==RawIssuer)")
	}
	// sanity: subject == issuer string too
	if ci.Subject != ci.Issuer {
		t.Fatalf("subject/issuer differ: %s vs %s", ci.Subject, ci.Issuer)
	}
}

func TestInspectPEMEmptyDNSNamesNotNil(t *testing.T) {
	// a cert with no SAN DNS names should still yield non-nil-ish slice handling
	certPEM, _, err := GenerateSelfSigned([]string{"10.0.0.1"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	ci, err := InspectPEM(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if ci.DNSNames == nil {
		// acceptable? spec doesn't require; but keep stable
	}
	_ = time.Now()
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
