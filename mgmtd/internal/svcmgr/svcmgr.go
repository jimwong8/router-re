// Package svcmgr manages the enable/disable status of system services
// (ssh, telnet, ftp, dnsmasq) on OpenWrt-style hosts via /etc/init.d/<name>.
//
// It renders the OpenWrt rc.d / init.d enable/disable semantics as argv slices
// and executes them through an injected execx.Runner (real DefaultRunner in
// production, FakeRunner in tests) — never by building a shell string.
package svcmgr

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Services is the canonical list of managed services.
var Services = []string{"ssh", "telnet", "ftp", "dnsmasq"}

// Status of one service.
type Status struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
}

// Manager wraps an exec runner.
type Manager struct {
	R execx.Runner
}

// New builds a Manager bound to r.
func New(r execx.Runner) *Manager { return &Manager{R: r} }

// StatusAll returns the status of every managed service.
func (m *Manager) StatusAll() []Status {
	out := make([]Status, 0, len(Services))
	for _, name := range Services {
		st, _ := m.Status(name)
		out = append(out, st)
	}
	return out
}

// Status returns the enabled/running status of one service.
func (m *Manager) Status(name string) (Status, error) {
	if !validService(name) {
		return Status{}, fmt.Errorf("unknown service %q (allowed: %s)", name, strings.Join(Services, ", "))
	}
	st := Status{Name: name}
	// enabled: /etc/init.d/<name> enabled  (exit 0 == enabled)
	if _, err := m.R.Run("/etc/init.d/"+name, "enabled"); err == nil {
		st.Enabled = true
	}
	// running: /etc/init.d/<name> running  (exit 0 == running)
	if _, err := m.R.Run("/etc/init.d/"+name, "running"); err == nil {
		st.Running = true
	}
	return st, nil
}

// SetEnabled toggles a service: runs /etc/init.d/<name> enable|disable.
func (m *Manager) SetEnabled(name string, enabled bool) error {
	if !validService(name) {
		return fmt.Errorf("unknown service %q (allowed: %s)", name, strings.Join(Services, ", "))
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	_, err := m.R.Run("/etc/init.d/"+name, action)
	return err
}

func validService(name string) bool {
	for _, s := range Services {
		if s == name {
			return true
		}
	}
	return false
}
