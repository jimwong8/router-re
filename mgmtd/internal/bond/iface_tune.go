package bond

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// TuneConfig 描述接口高级参数调优。
type TuneConfig struct {
	Iface      string `json:"iface"`
	MTU        int    `json:"mtu"`          // 0 或 576-9000
	TxQueueLen int    `json:"txqueuelen"`   // 0 或 1-10000
	GroMaxSize int    `json:"gro_max_size"` // 0 或 0-65536
	RPS        []int  `json:"rps"`          // 每队列的 CPU 掩码，空=不改
}

// ValidateTune 校验接口调优。
func ValidateTune(c TuneConfig) error {
	iface := strings.TrimSpace(c.Iface)
	if iface == "" {
		return fmt.Errorf("iface required")
	}
	if iface == "lo" {
		return fmt.Errorf("iface must not be loopback (lo)")
	}
	if c.MTU != 0 && (c.MTU < 576 || c.MTU > 9000) {
		return fmt.Errorf("mtu must be 0 or in [576,9000], got %d", c.MTU)
	}
	if c.TxQueueLen != 0 && (c.TxQueueLen < 1 || c.TxQueueLen > 10000) {
		return fmt.Errorf("txqueuelen must be 0 or in [1,10000], got %d", c.TxQueueLen)
	}
	if c.GroMaxSize != 0 && (c.GroMaxSize < 0 || c.GroMaxSize > 65536) {
		return fmt.Errorf("gro_max_size must be 0 or in [0,65536], got %d", c.GroMaxSize)
	}
	return nil
}

// ApplyTune 执行接口调优。
// MTU/TxQueueLen 用 ip link set dev <iface> mtu/txqueuelen。
// RPS 写 /sys/class/net/<iface>/queues/rx-<i>/rps_cpus，**必须回读确认**。
func ApplyTune(r execx.Runner, c TuneConfig) (bool, string) {
	if err := ValidateTune(c); err != nil {
		return false, err.Error()
	}
	if c.MTU > 0 {
		if _, err := r.Run("ip", "link", "set", "dev", c.Iface, "mtu", strconv.Itoa(c.MTU)); err != nil {
			return false, fmt.Sprintf("mtu apply failed: %v", err)
		}
	}
	if c.TxQueueLen > 0 {
		if _, err := r.Run("ip", "link", "set", "dev", c.Iface, "txqueuelen", strconv.Itoa(c.TxQueueLen)); err != nil {
			return false, fmt.Sprintf("txqueuelen apply failed: %v", err)
		}
	}
	if c.GroMaxSize > 0 {
		path := "/sys/class/net/" + c.Iface + "/gro_max_size"
		if _, err := r.Run("sh", "-c", "echo "+strconv.Itoa(c.GroMaxSize)+" > "+path); err != nil {
			return false, fmt.Sprintf("gro_max_size apply failed: %v", err)
		}
	}
	// RPS 写入 + 回读确认。
	for i, mask := range c.RPS {
		path := fmt.Sprintf("/sys/class/net/%s/queues/rx-%d/rps_cpus", c.Iface, i)
		if _, err := r.Run("sh", "-c", "echo "+strconv.Itoa(mask)+" > "+path); err != nil {
			return false, fmt.Sprintf("rps write failed on rx-%d: %v", i, err)
		}
		// 回读确认
		got, err := r.Run("cat", path)
		if err != nil {
			return false, fmt.Sprintf("rps readback failed on rx-%d: %v", i, err)
		}
		if strings.TrimSpace(got) != strconv.Itoa(mask) {
			return false, fmt.Sprintf("rps readback mismatch rx-%d: want %d got %q", i, mask, got)
		}
	}
	return true, "interface tune applied"
}

// ReadTune 从 sysfs 读回当前调优值。
func ReadTune(r execx.Runner, iface string) (map[string]string, error) {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return nil, fmt.Errorf("iface required")
	}
	out := map[string]string{}
	if v, err := r.Run("ip", "-o", "link", "show", "dev", iface); err == nil {
		out["link"] = strings.TrimSpace(v)
	}
	if v, err := r.Run("cat", "/sys/class/net/"+iface+"/tx_queue_len"); err == nil {
		out["tx_queue_len"] = strings.TrimSpace(v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("read tune %s: no data", iface)
	}
	return out, nil
}
