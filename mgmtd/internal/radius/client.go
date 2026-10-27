package radius

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// Config describes a RADIUS server connection.
type Config struct {
	Server     string `json:"server"`
	Port       int    `json:"port"`
	Secret     string `json:"secret"`
	TimeoutSec int    `json:"timeout_sec"`
}

// Result is the outcome of a RADIUS authentication attempt.
type Result struct {
	Accept     bool              `json:"accept"`
	Message    string            `json:"message"`
	Attributes map[string]string `json:"attributes"`
}

func (c Config) port() int {
	if c.Port <= 0 {
		return 1812
	}
	return c.Port
}

func (c Config) timeout() time.Duration {
	if c.TimeoutSec <= 0 {
		return 5 * time.Second
	}
	return time.Duration(c.TimeoutSec) * time.Second
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

// encryptPassword implements RFC 2865 §5.2 User-Password encryption.
func encryptPassword(secret, ra []byte, password string) []byte {
	// pad password to multiple of 16 with null bytes
	p := []byte(password)
	if len(p)%16 != 0 {
		padded := make([]byte, ((len(p)/16)+1)*16)
		copy(padded, p)
		p = padded
	}
	if len(p) == 0 {
		p = make([]byte, 16)
	}
	out := make([]byte, len(p))
	var prev []byte = ra
	for i := 0; i < len(p); i += 16 {
		h := md5.New()
		h.Write(secret)
		h.Write(prev)
		sum := h.Sum(nil)
		for j := 0; j < 16; j++ {
			out[i+j] = p[i+j] ^ sum[j]
		}
		prev = out[i : i+16]
	}
	return out
}

// attrType names for common RADIUS attributes.
var attrNames = map[byte]string{
	1:  "User-Name",
	2:  "User-Password",
	4:  "NAS-IP-Address",
	18: "Reply-Message",
	27: "Session-Timeout",
}

// Authenticate sends an Access-Request and returns the result.
func (c Config) Authenticate(username, password string) (Result, error) {
	if c.Server == "" {
		return Result{}, fmt.Errorf("radius server not configured")
	}
	ra := randBytes(16)
	id := randBytes(1)[0]

	// Build attributes
	var attrs []byte
	// User-Name (1)
	attrs = append(attrs, 1, byte(2+len(username)))
	attrs = append(attrs, []byte(username)...)
	// User-Password (2)
	ep := encryptPassword([]byte(c.Secret), ra, password)
	attrs = append(attrs, 2, byte(2+len(ep)))
	attrs = append(attrs, ep...)
	// NAS-IP-Address (4) = 0.0.0.0
	attrs = append(attrs, 4, 6, 0, 0, 0, 0)

	// Header
	length := 20 + len(attrs)
	pkt := make([]byte, length)
	pkt[0] = 1 // Access-Request
	pkt[1] = id
	binary.BigEndian.PutUint16(pkt[2:4], uint16(length))
	copy(pkt[4:20], ra)
	copy(pkt[20:], attrs)

	addr := fmt.Sprintf("%s:%d", c.Server, c.port())
	conn, err := net.DialTimeout("udp", addr, c.timeout())
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout()))
	if _, err := conn.Write(pkt); err != nil {
		return Result{}, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return Result{}, err
	}
	resp := buf[:n]
	if len(resp) < 20 {
		return Result{}, fmt.Errorf("short response")
	}
	code := resp[0]
	respID := resp[1]
	respLen := int(binary.BigEndian.Uint16(resp[2:4]))
	if respLen > len(resp) {
		respLen = len(resp)
	}
	respAuth := resp[4:20]
	// verify Response Authenticator
	h := md5.New()
	h.Write(resp[:4]) // code+id+length
	h.Write(ra)       // Request Authenticator
	h.Write(resp[20:respLen])
	h.Write([]byte(c.Secret))
	sum := h.Sum(nil)
	for i := 0; i < 16; i++ {
		if sum[i] != respAuth[i] {
			return Result{}, fmt.Errorf("bad response authenticator")
		}
	}
	_ = respID
	// parse attributes
	attrs2 := resp[20:respLen]
	parsed := map[string]string{}
	i := 0
	for i < len(attrs2) {
		if i+2 > len(attrs2) {
			break
		}
		typ := attrs2[i]
		l := int(attrs2[i+1])
		if l < 2 || i+l > len(attrs2) {
			break
		}
		val := attrs2[i+2 : i+l]
		name := attrNames[typ]
		if name == "" {
			name = fmt.Sprintf("attr_%d", typ)
		}
		// Reply-Message is string; others hex
		if typ == 18 {
			parsed[name] = string(val)
		} else {
			parsed[name] = fmt.Sprintf("%x", val)
		}
		i += l
	}
	res := Result{Attributes: parsed}
	if code == 2 {
		res.Accept = true
		res.Message = parsed["Reply-Message"]
	} else if code == 3 {
		res.Accept = false
		res.Message = parsed["Reply-Message"]
	} else {
		return res, fmt.Errorf("unexpected response code %d", code)
	}
	return res, nil
}
