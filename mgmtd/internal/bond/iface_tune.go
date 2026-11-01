package bond

import (
	"fmt"
	"os"
	"regexp"
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

// ifaceNameRe 是网络接口名的字符白名单。
//
// 这是纵深防御：ApplyTune 已经不再经过 shell（改 os.WriteFile），但接口名
// 还会被送进 ip/tc 等命令、拼进 /sys 路径。允许任意字符意味着参数注入
// （以 "-" 开头的值被当成选项）和路径穿越（../）仍然可行。Linux 内核实
// 际允许的字符集是 [A-Za-z0-9_.-]，长度上限 IFNAMSIZ-1 = 15；这里刻意放
// 宽到 32 以兼容 vlan/bond 的复合名，仍然挡住 "/"、".."、" "、";"、"$"。
var ifaceNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:-]{0,31}$`)

// ValidateTune 校验接口调优。
func ValidateTune(c TuneConfig) error {
	iface := strings.TrimSpace(c.Iface)
	if iface == "" {
		return fmt.Errorf("iface required")
	}
	if iface == "lo" {
		return fmt.Errorf("iface must not be loopback (lo)")
	}
	if !ifaceNameRe.MatchString(iface) {
		return fmt.Errorf("iface contains illegal characters: %q", c.Iface)
	}
	if strings.Contains(iface, "..") {
		return fmt.Errorf("iface must not contain '..'")
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
// sysfsWrite / sysfsRead 是写 /sys 的接缝。
//
// 生产环境直接落 os.WriteFile——参数不经过任何解释器，从根上消灭命令注入。
// 测试通过替换这两个函数验证写入内容与回读校验，不依赖真实的 /sys 布局。
var (
	sysfsWrite = func(path, value string) error {
		return os.WriteFile(path, []byte(value), 0o644)
	}
	sysfsRead = func(path string) (string, error) {
		b, err := os.ReadFile(path)
		return string(b), err
	}
)

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
	// 直接写 sysfs，不经 shell。
	// 旧实现是 r.Run("sh","-c","echo <n> > "+path)，而 path 里含用户可控的
	// c.Iface —— 传 iface="eth0; touch /tmp/x #" 就能以 root 执行任意命令
	// （已实测复现）。改成 os.WriteFile 后参数不经过任何解释器。
	if c.GroMaxSize > 0 {
		path := "/sys/class/net/" + c.Iface + "/gro_max_size"
		if err := sysfsWrite(path, strconv.Itoa(c.GroMaxSize)+"\n"); err != nil {
			return false, fmt.Sprintf("gro_max_size apply failed: %v", err)
		}
	}
	// RPS 写入 + 回读确认。
	for i, mask := range c.RPS {
		path := fmt.Sprintf("/sys/class/net/%s/queues/rx-%d/rps_cpus", c.Iface, i)
		if err := sysfsWrite(path, strconv.Itoa(mask)+"\n"); err != nil {
			return false, fmt.Sprintf("rps write failed on rx-%d: %v", i, err)
		}
		// 回读确认
		got, err := sysfsRead(path)
		if err != nil {
			return false, fmt.Sprintf("rps readback failed on rx-%d: %v", i, err)
		}
		if strings.TrimSpace(got) != strconv.Itoa(mask) {
			return false, fmt.Sprintf("rps readback mismatch rx-%d: want %d got %q", i, mask, strings.TrimSpace(got))
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
