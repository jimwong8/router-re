package ovpn

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewCASelfSigned(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	cert := ca.Certificate()
	if cert == nil {
		t.Fatal("CA cert is nil")
	}
	if !cert.IsCA {
		t.Fatal("CA cert IsCA must be true")
	}
	if cert.Subject.CommonName == "" {
		t.Fatal("CA CN empty")
	}
	if cert.Subject.String() != cert.Issuer.String() {
		t.Fatalf("CA not self-signed: subject=%s issuer=%s", cert.Subject, cert.Issuer)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("CA self-verify: %v", err)
	}
	now := time.Now()
	if cert.NotAfter.Before(now.AddDate(9, 0, 0)) {
		t.Fatalf("CA expires too soon: %v", cert.NotAfter)
	}
	if cert.NotAfter.After(now.AddDate(11, 0, 0)) {
		t.Fatalf("CA expires too late: %v", cert.NotAfter)
	}
}

func TestIssueServerCNAndExpiry(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, err := ca.IssueServer("vpn-server", []string{"vpn.example.com"}, []string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("IssueServer: %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("empty pem output")
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("bad cert pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "vpn-server" {
		t.Fatalf("CN mismatch: got %s", cert.Subject.CommonName)
	}
	foundDNS := false
	for _, d := range cert.DNSNames {
		if d == "vpn.example.com" {
			foundDNS = true
		}
	}
	if !foundDNS {
		t.Fatalf("DNSNames missing: %v", cert.DNSNames)
	}
	foundIP := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("10.0.0.1")) {
			foundIP = true
		}
	}
	if !foundIP {
		t.Fatalf("IPAddresses missing: %v", cert.IPAddresses)
	}
	now := time.Now()
	if cert.NotAfter.Before(now.AddDate(2, 0, 0)) {
		t.Fatalf("server cert expires too soon: %v", cert.NotAfter)
	}
	if cert.NotAfter.After(now.AddDate(4, 0, 0)) {
		t.Fatalf("server cert expires too late: %v", cert.NotAfter)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Certificate())
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("server cert verify: %v", err)
	}
	kblock, _ := pem.Decode(keyPEM)
	if kblock == nil {
		t.Fatal("bad key pem")
	}
}

func TestIssueClientCN(t *testing.T) {
	ca, _ := NewCA()
	certPEM, _, err := ca.IssueClient("alice")
	if err != nil {
		t.Fatalf("IssueClient: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)
	if cert.Subject.CommonName != "alice" {
		t.Fatalf("CN mismatch: got %s", cert.Subject.CommonName)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Certificate())
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("client cert verify: %v", err)
	}
	hasClient := false
	for _, e := range cert.ExtKeyUsage {
		if e == x509.ExtKeyUsageClientAuth {
			hasClient = true
		}
	}
	if !hasClient {
		t.Fatal("client cert missing ExtKeyUsageClientAuth")
	}
}

func TestLoadOrCreateNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	ca1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	cert1 := ca1.Certificate()
	serial1 := cert1.SerialNumber.String()

	if _, _, err := ca1.IssueClient("bob"); err != nil {
		t.Fatal(err)
	}

	ca2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	cert2 := ca2.Certificate()
	if cert2.SerialNumber.String() != serial1 {
		t.Fatalf("CA overwritten: serial %s -> %s", serial1, cert2.SerialNumber.String())
	}
	if string(ca1.Certificate().Raw) != string(ca2.Certificate().Raw) {
		t.Fatal("CA cert changed after reload")
	}
	found := false
	for _, c := range ca2.List() {
		if c.CN == "bob" {
			found = true
		}
	}
	if !found {
		t.Fatalf("List missing bob: %+v", ca2.List())
	}
}

func TestLoadOrCreateKeyPermission(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ca.IssueServer("s", nil, nil); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "issued", "*.key"))
	if len(matches) == 0 {
		t.Fatal("no key file written")
	}
	fi, err := os.Stat(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key perm = %o, want 0600", fi.Mode().Perm())
	}
	fi2, _ := os.Stat(filepath.Join(dir, "ca.key"))
	if fi2.Mode().Perm() != 0o600 {
		t.Fatalf("ca.key perm = %o, want 0600", fi2.Mode().Perm())
	}
}

// TestCNValidationRejectsInjection CN 会进配置与文件路径，带换行/斜杠必须在入口拦下。
func TestCNValidationRejectsInjection(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	bad := []string{
		"",
		"alice\npush \"route 0.0.0.0 0.0.0.0\"",
		"../../etc/passwd",
		"a/b",
		"has space",
		strings.Repeat("x", 65),
	}
	for _, cn := range bad {
		if _, _, err := ca.IssueClient(cn); err == nil {
			t.Fatalf("CN %q 应被拒绝", cn)
		}
	}
	for _, cn := range []string{"alice", "bob-01", "site_2.a"} {
		if _, _, err := ca.IssueClient(cn); err != nil {
			t.Fatalf("合法 CN %q 不该被拒: %v", cn, err)
		}
	}
}
