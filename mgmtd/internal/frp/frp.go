package frp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Tunnel describes one frp tunnel.
type Tunnel struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // tcp | udp  (alias: proto)
	Proto      string `json:"proto"` // tcp | udp (alias of type)
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
}

// Config is the frpc client configuration.
type Config struct {
	ServerAddr string   `json:"server_addr"`
	ServerPort int      `json:"server_port"`
	Token      string   `json:"token"`
	Tunnels    []Tunnel `json:"tunnels"`
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ServerAddr) == "" {
		return fmt.Errorf("server_addr required")
	}
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("server_port must be 1..65535")
	}
	seen := map[string]bool{}
	for i, t := range c.Tunnels {
		if t.LocalPort <= 0 || t.LocalPort > 65535 {
			return fmt.Errorf("tunnels[%d] invalid local_port", i)
		}
		if t.RemotePort <= 0 || t.RemotePort > 65535 {
			return fmt.Errorf("tunnels[%d] invalid remote_port", i)
		}
		tp := strings.ToLower(strings.TrimSpace(t.Proto))
		if tp == "" {
			tp = strings.ToLower(strings.TrimSpace(t.Type))
		}
		if tp == "" {
			tp = "tcp"
		}
		if tp != "tcp" && tp != "udp" {
			return fmt.Errorf("tunnels[%d] invalid type %q", i, t.Type)
		}
		name := t.Name
		if name == "" {
			name = fmt.Sprintf("tunnel%d", i+1)
		}
		if seen[name] {
			return fmt.Errorf("duplicate tunnel name %q", name)
		}
		seen[name] = true
		// keep type/proto in sync so both JSON keys round-trip
		c.Tunnels[i].Type = tp
		c.Tunnels[i].Proto = tp
	}
	return nil
}

// RenderConfig renders an frpc (INI) configuration.
func RenderConfig(c Config) string {
	var b strings.Builder
	b.WriteString("[common]\n")
	b.WriteString(fmt.Sprintf("server_addr = %s\n", c.ServerAddr))
	if c.ServerPort == 0 {
		c.ServerPort = 7000
	}
	b.WriteString(fmt.Sprintf("server_port = %d\n", c.ServerPort))
	if c.Token != "" {
		b.WriteString(fmt.Sprintf("token = %s\n", c.Token))
	}
	b.WriteString("\n")
	for i, t := range c.Tunnels {
		name := t.Name
		if name == "" {
			name = fmt.Sprintf("tunnel%d", i+1)
		}
		tp := strings.ToLower(strings.TrimSpace(t.Type))
		if tp == "" {
			tp = strings.ToLower(strings.TrimSpace(t.Proto))
		}
		if tp == "" {
			tp = "tcp"
		}
		localIP := t.LocalIP
		if localIP == "" {
			localIP = "127.0.0.1"
		}
		b.WriteString(fmt.Sprintf("[%s]\n", name))
		b.WriteString(fmt.Sprintf("type = %s\n", tp))
		b.WriteString(fmt.Sprintf("local_ip = %s\n", localIP))
		b.WriteString(fmt.Sprintf("local_port = %d\n", t.LocalPort))
		b.WriteString(fmt.Sprintf("remote_port = %d\n", t.RemotePort))
		b.WriteString("\n")
	}
	return b.String()
}

// ConfigPath returns the on-disk frpc config path.
func ConfigPath() string {
	if v := os.Getenv("MGM_FRP_CONFIG"); v != "" {
		return v
	}
	return "/etc/frp/frpc.toml"
}

// BinaryPath returns the frpc binary path.
func BinaryPath() string {
	if v := os.Getenv("MGM_FRP_BINARY"); v != "" {
		return v
	}
	if p, err := exec.LookPath("frpc"); err == nil {
		return p
	}
	return "frpc"
}

// Status describes process state.
type Status struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid"`
	Binary  string `json:"binary"`
	Config  string `json:"config"`
	Exists  bool   `json:"binary_exists"`
}

var (
	mu      sync.Mutex
	proc    *exec.Cmd
	pid     int
)

// Apply writes config to disk and starts/reloads frpc.
func Apply(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	p := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(RenderConfig(cfg)), 0o644); err != nil {
		return err
	}
	// stop existing
	_ = Stop()
	bin := BinaryPath()
	if _, err := exec.LookPath(bin); err != nil && !filepath.IsAbs(bin) {
		// maybe binary path not found
		if _, err2 := exec.LookPath("frpc"); err2 != nil {
			return fmt.Errorf("frpc binary not found: %v", err2)
		}
	}
	cmd := exec.Command(bin, "-c", p)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start frpc: %w", err)
	}
	mu.Lock()
	proc = cmd
	pid = cmd.Process.Pid
	mu.Unlock()
	// reap
	go func() { _ = cmd.Wait() }()
	return nil
}

// Stop terminates the running frpc process.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
		proc = nil
		pid = 0
		return nil
	}
	return nil
}

// Status returns current process status.
func CurrentStatus() Status {
	mu.Lock()
	defer mu.Unlock()
	bin := BinaryPath()
	exists := false
	if _, err := exec.LookPath(bin); err == nil {
		exists = true
	} else if filepath.IsAbs(bin) {
		if _, err := os.Stat(bin); err == nil {
			exists = true
		}
	}
	running := false
	if proc != nil && proc.Process != nil {
		if proc.ProcessState == nil {
			running = true
		} else {
			running = !proc.ProcessState.Exited()
		}
		if !running {
			proc = nil
			pid = 0
		}
	}
	p := pid
	return Status{Running: running, PID: p, Binary: bin, Config: ConfigPath(), Exists: exists}
}

// ConfigFromJSON helper for server.
func ParseConfig(raw string) (Config, error) {
	var c Config
	if raw == "" {
		return c, nil
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, err
	}
	return c, nil
}

// SortTunnels sorts tunnels by name for stable output (unused but handy).
func SortTunnels(ts []Tunnel) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
}
