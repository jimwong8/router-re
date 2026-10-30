// Package ovpn 实现 OpenVPN 服务端的证书体系（PKI），纯 crypto/x509，不调用 openssl。
//
// 提供自签 CA 的生成/加载，以及基于 CA 签发服务端/客户端证书的能力。
// 私钥仅以 PEM 形式写入磁盘（权限 0600），绝不进入日志。
package ovpn

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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	caCertName = "ca.crt"
	caKeyName  = "ca.key"
	issuedDir  = "issued"
)

// ErrNoCA 表示目录中不存在 CA 证书/私钥。
var ErrNoCA = errors.New("ovpn: no CA found in directory")

// CA 是一个自签证书颁发机构，持有 CA 证书与私钥，并跟踪已签发的证书。
type CA struct {
	dir  string
	cert *x509.Certificate
	key  *rsa.PrivateKey

	// 内存中跟踪的已签发证书（LoadOrCreate 会从磁盘重建）。
	issued []IssuedCert
}

// IssuedCert 是一张已签发证书的摘要信息（不含私钥）。
type IssuedCert struct {
	CN       string    `json:"cn"`
	Serial   string    `json:"serial"`
	NotAfter time.Time `json:"not_after"`
	CertPath string    `json:"-"`
	KeyPath  string    `json:"-"`
}

// Certificate 返回 CA 的 x509 证书。
func (ca *CA) Certificate() *x509.Certificate { return ca.cert }

// Dir 返回 CA 所在目录（NewCA 生成的无目录 CA 返回空字符串）。
func (ca *CA) Dir() string { return ca.dir }

// CACertPEM 返回 CA 证书的 PEM 编码。
func (ca *CA) CACertPEM() []byte {
	if ca.cert == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
}

// CAKeyPEM 返回 CA 私钥的 PEM 编码（PKCS#1，权限由调用方保证）。
func (ca *CA) CAKeyPEM() []byte {
	if ca.key == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(ca.key)})
}

// cnPattern 限定 CN 只能由这些字符组成。
//
// 为什么必须卡死：CN 会直接进入客户端配置文件名、server.conf 的 client-config-dir、
// 以及 CRL 的序列化路径。一个带换行的 CN 就能往配置文件里塞一行指令
// （比如 push "route ..." 甚至 script-security），这是配置注入而不是理论风险。
var cnPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// ValidateCN 校验通用名。导出给 API 层复用，避免只在签发时校验、
// 而查询/删除路径拿未校验的 CN 去拼路径。
func ValidateCN(cn string) error {
	if !cnPattern.MatchString(cn) {
		return fmt.Errorf("ovpn: 通用名 %q 非法（只允许字母数字与 _ . -，1-64 位）", cn)
	}
	return nil
}

// NewCA 生成一个新的自签 CA（RSA 2048，CA:true，有效期 10 年）。
func NewCA() (*CA, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("ovpn: generate ca key: %w", err)
	}

	now := time.Now().UTC()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ovpn: generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "router-re OpenVPN CA",
			Organization: []string{"router-re"},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("ovpn: create ca cert: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("ovpn: parse ca cert: %w", err)
	}

	return &CA{cert: cert, key: key}, nil
}

// IssueServer 签发一张服务端证书（有效期 3 年），写入 PEM 文件（若 CA 有目录）。
// ips 可为 []string 或 []net.IP（或 nil）。
func (ca *CA) IssueServer(commonName string, dnsNames []string, ips interface{}) (certPEM, keyPEM []byte, err error) {
	if ca == nil || ca.cert == nil || ca.key == nil {
		return nil, nil, errors.New("ovpn: CA not initialized")
	}
	if err := ValidateCN(commonName); err != nil {
		return nil, nil, err
	}
	ipAddrs, err := toIPs(ips)
	if err != nil {
		return nil, nil, err
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: generate server key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: generate serial: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: now,
		NotAfter:  now.AddDate(3, 0, 0),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: create server cert: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: parse server cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	entry := IssuedCert{
		CN:       cert.Subject.CommonName,
		Serial:   cert.SerialNumber.String(),
		NotAfter: cert.NotAfter,
	}

	// 写入磁盘（若 CA 有目录）。
	if ca.dir != "" {
		if err := os.MkdirAll(filepath.Join(ca.dir, issuedDir), 0o755); err != nil {
			return nil, nil, fmt.Errorf("ovpn: mkdir issued: %w", err)
		}
		base := sanitizeCN(commonName) + "-" + cert.SerialNumber.String()
		certPath := filepath.Join(ca.dir, issuedDir, base+".crt")
		keyPath := filepath.Join(ca.dir, issuedDir, base+".key")
		if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
			return nil, nil, fmt.Errorf("ovpn: write cert: %w", err)
		}
		if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
			return nil, nil, fmt.Errorf("ovpn: write key: %w", err)
		}
		entry.CertPath = certPath
		entry.KeyPath = keyPath
	}
	ca.issued = append(ca.issued, entry)
	return certPEM, keyPEM, nil
}

// IssueClient 签发一张客户端证书（有效期 3 年）。
func (ca *CA) IssueClient(commonName string) (certPEM, keyPEM []byte, err error) {
	if ca == nil || ca.cert == nil || ca.key == nil {
		return nil, nil, errors.New("ovpn: CA not initialized")
	}
	if err := ValidateCN(commonName); err != nil {
		return nil, nil, err
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: generate client key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: generate serial: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: now,
		NotAfter:  now.AddDate(3, 0, 0),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: create client cert: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("ovpn: parse client cert: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	entry := IssuedCert{
		CN:       cert.Subject.CommonName,
		Serial:   cert.SerialNumber.String(),
		NotAfter: cert.NotAfter,
	}

	if ca.dir != "" {
		if err := os.MkdirAll(filepath.Join(ca.dir, issuedDir), 0o755); err != nil {
			return nil, nil, fmt.Errorf("ovpn: mkdir issued: %w", err)
		}
		base := sanitizeCN(commonName) + "-" + cert.SerialNumber.String()
		certPath := filepath.Join(ca.dir, issuedDir, base+".crt")
		keyPath := filepath.Join(ca.dir, issuedDir, base+".key")
		if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
			return nil, nil, fmt.Errorf("ovpn: write cert: %w", err)
		}
		if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
			return nil, nil, fmt.Errorf("ovpn: write key: %w", err)
		}
		entry.CertPath = certPath
		entry.KeyPath = keyPath
	}
	ca.issued = append(ca.issued, entry)
	return certPEM, keyPEM, nil
}

// LoadOrCreate 从目录加载 CA；目录不存在或无 CA 时生成并保存。
func LoadOrCreate(dir string) (*CA, error) {
	if dir == "" {
		return nil, errors.New("ovpn: empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("ovpn: mkdir: %w", err)
	}

	certPath := filepath.Join(dir, caCertName)
	keyPath := filepath.Join(dir, caKeyName)

	// 尝试读取已有 CA。
	if certBytes, err := os.ReadFile(certPath); err == nil {
		if keyBytes, err := os.ReadFile(keyPath); err == nil {
			ca, err := parseCA(dir, certBytes, keyBytes)
			if err == nil {
				// 重建 issued 列表
				ca.issued = scanIssued(dir)
				return ca, nil
			}
		}
	}

	// 生成新 CA。
	ca, err := NewCA()
	if err != nil {
		return nil, err
	}
	ca.dir = dir

	// 保存 CA 证书与私钥。
	certPEM := ca.CACertPEM()
	keyPEM := ca.CAKeyPEM()
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("ovpn: write ca cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("ovpn: write ca key: %w", err)
	}

	ca.issued = scanIssued(dir)
	return ca, nil
}

// parseCA 从 PEM 字节解析 CA。
func parseCA(dir string, certBytes, keyBytes []byte) (*CA, error) {
	certBlock, _ := pem.Decode(certBytes)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("ovpn: invalid ca cert pem")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ovpn: parse ca cert: %w", err)
	}
	if !cert.IsCA {
		return nil, errors.New("ovpn: cert is not a CA")
	}

	keyBlock, _ := pem.Decode(keyBytes)
	if keyBlock == nil {
		return nil, fmt.Errorf("ovpn: invalid ca key pem")
	}
	var key *rsa.PrivateKey
	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("ovpn: parse ca key: %w", err)
		}
		key = k
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("ovpn: parse ca key: %w", err)
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("ovpn: ca key is not RSA")
		}
		key = rk
	default:
		// 尝试 PKCS1
		k, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("ovpn: unsupported key type %s", keyBlock.Type)
		}
		key = k
	}

	// 验证密钥与证书匹配。
	if !key.PublicKey.Equal(cert.PublicKey) {
		return nil, errors.New("ovpn: ca key does not match cert")
	}

	return &CA{dir: dir, cert: cert, key: key}, nil
}

// List 返回已签发证书的摘要列表（扫描 issued 目录 + 内存）。
func (ca *CA) List() []IssuedCert {
	if ca == nil {
		return []IssuedCert{}
	}
	if ca.dir != "" {
		res := scanIssued(ca.dir)
		if len(res) > 0 {
			return res
		}
	}
	// 内存回退
	out := make([]IssuedCert, len(ca.issued))
	copy(out, ca.issued)
	return out
}

// scanIssued 扫描 issued 目录下的 *.crt，解析为 IssuedCert 列表。
func scanIssued(dir string) []IssuedCert {
	issuedPath := filepath.Join(dir, issuedDir)
	entries, err := os.ReadDir(issuedPath)
	if err != nil {
		return []IssuedCert{}
	}
	var out []IssuedCert
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".crt") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(issuedPath, e.Name()))
		if err != nil {
			continue
		}
		block, _ := pem.Decode(b)
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		out = append(out, IssuedCert{
			CN:       cert.Subject.CommonName,
			Serial:   cert.SerialNumber.String(),
			NotAfter: cert.NotAfter,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CN < out[j].CN })
	if out == nil {
		out = []IssuedCert{}
	}
	return out
}

// toIPs 把 interface{} 转为 []net.IP。
func toIPs(v interface{}) ([]net.IP, error) {
	if v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case []net.IP:
		return t, nil
	case []string:
		out := make([]net.IP, 0, len(t))
		for _, s := range t {
			if s == "" {
				continue
			}
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, fmt.Errorf("ovpn: invalid ip %q", s)
			}
			out = append(out, ip)
		}
		return out, nil
	case string:
		if t == "" {
			return nil, nil
		}
		ip := net.ParseIP(t)
		if ip == nil {
			return nil, fmt.Errorf("ovpn: invalid ip %q", t)
		}
		return []net.IP{ip}, nil
	case net.IP:
		return []net.IP{t}, nil
	default:
		return nil, fmt.Errorf("ovpn: unsupported ips type %T", v)
	}
}

func sanitizeCN(cn string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return r.Replace(cn)
}
