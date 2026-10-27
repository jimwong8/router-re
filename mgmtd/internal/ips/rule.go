// Package ips 提供 Suricata 风格入侵检测规则的解析、校验与匹配，
// 以及封禁 IP 列表到 nftables blackhole set 的渲染。
//
// 解析/匹配引擎是纯 Go，不依赖宿主机 Suricata；nft 下发沿用
// internal/acl 的 delete-table + nft -f - 模式（绝不用 flush ruleset）。
package ips

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Rule 是一条 Suricata 风格告警/阻断规则。
type Rule struct {
	SID          int    `json:"sid"`
	Msg          string `json:"msg"`
	Proto        string `json:"proto"`         // tcp/udp/icmp/any
	SrcPort      string `json:"src_port"`      // any | <port> | a:b
	DstPort      string `json:"dst_port"`      // any | <port> | a:b
	PayloadHex   string `json:"payload_hex"`   // |xx xx| 解码后的字节（string of bytes）
	PayloadAscii string `json:"payload_ascii"` // 字面 ASCII 片段
	Flow         string `json:"flow"`          // Suricata flow: 选项
	Action       string `json:"action"`        // alert/drop/pass
	Enabled      bool   `json:"enabled"`
	Severity     int    `json:"severity"` // 1-4；0 视为 3
}

var validActions = map[string]bool{"alert": true, "drop": true, "pass": true}
var validProtos = map[string]bool{"tcp": true, "udp": true, "icmp": true, "any": true}

// ParseRule 解析一行 Suricata 风格告警行。
// 例如：alert tcp any any -> any 80 (msg:"ET WEB SQL Injection"; content:"union select"; nocase; sid:1000001; rev:1; classification:web-application-attack; priority:2;)
// content 内的 \|xx xx| 十六进制与 \" 转义/ \\ 反斜杠会被正确处理。
func ParseRule(line string) (Rule, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return Rule{}, fmt.Errorf("empty or comment line")
	}

	// 分离 header 与 options。
	paren := strings.Index(line, "(")
	if paren < 0 {
		return Rule{}, fmt.Errorf("missing options section '()' in rule")
	}
	header := strings.TrimSpace(line[:paren])
	body := strings.TrimSpace(line[paren+1:])
	// body 结尾的 ')'。
	if i := strings.LastIndex(body, ")"); i >= 0 {
		body = strings.TrimSpace(body[:i])
	}

	fields := strings.Fields(header)
	// 期望: action proto src srcport -> dst dstport
	if len(fields) < 7 || fields[4] != "->" {
		return Rule{}, fmt.Errorf("malformed rule header (expected 'action proto src sport -> dst dport'): %s", header)
	}
	r := Rule{
		Action:   fields[0],
		Proto:    fields[1],
		SrcPort:  fields[3],
		DstPort:  fields[6],
		Enabled:  true,
		Severity: 3, // 默认
	}

	// options 解析：按 ; 拆，但跳过引号内的 ;。
	opts := splitOptions(body)
	var asciiAcc strings.Builder
	var hexAcc []byte
	for _, opt := range opts {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}
		key := opt
		val := ""
		if i := strings.IndexByte(opt, ':'); i >= 0 {
			key = strings.TrimSpace(opt[:i])
			val = strings.TrimSpace(opt[i+1:])
		}
		switch key {
		case "msg":
			r.Msg = unquote(val)
		case "content":
			a, h := decodeContent(unquote(val))
			asciiAcc.WriteString(a)
			hexAcc = append(hexAcc, h...)
		case "sid":
			if n, err := strconv.Atoi(val); err == nil {
				r.SID = n
			}
		case "priority":
			if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= 4 {
				r.Severity = n
			}
		case "flow":
			r.Flow = val
		case "nocase":
			// 在 Match 里统一大小写不敏感，这里不额外存储。
		}
	}
	r.PayloadAscii = asciiAcc.String()
	r.PayloadHex = string(hexAcc)
	return r, nil
}

// splitOptions 按分号拆分 options，跳过引号内的分号（处理 \" 转义）。
func splitOptions(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
			cur.WriteByte(c)
			cur.WriteByte(s[i+1])
			i += 2
			continue
		}
		if c == '"' {
			inQuote = !inQuote
			cur.WriteByte(c)
			i++
			continue
		}
		if c == ';' && !inQuote {
			out = append(out, cur.String())
			cur.Reset()
			i++
			continue
		}
		cur.WriteByte(c)
		i++
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// unquote 去掉 value 外层引号（Suricata 的 msg:"..." content:"..."）。
func unquote(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		return val[1 : len(val)-1]
	}
	return val
}

// decodeContent 解析 content 值中的 \|xx xx| 与 \" \\ 转义。
// 返回 (ascii chars, hex bytes)。
func decodeContent(body string) (string, string) {
	var a strings.Builder
	var h []byte
	i := 0
	for i < len(body) {
		switch {
		case body[i] == '\\' && i+1 < len(body) && (body[i+1] == '"' || body[i+1] == '\\'):
			a.WriteByte(body[i+1])
			i += 2
		case body[i] == '|':
			j := strings.IndexByte(body[i+1:], '|')
			if j < 0 {
				a.WriteByte('|')
				i++
				continue
			}
			j += i + 1 // 第二个 | 的位置
			hexPart := strings.ReplaceAll(body[i+1:j], " ", "")
			if decoded, err := hex.DecodeString(hexPart); err == nil {
				h = append(h, decoded...)
			}
			i = j + 1
		default:
			a.WriteByte(body[i])
			i++
		}
	}
	return a.String(), string(h)
}

// parsePortSpec 报告端口 spec 是否合法（any 或 1-65535 或 a:b）。
func parsePortSpec(s string) bool {
	s = strings.TrimSpace(s)
	if s == "any" || s == "" || s == "*" {
		return true
	}
	if i := strings.IndexByte(s, ':'); i > 0 {
		a, ok1 := strconv.Atoi(s[:i])
		b, ok2 := strconv.Atoi(s[i+1:])
		if ok1 != nil || ok2 != nil {
			return false
		}
		return a >= 1 && a <= 65535 && b >= 1 && b <= 65535 && a <= b
	}
	n, ok := strconv.Atoi(s)
	return ok == nil && n >= 1 && n <= 65535
}

// portMatch 检查 dst 端口是否命中 spec。
func portMatch(spec string, dst int) bool {
	spec = strings.TrimSpace(spec)
	if spec == "any" || spec == "" || spec == "*" {
		return true
	}
	if i := strings.IndexByte(spec, ':'); i > 0 {
		a, ok1 := strconv.Atoi(spec[:i])
		b, ok2 := strconv.Atoi(spec[i+1:])
		if ok1 != nil || ok2 != nil || a > b {
			return false
		}
		return dst >= a && dst <= b
	}
	p, err := strconv.Atoi(spec)
	if err != nil {
		return false
	}
	return dst == p
}

// Validate 校验规则结构。
// 注意：error 永远不会带上任何密码/密文（本结构无密码，但保持约定）。
func (r Rule) Validate() error {
	if r.SID <= 0 {
		return fmt.Errorf("sid must be > 0")
	}
	if !validActions[r.Action] {
		return fmt.Errorf("action must be alert/drop/pass, got %q", r.Action)
	}
	if !validProtos[strings.ToLower(r.Proto)] {
		return fmt.Errorf("proto must be tcp/udp/icmp/any, got %q", r.Proto)
	}
	if !parsePortSpec(r.SrcPort) {
		return fmt.Errorf("invalid src_port %q", r.SrcPort)
	}
	if !parsePortSpec(r.DstPort) {
		return fmt.Errorf("invalid dst_port %q", r.DstPort)
	}
	if strings.TrimSpace(r.Msg) == "" {
		return fmt.Errorf("msg must not be empty")
	}
	if !(r.Severity >= 1 && r.Severity <= 4) {
		return fmt.Errorf("severity must be 1-4, got %d", r.Severity)
	}
	if r.PayloadAscii == "" && r.PayloadHex == "" {
		return fmt.Errorf("payload_ascii/payload_hex cannot both be empty")
	}
	return nil
}

// Match 执行一次真实匹配。
func (r Rule) Match(proto string, dstPort int, payload []byte) bool {
	if r.Proto != "any" && strings.ToLower(r.Proto) != strings.ToLower(proto) {
		return false
	}
	if !portMatch(r.DstPort, dstPort) {
		return false
	}
	if r.PayloadAscii != "" {
		return bytes.Contains(bytes.ToLower(payload), bytes.ToLower([]byte(r.PayloadAscii)))
	}
	if r.PayloadHex != "" {
		return bytes.Contains(payload, []byte(r.PayloadHex))
	}
	return true
}

// ipStrForValidation is intentionally removed: IPS rules have no trap concept.
