package radius

import (
	"crypto/md5"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// decryptPassword is the server-side inverse of encryptPassword.
func decryptPassword(secret, ra []byte, enc []byte) string {
	out := make([]byte, len(enc))
	var prev []byte = ra
	for i := 0; i < len(enc); i += 16 {
		h := md5.New()
		h.Write(secret)
		h.Write(prev)
		sum := h.Sum(nil)
		for j := 0; j < 16; j++ {
			out[i+j] = enc[i+j] ^ sum[j]
		}
		prev = enc[i : i+16]
	}
	// trim null padding
	n := len(out)
	for n > 0 && out[n-1] == 0 {
		n--
	}
	return string(out[:n])
}

func parseAttrs(b []byte) map[byte][]byte {
	m := map[byte][]byte{}
	i := 0
	for i < len(b) {
		if i+2 > len(b) {
			break
		}
		t := b[i]
		l := int(b[i+1])
		if l < 2 || i+l > len(b) {
			break
		}
		m[t] = b[i+2 : i+l]
		i += l
	}
	return m
}

func fakeServer(t *testing.T, secret string, acceptUser string) *net.UDPConn {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			req := append([]byte(nil), buf[:n]...)
			ra := req[4:20]
			id := req[1]
			attrs := parseAttrs(req[20:])
			user := string(attrs[1])
			enc := attrs[2]
			gotPass := decryptPassword([]byte(secret), ra, enc)

			code := byte(3) // reject
			if user == acceptUser && gotPass == "secret123" {
				code = 2 // accept
			}

			// response attrs
			var respAttrs []byte
			msg := "Access granted"
			if code == 3 {
				msg = "Access denied"
			}
			// Reply-Message (18)
			respAttrs = append(respAttrs, 18, byte(2+len(msg)))
			respAttrs = append(respAttrs, []byte(msg)...)

			respLen := 20 + len(respAttrs)
			resp := make([]byte, respLen)
			resp[0] = code
			resp[1] = id
			binary.BigEndian.PutUint16(resp[2:4], uint16(respLen))
			copy(resp[4:20], ra) // placeholder, overwrite below
			copy(resp[20:], respAttrs)
			// compute Response Authenticator
			h := md5.New()
			h.Write(resp[:4])
			h.Write(ra)
			h.Write(resp[20:respLen])
			h.Write([]byte(secret))
			sum := h.Sum(nil)
			copy(resp[4:20], sum)

			conn.WriteTo(resp, addr)
		}
	}()
	return conn.(*net.UDPConn)
}

func TestAuthenticateAccept(t *testing.T) {
	conn := fakeServer(t, "testing123", "alice")
	defer conn.Close()
	cfg := Config{
		Server:     "127.0.0.1",
		Port:       conn.LocalAddr().(*net.UDPAddr).Port,
		Secret:     "testing123",
		TimeoutSec: 3,
	}
	res, err := cfg.Authenticate("alice", "secret123")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if !res.Accept {
		t.Fatalf("expected accept, got reject: %s", res.Message)
	}
	if res.Attributes["Reply-Message"] != "Access granted" {
		t.Errorf("unexpected reply message: %q", res.Attributes["Reply-Message"])
	}
}

func TestAuthenticateReject(t *testing.T) {
	conn := fakeServer(t, "testing123", "alice")
	defer conn.Close()
	cfg := Config{
		Server:     "127.0.0.1",
		Port:       conn.LocalAddr().(*net.UDPAddr).Port,
		Secret:     "testing123",
		TimeoutSec: 3,
	}
	res, err := cfg.Authenticate("bob", "secret123")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if res.Accept {
		t.Fatalf("expected reject, got accept")
	}
	if res.Attributes["Reply-Message"] != "Access denied" {
		t.Errorf("unexpected reply message: %q", res.Attributes["Reply-Message"])
	}
}

func TestPasswordRoundTrip(t *testing.T) {
	// Direct unit test of encrypt/decrypt round trip.
	secret := []byte("s3cr3t")
	ra := make([]byte, 16)
	for i := range ra {
		ra[i] = byte(i + 1)
	}
	pw := "hunter2-longer-password!"
	enc := encryptPassword(secret, ra, pw)
	dec := decryptPassword(secret, ra, enc)
	if dec != pw {
		t.Fatalf("round trip mismatch: got %q want %q", dec, pw)
	}
}

func TestRenderRadius(t *testing.T) {
	cfg := Config{Server: "10.0.0.1", Port: 1812, Secret: "secret"}
	if !contains(RenderRadiusClientConf(cfg), "authserver 10.0.0.1:1812") {
		t.Errorf("render conf missing authserver")
	}
	if !contains(RenderServersFile(cfg), "10.0.0.1\tsecret") {
		t.Errorf("servers file missing line")
	}
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
func init() {
	_ = time.Second
}
