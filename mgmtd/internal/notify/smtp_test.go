package notify

import (
	"strings"
	"testing"
)

func TestValidateSMTPRejectsTLSNonePort465(t *testing.T) {
	c := SMTPConfig{Host: "smtp.example.com", Port: 465, TLS: "none", From: "a@b.com", To: []string{"x@y.com"}}
	if err := ValidateSMTP(c); err == nil {
		t.Fatal("tls=none + port=465 must be rejected")
	}
}

func TestValidateSMTPRejectsFromWithoutAt(t *testing.T) {
	c := SMTPConfig{Host: "smtp.example.com", Port: 587, TLS: "starttls", From: "no-at", To: []string{"x@y.com"}}
	if err := ValidateSMTP(c); err == nil {
		t.Fatal("from without @ must be rejected")
	}
}

func TestValidateSMTPRejectsEmptyTo(t *testing.T) {
	c := SMTPConfig{Host: "smtp.example.com", Port: 587, TLS: "starttls", From: "a@b.com", To: nil}
	if err := ValidateSMTP(c); err == nil {
		t.Fatal("empty To must be rejected")
	}
}

func TestValidateSMTPRejectsSSLPort25(t *testing.T) {
	c := SMTPConfig{Host: "smtp.example.com", Port: 25, TLS: "ssl", From: "a@b.com", To: []string{"x@y.com"}}
	if err := ValidateSMTP(c); err == nil {
		t.Fatal("tls=ssl + port=25 must be rejected")
	}
}

func TestValidateSMTPAcceptsValid(t *testing.T) {
	c := SMTPConfig{Host: "smtp.example.com", Port: 587, TLS: "starttls", From: "a@b.com", To: []string{"x@y.com"}}
	if err := ValidateSMTP(c); err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}
}

func TestValidateNoInjectionRejectsCRLF(t *testing.T) {
	if err := ValidateNoInjection("Subject", "hello\r\nBcc: evil@x.com"); err == nil {
		t.Fatal("CRLF in subject must be rejected")
	}
	if err := ValidateNoInjection("From", "a@b.com\nTo: c@d.com"); err == nil {
		t.Fatal("newline in from must be rejected")
	}
}

func TestValidateNoInjectionAcceptsClean(t *testing.T) {
	if err := ValidateNoInjection("Subject", "normal subject"); err != nil {
		t.Fatalf("clean subject should pass: %v", err)
	}
}

func TestBuildMessageEncodesChineseSubject(t *testing.T) {
	c := SMTPConfig{Host: "h", Port: 587, TLS: "starttls", From: "a@b.com", To: []string{"x@y.com"}}
	out, err := BuildMessage(c, "中文主题", "body text")
	if err != nil {
		t.Fatalf("BuildMessage error: %v", err)
	}
	// raw Chinese must not appear in the header
	if strings.Contains(out, "中文主题") {
		t.Errorf("subject must be encoded, got raw Chinese:\n%s", out)
	}
	// required headers present
	for _, want := range []string{"From: a@b.com", "To: x@y.com", "Date:", "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing header %q:\n%s", want, out)
		}
	}
	// decode and verify round-trip
	line := ""
	for _, l := range strings.Split(out, "\r\n") {
		if strings.HasPrefix(l, "Subject:") {
			line = strings.TrimSpace(l[len("Subject:"):])
			break
		}
	}
	dec, err := decodeWord(line)
	if err != nil {
		t.Fatalf("decode encoded subject: %v", err)
	}
	if dec != "中文主题" {
		t.Errorf("decoded subject = %q, want 中文主题", dec)
	}
}

func TestBuildMessageRejectsInjection(t *testing.T) {
	c := SMTPConfig{Host: "h", Port: 587, TLS: "starttls", From: "a@b.com", To: []string{"x@y.com"}}
	_, err := BuildMessage(c, "subj\r\nX-Injected: 1", "body")
	if err == nil {
		t.Fatal("injected subject must be rejected")
	}
}

func TestMaskedPassword(t *testing.T) {
	c := SMTPConfig{Host: "h", Port: 587, TLS: "starttls", From: "a@b.com", Password: "secret", To: []string{"x@y.com"}}
	m := c.Masked()
	if m.Password != "***" {
		t.Errorf("masked password = %q", m.Password)
	}
	if c.Password != "secret" {
		t.Fatal("original must not be mutated")
	}
}
