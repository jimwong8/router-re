// Package dpi 提供应用识别（DPI）的配置模型、规则渲染与可插拔分类器抽象。
//
// 本模块不做真正的深度包检测（那需要 nDPI 等库），而是提供：
//   - AppRule 配置模型：把应用规则（按 host / dport / proto）抽象为结构化数据
//   - RenderNftRules：把规则渲染成 nftables 文本（可下发到内核）
//   - Classifier 接口：作为真实 DPI（如 nDPI）的可插拔抽象，供后续接入
//   - NewDportClassifier：基于端口匹配的简单实现（无外部依赖）
package dpi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrUnsupported 表示当前构建/运行环境不支持 nDPI（无 cgo 或非 linux）。
// 由 ndpi_stub.go（非 cgo 环境）返回；ndpi.go（cgo 环境）正常返回 nil error。
var ErrUnsupported = errors.New("nDPI not supported in this build (requires linux && cgo)")

// Flow 描述一个待分类的网络流（供 Classifier 使用）。
type Flow struct {
	SrcAddr string // 源地址
	DstAddr string // 目的地址
	SrcPort uint16 // 源端口
	DstPort uint16 // 目的端口
	Proto   string // "tcp" / "udp" / ""
}

// AppRule 描述一条应用识别规则。
type AppRule struct {
	Name       string `json:"name"`                // 应用名，如 "netflix"
	Category   string `json:"category"`            // 分类："video"/"game"/"p2p"/"voip"/"other"
	MatchType  string `json:"match_type"`          // 匹配方式："host"/"dport"/"proto"
	MatchValue string `json:"match_value"`         // 匹配值：host=域名、dport=端口、proto=tcp/udp
	Action     string `json:"action"`              // "accept"/"drop"/"limit"
	RateMbps   int    `json:"rate_mbps,omitempty"` // limit 时的限速（Mbps）
	Priority   int    `json:"priority,omitempty"`  // 优先级，越大越先匹配
}

// Classifier 是应用识别的可插拔抽象。
//
// 真实实现（如 nDPI）返回 AppRule 而非具体库类型，使得调用方无需感知
// 底层识别引擎。当前只提供基于端口的简单实现 NewDportClassifier。
type Classifier interface {
	// Classify 对一条流分类，命中则返回规则与 true，否则 false。
	Classify(flow Flow) (AppRule, bool)
}

func BuildTCPPacket(dstPort uint16) []byte {
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], 40)
	binary.BigEndian.PutUint16(pkt[6:8], 0x4000)
	pkt[8] = 64
	pkt[9] = 6
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	pkt[32] = 0x50
	pkt[33] = 0x02
	binary.BigEndian.PutUint16(pkt[34:36], 65535)
	return pkt
}

// RenderNftRules 将应用规则渲染为 nftables 规则文本。
//
// 输出结构：
//   - proto 匹配：`tcp` / `udp` → `meta l4proto tcp`
//   - dport 匹配：`tcp/53` → `tcp dport 53`；`udp/53` → `udp dport 53`（未指定 proto 时同时匹配 tcp+udp）
//   - host 匹配：`ip daddr <addr>`
//   - action：accept / drop / limit（`limit rate <N> mbytes/second`）
//
// 规则按 Priority 降序排列后渲染。
func RenderNftRules(rules []AppRule) string {
	// 按 Priority 降序（优先级高在前）
	sorted := append([]AppRule(nil), rules...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority > sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	var b strings.Builder
	for _, r := range sorted {
		// 拼 match 条件
		var match string
		switch r.MatchType {
		case "proto":
			// MatchValue = "tcp" / "udp"
			if r.MatchValue == "tcp" || r.MatchValue == "udp" {
				match = "meta l4proto " + r.MatchValue
			}
		case "dport":
			port := r.MatchValue
			proto := "tcp udp" // 默认 tcp+udp
			if i := strings.IndexByte(port, '/'); i >= 0 {
				proto = port[:i]
				port = port[i+1:]
			}
			// 解析端口数字
			if _, err := strconv.Atoi(port); err == nil {
				switch proto {
				case "tcp":
					match = "tcp dport " + port
				case "udp":
					match = "udp dport " + port
				default:
					match = "tcp dport " + port + " or udp dport " + port
				}
			}
		case "host":
			if r.MatchValue != "" {
				match = "ip daddr " + r.MatchValue
			}
		}

		if match == "" {
			continue // 无法渲染的规则跳过
		}

		// 拼 action
		var act string
		switch r.Action {
		case "accept":
			act = "accept"
		case "drop":
			act = "drop"
		case "limit":
			rate := r.RateMbps
			if rate <= 0 {
				rate = 10
			}
			act = fmt.Sprintf("limit rate %d mbytes/second", rate)
		default:
			continue
		}

		b.WriteString(match + " " + act + "\n")
	}
	return b.String()
}

// dportClassifier 是基于端口匹配的简单 Classifier 实现。
type dportClassifier struct {
	rules []AppRule
}

// NewDportClassifier 返回一个按目标端口匹配的 Classifier。
// 匹配规则：MatchType=="dport" 且 Flow.DstPort == MatchValue（支持 "tcp/53" 格式）。
func NewDportClassifier(rules []AppRule) Classifier {
	return &dportClassifier{rules: rules}
}

// Classify 在规则列表中查找第一条命中 Flow 的规则（按 Priority 降序）。
func (c *dportClassifier) Classify(flow Flow) (AppRule, bool) {
	// 排序规则：Priority 高者优先
	sorted := append([]AppRule(nil), c.rules...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority > sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	for _, r := range sorted {
		if r.MatchType != "dport" {
			continue
		}
		// 解析 MatchValue："53" 或 "tcp/53" / "udp/53"
		val := r.MatchValue
		proto := ""
		if i := strings.IndexByte(val, '/'); i >= 0 {
			proto = val[:i]
			val = val[i+1:]
		}
		port, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		if uint16(port) == flow.DstPort {
			if proto == "" || proto == flow.Proto {
				return r, true
			}
		}
	}
	return AppRule{}, false
}
