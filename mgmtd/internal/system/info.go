package system

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

// Info holds real host system information read from /proc and /etc.
type Info struct {
	Hostname    string `json:"hostname"`
	CPUModel    string `json:"cpu_model"`
	CPUCores    int    `json:"cpu_cores"`
	MemTotalKB  int64  `json:"mem_total_kb"`
	MemAvailKB  int64  `json:"mem_avail_kb"`
	OSVersion   string `json:"os_version"`
	GoVersion   string `json:"go_version"`
}

// GetInfo reads real system data. Failures on optional fields are tolerated
// (empty value), but hostname is expected to be present.
func GetInfo() Info {
	info := Info{
		Hostname:  hostname(),
		CPUModel:  cpuModel(),
		CPUCores:  runtime.NumCPU(),
		OSVersion: osReleasePrettyName(),
		GoVersion: runtime.Version(),
	}
	info.MemTotalKB, info.MemAvailKB = memInfo()
	return info
}

func hostname() string {
	if b, err := os.ReadFile("/etc/hostname"); err == nil {
		if h := strings.TrimSpace(string(b)); h != "" {
			return h
		}
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return ""
}

func cpuModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "model name") {
			if i := strings.IndexByte(line, ':'); i >= 0 {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return ""
}

// memInfo returns (MemTotalKB, MemAvailableKB) from /proc/meminfo.
func memInfo() (total, avail int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		var kb int64
		if strings.HasPrefix(line, "MemTotal:") {
			if _, err := fmtSscanf(line, "MemTotal:", &kb); err == nil {
				total = kb
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			if _, err := fmtSscanf(line, "MemAvailable:", &kb); err == nil {
				avail = kb
			}
		}
	}
	return
}

// fmtSscanf parses "MemTotal:       16298668 kB" -> 16298668.
func fmtSscanf(line, prefix string, out *int64) (int, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, os.ErrInvalid
	}
	n := int64(0)
	for _, r := range fields[0] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int64(r-'0')
	}
	*out = n
	return 1, nil
}

func osReleasePrettyName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}
