package cloud

import (
	"testing"
	"time"
)

func TestRegisterAndHeartbeat(t *testing.T) {
	m := NewLegacyManager()
	dev := LegacyDevice{ID: "dev-test-01", Name: "Router-A", Model: "RM200", Version: "v1.2.3"}
	if err := m.Register(dev); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	list := m.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 device, got %d", len(list))
	}
	if list[0].Name != "Router-A" {
		t.Errorf("expected name Router-A, got %s", list[0].Name)
	}
	if err := m.Heartbeat("dev-test-01", time.Now().Unix()); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	dev2, ok := m.devices["dev-test-01"]
	if !ok {
		t.Fatal("device missing after heartbeat")
	}
	if !dev2.Online {
		t.Errorf("expected Online=true after heartbeat")
	}
}

func TestPushConfigAndPoll(t *testing.T) {
	m := NewLegacyManager()
	m.Register(LegacyDevice{ID: "dev-cfg", Name: "C", Model: "M", Version: "1"})
	ver, err := m.PushConfig("dev-cfg", map[string]string{"dns": "8.8.8.8"})
	if err != nil {
		t.Fatalf("PushConfig failed: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	cfg, v := m.PollConfig("dev-cfg", 0)
	if v != 1 || cfg["dns"] != "8.8.8.8" {
		t.Errorf("PollConfig returned wrong version or data: v=%d cfg=%v", v, cfg)
	}
	cfg2, v2 := m.PollConfig("dev-cfg", 1)
	if cfg2 != nil || v2 != 1 {
		t.Errorf("expected no change when sinceVersion == current")
	}
}
