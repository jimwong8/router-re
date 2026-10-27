package procmon

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Process 描述一个进程。
type Process struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu_pct"`
	Mem     float64 `json:"mem_pct"`
	RSSKB   int64   `json:"rss_kb"`
	Command string  `json:"command"`
}

// ParsePS 解析 `ps -eo pid,user,pcpu,pmem,rss,args --no-headers` 输出。
// 最后一列 args 可含空格，因此用 FieldsN 限制前 5 列。
func ParsePS(out string) []Process {
	var procs []Process
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 前 5 列固定，第 6 列（args）保留空格。
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		rss, _ := strconv.ParseInt(fields[4], 10, 64)
		cmd := strings.Join(fields[5:], " ")
		procs = append(procs, Process{
			PID:     pid,
			User:    fields[1],
			CPU:     cpu,
			Mem:     mem,
			RSSKB:   rss,
			Command: cmd,
		})
	}
	return procs
}

// TopN 按 CPU 降序取前 n 个。n 大于长度时返回全部。
func TopN(procs []Process, n int) []Process {
	if n <= 0 {
		return []Process{}
	}
	cp := make([]Process, len(procs))
	copy(cp, procs)
	sort.Slice(cp, func(i, j int) bool { return cp[i].CPU > cp[j].CPU })
	if n > len(cp) {
		n = len(cp)
	}
	return cp[:n]
}

// ValidatePID 只接受 > 1 的整数（PID 1 是 init，杀它等于关机）。
func ValidatePID(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("pid must be > 1 (pid 1 is init; killing it shuts down the system), got %d", pid)
	}
	return nil
}