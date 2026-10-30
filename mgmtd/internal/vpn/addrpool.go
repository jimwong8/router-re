package vpn

import (
	"fmt"
	"net"
	"strings"
)

// AddrPool 是 iKuai 风格的地址池，写成 "起始IP-结束IP" 的多段逗号分隔字符串，
// 例如 "192.168.20.100-192.168.20.200,192.168.21.100-192.168.21.150"。
// 单段也可以写成 "192.168.20.100-200"（结束段省略前缀）。
//
// 用字符串而不是结构化切片，是为了和 iKuai 的 addr_pool 字段逐字对应：
// 迁移配置时不需要重新拼装，比对配置差异时也是同一份文本。
type AddrPool struct {
	Raw    string
	Ranges []IPRange
}

// IPRange 是闭区间 [Start, End]。
type IPRange struct {
	Start net.IP
	End   net.IP
}

// ParseAddrPool 解析地址池字符串。空串返回空池（不报错，表示"未配置"）。
func ParseAddrPool(raw string) (AddrPool, error) {
	p := AddrPool{Raw: strings.TrimSpace(raw)}
	if p.Raw == "" {
		return p, nil
	}
	parts := strings.Split(p.Raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		r, err := parseIPRange(part)
		if err != nil {
			return p, err
		}
		p.Ranges = append(p.Ranges, r)
	}
	if len(p.Ranges) > 16 {
		return p, fmt.Errorf("地址池最多 16 段，当前 %d 段", len(p.Ranges))
	}
	return p, nil
}

func parseIPRange(s string) (IPRange, error) {
	var r IPRange
	if !strings.Contains(s, "-") {
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() == nil {
			return r, fmt.Errorf("地址池条目不是合法 IPv4 或区间：%s", s)
		}
		r.Start, r.End = ip.To4(), ip.To4()
		return r, nil
	}
	segs := strings.SplitN(s, "-", 2)
	startStr := strings.TrimSpace(segs[0])
	endStr := strings.TrimSpace(segs[1])
	start := net.ParseIP(startStr)
	if start == nil || start.To4() == nil {
		return r, fmt.Errorf("地址池起点不是合法 IPv4：%s", startStr)
	}
	// 允许简写：192.168.20.100-200
	if !strings.Contains(endStr, ".") {
		prefix := startStr[:strings.LastIndex(startStr, ".")+1]
		endStr = prefix + endStr
	}
	end := net.ParseIP(endStr)
	if end == nil || end.To4() == nil {
		return r, fmt.Errorf("地址池终点不是合法 IPv4：%s", endStr)
	}
	if ip4ToUint(start.To4()) > ip4ToUint(end.To4()) {
		return r, fmt.Errorf("地址池起点大于终点：%s", s)
	}
	r.Start, r.End = start.To4(), end.To4()
	return r, nil
}

// Total 返回地址池可用地址总数。
func (p AddrPool) Total() int {
	n := 0
	for _, r := range p.Ranges {
		n += int(ip4ToUint(r.End)-ip4ToUint(r.Start)) + 1
	}
	return n
}

// Contains 判断地址是否落在池内。
func (p AddrPool) Contains(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return false
	}
	v := ip4ToUint(ip.To4())
	for _, r := range p.Ranges {
		if v >= ip4ToUint(r.Start) && v <= ip4ToUint(r.End) {
			return true
		}
	}
	return false
}

// Validate 校验地址池：非空、总量不超过 1024、不含网络号/广播号。
func (p AddrPool) Validate(required bool) error {
	if len(p.Ranges) == 0 {
		if required {
			return fmt.Errorf("地址池不能为空")
		}
		return nil
	}
	if p.Total() > 1024 {
		return fmt.Errorf("地址池过大（%d 个地址，最多 1024）", p.Total())
	}
	for _, r := range p.Ranges {
		if r.Start.To4()[3] == 0 {
			return fmt.Errorf("地址池起点 %s 是网络号，请避开 .0", r.Start)
		}
		if r.End.To4()[3] == 255 {
			return fmt.Errorf("地址池终点 %s 是广播号，请避开 .255", r.End)
		}
	}
	return nil
}
