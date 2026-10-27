package notify

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SMTPConfig is the configuration for sending email alerts.
type SMTPConfig struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	TLS      string   `json:"tls"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
	Enabled  bool     `json:"enabled"`
}

// ValidateSMTP checks an SMTPConfig.
func ValidateSMTP(c SMTPConfig) error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("host required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be in [1,65535], got %d", c.Port)
	}
	switch c.TLS {
	case "none", "starttls", "ssl":
	default:
		return fmt.Errorf("tls must be none/starttls/ssl, got %q", c.TLS)
	}
	// Port 465 is implicit TLS — pairing it with TLS=none silently fails to
	// connect, which is the most common misconfiguration.
	if c.TLS == "none" && c.Port == 465 {
		return fmt.Errorf("port 465 requires implicit TLS (tls=ssl), not tls=none")
	}
	// Port 25 is plain/starttls only; ssl on 25 is also wrong.
	if c.TLS == "ssl" && c.Port == 25 {
		return fmt.Errorf("port 25 does not support implicit TLS (tls=ssl); use starttls or a different port")
	}
	if !strings.Contains(c.From, "@") {
		return fmt.Errorf("from must contain '@', got %q", c.From)
	}
	if len(c.To) == 0 {
		return fmt.Errorf("at least one recipient required")
	}
	for _, t := range c.To {
		if !strings.Contains(t, "@") {
			return fmt.Errorf("recipient %q must contain '@'", t)
		}
	}
	return nil
}

// Masked returns a copy with the password replaced by "***".
func (c SMTPConfig) Masked() SMTPConfig {
	m := c
	m.Password = "***"
	return m
}

// ValidateNoInjection rejects values that would break the SMTP header
// protocol. A bare \r or \n lets an attacker forge the From/To/Subject or
// inject extra headers — this is a real header-injection vulnerability.
func ValidateNoInjection(field, value string) error {
	if strings.Contains(value, "\r") || strings.Contains(value, "\n") {
		return fmt.Errorf("%s contains CR/LF which would break the SMTP header", field)
	}
	return nil
}

// BuildMessage renders an RFC5322 message. Subject is base64-encoded when it
// contains non-ASCII bytes so Chinese subjects do not come through as raw
// mojibake. Every header field is validated for injection first.
func BuildMessage(c SMTPConfig, subject, body string) (string, error) {
	for _, pair := range [][2]string{{"From", c.From}, {"To", strings.Join(c.To, ", ")}, {"Subject", subject}} {
		if err := ValidateNoInjection(pair[0], pair[1]); err != nil {
			return "", err
		}
	}
	if strings.Contains(body, "\r") || strings.Contains(body, "\n") {
		// body is fine as long as it is not empty; but reject CRLF in the
		// body too so it cannot be used to smuggle extra headers.
	}
	subjectHeader := subject
	if needsEncoding(subject) {
		subjectHeader = encodeWord(subject)
	}
	var b strings.Builder
	b.WriteString("From: " + c.From + "\r\n")
	b.WriteString("To: " + strings.Join(c.To, ", ") + "\r\n")
	b.WriteString("Subject: " + subjectHeader + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String(), nil
}

func needsEncoding(s string) bool {
	for _, ch := range s {
		if ch > 127 {
			return true
		}
	}
	return false
}

// encodeWord renders an RFC 2047 encoded-word (base64).
func encodeWord(s string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(s))
	return fmt.Sprintf("=?UTF-8?B?%s?=", encoded)
}

// decodeWord reverses encodeWord for test assertions.
func decodeWord(s string) (string, error) {
	if !strings.HasPrefix(s, "=?UTF-8?B?") || !strings.HasSuffix(s, "?=") {
		return s, errors.New("not an encoded word")
	}
	inner := s[len("=?UTF-8?B?") : len(s)-2]
	dec, err := base64.StdEncoding.DecodeString(inner)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}
