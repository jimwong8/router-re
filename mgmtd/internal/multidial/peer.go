package multidial

import (
	"fmt"
	"strings"
)

// RenderPeer renders the /etc/ppp/peers/<name> file for a client session.
// The peer file itself contains no password — that lives in chap-secrets.
func RenderPeer(s Session) string {
	var b strings.Builder
	b.WriteString("plugin rp-pppoe.so\n")
	b.WriteString(s.Iface + "\n")
	b.WriteString(fmt.Sprintf("user \"%s\"\n", s.Username))
	b.WriteString("noauth\n")
	b.WriteString("noipdefault\n")
	b.WriteString("defaultroute\n")
	b.WriteString("replacedefaultroute\n")
	b.WriteString("persist\n")
	b.WriteString("maxfail 0\n")
	if s.MRU > 0 {
		fmt.Fprintf(&b, "mtu %d\n", s.MRU)
		fmt.Fprintf(&b, "mru %d\n", s.MRU)
	}
	if s.LCPEcho > 0 {
		fmt.Fprintf(&b, "lcp-echo-interval %d\n", s.LCPEcho)
		b.WriteString("lcp-echo-failure 3\n")
	}
	if s.Service != "" {
		fmt.Fprintf(&b, "service %s\n", s.Service)
	}
	if s.AC != "" {
		fmt.Fprintf(&b, "nic-%s\n", s.AC)
	}
	return b.String()
}

// RenderChapSecret renders one /etc/ppp/chap-secrets line.
func RenderChapSecret(s Session) string {
	return fmt.Sprintf("%s * %s *\n", s.Username, s.Password)
}

// RedactPeer returns the peer file content with any sensitive-looking
// fields masked. The peer file itself carries no password, so this is
// currently a passthrough — kept for safety in case a future edit leaks
// credentials into the peer file.
func RedactPeer(s Session) string {
	return RenderPeer(s)
}
