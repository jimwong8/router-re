package notify

import (
	"strings"
	"testing"
)

func TestValidateTrapRejectsEmptyCommunity(t *testing.T) {
	tt := TrapTarget{Host: "snmp.example.com", Port: 162, Version: "v2c", Community: ""}
	if err := ValidateTrap(tt); err == nil {
		t.Fatal("empty community must be rejected")
	}
}

func TestValidateTrapRejectsIllegalVersion(t *testing.T) {
	tt := TrapTarget{Host: "snmp.example.com", Port: 162, Version: "v3", Community: "public"}
	if err := ValidateTrap(tt); err == nil {
		t.Fatal("illegal version must be rejected")
	}
}

func TestValidateTrapRejectsCommunityWithSpace(t *testing.T) {
	tt := TrapTarget{Host: "snmp.example.com", Port: 162, Version: "v2c", Community: "my public"}
	if err := ValidateTrap(tt); err == nil {
		t.Fatal("community with space must be rejected")
	}
}

func TestValidateTrapAcceptsValid(t *testing.T) {
	tt := TrapTarget{Host: "snmp.example.com", Port: 162, Version: "v2c", Community: "public"}
	if err := ValidateTrap(tt); err != nil {
		t.Fatalf("valid trap should pass: %v", err)
	}
}

func TestRenderSinkContainsAuthCommunity(t *testing.T) {
	tt := TrapTarget{Host: "snmp.example.com", Port: 162, Version: "v2c", Community: "public"}
	out := RenderSink(tt)
	if !strings.Contains(out, "authCommunity") {
		t.Errorf("RenderSink must contain authCommunity:\n%s", out)
	}
	if !strings.Contains(out, "public") {
		t.Errorf("RenderSink must contain the community:\n%s", out)
	}
}

func TestTrapMasked(t *testing.T) {
	tt := TrapTarget{Host: "h", Port: 162, Version: "v2c", Community: "secret"}
	m := tt.Masked()
	if m.Community != "***" {
		t.Errorf("masked community = %q", m.Community)
	}
	if tt.Community != "secret" {
		t.Fatal("original must not be mutated")
	}
}
