// Package traffic 采集 /proc/net/dev 的接口计数器并计算实时速率。
//
// 设计约定（与 internal/dpi、internal/stats 保持一致）：
//   - 只读 /proc/net/dev，不调用 ifconfig/ip 等外部命令，避免依赖 PATH 与解析差异；
//   - Runner 参数保留，用于与其它模块统一的"执行层"调用签名（当前实现不需要执行外部命令，
//     因此不通过 shell 取值，避免注入与权限问题）；
//   - 计数器回绕（接口重启或 64 位计数溢出）时速率归零，绝不返回负数。
package traffic

import (
	"bufio"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// procNetDev 是接口计数器来源，测试中可替换（见 readCountersFrom）。
const procNetDev = "/proc/net/dev"

// Sample 是某一时刻单个接口的累计计数器快照。
type Sample struct {
	Iface     string `json:"iface"`
	RXBytes   uint64 `json:"rx_bytes"`
	TXBytes   uint64 `json:"tx_bytes"`
	RXPackets uint64 `json:"rx_packets"`
	TXPackets uint64 `json:"tx_packets"`
	RXErrors  uint64 `json:"rx_errors"`
	TXErrors  uint64 `json:"tx_errors"`
}

// Rate 是某段时间内单个接口的平均速率（字节/秒）。
type Rate struct {
	Iface string
	RXBps float64
	TXBps float64
}

// ReadCounters 读取 /proc/net/dev，返回所有非 lo 接口的计数器。
//
// r 按项目约定传入（execx.DefaultRunner{}），但本函数直接用 os.Open 读内核文件，
// 不执行任何外部命令 —— /proc/net/dev 是纯文本，跑 shell 只会多一层失败点。
func ReadCounters(r execx.Runner) ([]Sample, error) {
	return readCountersFrom(procNetDev, r)
}

// readCountersFrom 从任意路径读取（便于用临时文件做解析测试）。
func readCountersFrom(path string, r execx.Runner) ([]Sample, error) {
	_ = r // 保留统一签名；读 /proc/net/dev 不需要执行器
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseNetDev(f)
}

// parseNetDev 解析 /proc/net/dev 内容。
//
// 格式：前两行为表头，之后每行 "iface: rx_bytes rx_packets rx_errs rx_drop rx_fifo
// rx_frame rx_compressed rx_multicast tx_bytes tx_packets tx_errs tx_drop tx_fifo
// tx_colls tx_carrier tx_compressed"（共 16 个字段）。接口名与冒号之间可能有空格。
func parseNetDev(r io.Reader) ([]Sample, error) {
	sc := bufio.NewScanner(r)
	out := make([]Sample, 0, 8)
	for sc.Scan() {
		line := sc.Text()
		// 表头行没有冒号（"Inter-|   Receive ..."）。
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" || name == "lo" {
			continue // 跳过畸形行与回环口
		}
		nums := strings.Fields(line[colon+1:])
		if len(nums) < 16 {
			continue // 字段不全，丢弃而不是补零污染统计
		}
		u := func(i int) uint64 {
			v, err := strconv.ParseUint(nums[i], 10, 64)
			if err != nil {
				return 0
			}
			return v
		}
		out = append(out, Sample{
			Iface:     name,
			RXBytes:   u(0),
			RXPackets: u(1),
			RXErrors:  u(2),
			TXBytes:   u(8),
			TXPackets: u(9),
			TXErrors:  u(10),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ComputeRates 按接口名匹配前后两次采样，计算平均速率。
//
// elapsedSec <= 0（首次采样、时钟回拨、调用过密）时全部返回 0。
// cur < prev 说明计数器回绕或接口重启过，该接口速率返回 0，绝不返回负数。
func ComputeRates(prev, cur []Sample, elapsedSec float64) []Rate {
	out := make([]Rate, 0, len(cur))
	if elapsedSec <= 0 {
		for _, c := range cur {
			out = append(out, Rate{Iface: c.Iface})
		}
		return out
	}
	prevByIface := make(map[string]Sample, len(prev))
	for _, p := range prev {
		prevByIface[p.Iface] = p
	}
	for _, c := range cur {
		r := Rate{Iface: c.Iface}
		if p, ok := prevByIface[c.Iface]; ok {
			if c.RXBytes >= p.RXBytes {
				r.RXBps = float64(c.RXBytes-p.RXBytes) / elapsedSec
			}
			if c.TXBytes >= p.TXBytes {
				r.TXBps = float64(c.TXBytes-p.TXBytes) / elapsedSec
			}
		}
		out = append(out, r)
	}
	return out
}

// TopN 按总速率（RX+TX）降序返回前 n 个接口；n <= 0 时返回全部。
// 不修改入参切片。
func TopN(rates []Rate, n int) []Rate {
	cp := make([]Rate, len(rates))
	copy(cp, rates)
	sort.SliceStable(cp, func(i, j int) bool {
		return cp[i].RXBps+cp[i].TXBps > cp[j].RXBps+cp[j].TXBps
	})
	if n > 0 && n < len(cp) {
		cp = cp[:n]
	}
	return cp
}
