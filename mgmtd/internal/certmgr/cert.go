// Package certmgr 解析与生成 TLS 证书（PEM）。
package certmgr

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"
)

// CertInfo 是一张证书的摘要信息（不含私钥）。
type CertInfo struct {
	Subject    string   `json:"subject"`
	Issuer     string   `json:"issuer"`
	NotBefore  int64    `json:"not_before"`
	NotAfter   int64    `json:"not_after"`
	DNSNames   []string `json:"dns_names"`
	IsCA       bool     `json:"is_ca"`
	SelfSigned bool     `json:"self_signed"`
	KeyBits    int      `json:"key_bits"`
	SigAlg     string   `json:"sig_alg"`
}

// InspectPEM 解析 PEM 里的第一张证书，填 CertInfo。
// 支持 CERTIFICATE 块；解析失败返回明确错误（不返回零值）。
// SelfSigned 判据：Subject == Issuer（用 x509 的 RawSubject 字节比较，不比字符串）。
func InspectPEM(pemBytes []byte) (CertInfo, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return CertInfo{}, fmt.Errorf("no PEM block found")
	}
	if block.Type != "CERTIFICATE" {
		return CertInfo{}, fmt.Errorf("PEM block type is %q, want CERTIFICATE", block.Type)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return CertInfo{}, fmt.Errorf("parse certificate: %w", err)
	}
	ci := CertInfo{
		Subject:    cert.Subject.String(),
		Issuer:     cert.Issuer.String(),
		NotBefore:  cert.NotBefore.Unix(),
		NotAfter:   cert.NotAfter.Unix(),
		DNSNames:   cert.DNSNames,
		IsCA:       cert.IsCA,
		SelfSigned: bytesEqual(cert.RawSubject, cert.RawIssuer),
		SigAlg:     cert.SignatureAlgorithm.String(),
	}
	// KeyBits：RSA 密钥位数
	if pk, ok := cert.PublicKey.(*rsa.PublicKey); ok {
		ci.KeyBits = pk.N.BitLen()
	}
	if ci.DNSNames == nil {
		ci.DNSNames = []string{}
	}
	return ci, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DaysLeft 返回距离过期还有多少天（用 now 计算）。已过期返回负数。
func DaysLeft(c CertInfo, now int64) int {
	diff := c.NotAfter - now
	if diff >= 0 {
		return int(diff / 86400)
	}
	// 已过期：向上取整为负数
	return -int((-diff + 86399) / 86400)
}

// ErrExpiringSoon 表示证书即将过期（剩余 < 7 天）。
var ErrExpiringSoon = errors.New("certificate expiring soon (less than 7 days)")

// ErrExpired 表示证书已过期。
var ErrExpired = errors.New("certificate expired")

// Validate 检查证书可用性。
func Validate(c CertInfo, now int64) error {
	if now >= c.NotAfter {
		return fmt.Errorf("certificate expired at %d: %w", c.NotAfter, ErrExpired)
	}
	if DaysLeft(c, now) < 7 {
		return fmt.Errorf("certificate expiring soon: %w", ErrExpiringSoon)
	}
	if len(c.DNSNames) == 0 {
		// IP 证书可能没有 SAN，不算错，但告知调用方
		// (non-fatal hint; returned as nil to not block usage)
	}
	return nil
}

// GenerateSelfSigned 生成自签证书（用于本地 Web 管理）。
//
// hostnames 为空时用 "router-re.local"。
// 必须包含 SAN（DNSNames + 如果 hostnames 里有 IP 就加 IPAddresses）：
// 现代浏览器不认没有 SAN 的证书（CN 已被弃用作为主机名匹配依据）。
// 有效期 365 天，RSA 2048，返回 (certPEM, keyPEM)。
func GenerateSelfSigned(hostnames []string, now int64) (certPEM, keyPEM []byte, err error) {
	if len(hostnames) == 0 {
		hostnames = []string{"router-re.local"}
	}
	var dnsNames []string
	var ipAddrs []net.IP
	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate rsa key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: hostnames[0],
		},
		NotBefore:             time.Unix(now, 0),
		NotAfter:              time.Unix(now, 0).Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, nil
}
