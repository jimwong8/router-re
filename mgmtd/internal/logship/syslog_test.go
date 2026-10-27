package logship

import (
	"strings"
	"testing"
)

func TestValidateRejectsMissingPort(t *testing.T) {
	if err := ValidateTarget(SyslogTarget{Name: "t1", Address: "host", Protocol: "udp", Format: "rfc5424", Facility: "local0"}); err == nil {
		t.Fatal("address without port must be rejected")
	}
}

func TestValidateRejectsPortZero(t *testing.T) {
	if err := ValidateTarget(SyslogTarget{Name: "t1", Address: "host:0", Protocol: "udp", Format: "rfc5424", Facility: "local0"}); err == nil {
		t.Fatal("port=0 must be rejected")
	}
}

func TestValidateRejectsBadProtocol(t *testing.T) {
	if err := ValidateTarget(SyslogTarget{Name: "t1", Address: "host:514", Protocol: "http", Format: "rfc5424", Facility: "local0"}); err == nil {
		t.Fatal("bad protocol must be rejected")
	}
}

func TestRenderRsyslogTcpUsesAtAt(t *testing.T) {
	tgt := SyslogTarget{Name: "t1", Address: "10.0.0.1:6514", Protocol: "tcp", Format: "rfc5424", Facility: "local0"}
	out := RenderRsyslog(tgt)
	if !strings.Contains(out, "@@10.0.0.1:6514;RSYSLOG_SyslogProtocol23Format") {
		t.Errorf("tcp must use @@ with RSYSLOG_SyslogProtocol23Format:\n%s", out)
	}
	// tcp must not use a bare single-@ line (which would be UDP)
	if strings.Contains(out, " @10.0.0.1:6514") {
		t.Errorf("tcp must not use single @:\n%s", out)
	}
}

func TestRenderRsyslogUdpUsesAt(t *testing.T) {
	tgt := SyslogTarget{Name: "t1", Address: "10.0.0.1:514", Protocol: "udp", Format: "rfc3164", Facility: "local0"}
	out := RenderRsyslog(tgt)
	if !strings.Contains(out, "@10.0.0.1:514") {
		t.Errorf("udp must use single @:\n%s", out)
	}
	if strings.Contains(out, "@@10.0.0.1:514") {
		t.Errorf("udp must not use @@:\n%s", out)
	}
}

func TestTestTargetLoggerMissing(t *testing.T) {
	stub := &lsStub{err: &lsErr{"logger: command not found"}}
	ok, detail := TestTarget(stub, SyslogTarget{Name: "t1", Address: "10.0.0.1:514", Protocol: "udp", Format: "rfc5424", Facility: "local0"})
	if ok {
		t.Fatal("missing logger must not report success")
	}
	if !strings.Contains(detail, "logger") {
		t.Errorf("detail must mention logger, got: %s", detail)
	}
}
