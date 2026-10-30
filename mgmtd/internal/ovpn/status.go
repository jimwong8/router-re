// Package ovpn parses OpenVPN management-interface status text.
//
// OpenVPN's management interface emits status information either as the
// reply to a "status" command (comma-separated CLIENT_LIST / ROUTING_TABLE
// sections) or as real-time ">STATUS" notifications. This file turns that
// text into structured values without shelling out to openvpn/openssl.
package ovpn

import (
	"fmt"
	"strconv"
	"strings"
)

// Client is one connected OpenVPN peer as reported by the management interface.
type Client struct {
	CommonName     string `json:"common_name"`
	RealAddress    string `json:"real_address"`
	VirtualAddress string `json:"virtual_address"`
	BytesReceived  int64  `json:"bytes_received"`
	BytesSent      int64  `json:"bytes_sent"`
	ConnectedSince string `json:"connected_since"`
}

// Status is the aggregated view of an OpenVPN server's status output.
type Status struct {
	UpSince       string   `json:"up_since"`
	Clients       []Client `json:"clients"`
	TotalBytesIn  int64    `json:"total_bytes_in"`
	TotalBytesOut int64    `json:"total_bytes_out"`
}

// ParseClientList parses the CLIENT_LIST (+ ROUTING_TABLE) sections of
// OpenVPN management output into a Status. Field names are taken from the
// HEADER rows when present; variants without headers fall back to the
// conventional positional order. Bad/truncated lines are skipped.
func ParseClientList(text string) Status {
	st := Status{Clients: []Client{}}
	var clientHeader, routingHeader []string
	routingByCN := map[string]string{}
	routingByReal := map[string]string{}

	for _, raw := range strings.Split(text, "\n") {
		line := normLine(raw)
		if line == "" {
			continue
		}
		fields := splitCSV(line)
		if len(fields) == 0 {
			continue
		}
		first := strings.ToUpper(strings.TrimSpace(fields[0]))

		if first == "HEADER" {
			if len(fields) >= 3 {
				sec := strings.ToUpper(strings.TrimSpace(fields[1]))
				names := trimFields(fields[2:])
				if sec == "CLIENT_LIST" {
					clientHeader = names
				} else if sec == "ROUTING_TABLE" {
					routingHeader = names
				}
			}
			continue
		}

		if first == "TIME" {
			if len(fields) >= 2 && st.UpSince == "" {
				st.UpSince = strings.TrimSpace(fields[1])
			}
			continue
		}

		if first == "CLIENT_LIST" {
			data := trimFields(fields[1:])
			hdr := clientHeader
			if hdr == nil {
				hdr = defaultClientHeader(len(data))
			}
			c := parseClientRow(hdr, data)
			st.Clients = append(st.Clients, c)
			continue
		}

		if first == "ROUTING_TABLE" {
			data := trimFields(fields[1:])
			hdr := routingHeader
			if hdr == nil {
				hdr = defaultRoutingHeader(len(data))
			}
			va := fieldByName(hdr, data, "virtual")
			cn := fieldByName(hdr, data, "common")
			ra := fieldByName(hdr, data, "real")
			if va != "" {
				if cn != "" {
					routingByCN[cn] = va
				}
				if ra != "" {
					routingByReal[ra] = va
				}
			}
			continue
		}
	}

	// Merge virtual addresses from the routing table into clients that did
	// not already carry one (header-less or non-standard client rows).
	for i := range st.Clients {
		if st.Clients[i].VirtualAddress != "" {
			continue
		}
		if va, ok := routingByCN[st.Clients[i].CommonName]; ok {
			st.Clients[i].VirtualAddress = va
		} else if va, ok := routingByReal[st.Clients[i].RealAddress]; ok {
			st.Clients[i].VirtualAddress = va
		}
	}
	return st
}

// ParseStatus parses the ">STATUS" section, extracting cumulative traffic
// from TOTAL lines. Bad input is skipped without panicking.
func ParseStatus(text string) Status {
	st := Status{Clients: []Client{}}
	for _, raw := range strings.Split(text, "\n") {
		line := normLine(raw)
		if line == "" {
			continue
		}
		fields := splitCSV(line)
		if len(fields) == 0 {
			continue
		}
		first := strings.ToUpper(strings.TrimSpace(fields[0]))

		if first == "STATUS" {
			continue
		}
		if first == "END" {
			continue
		}
		if first == "TIME" && st.UpSince == "" && len(fields) >= 2 {
			st.UpSince = strings.TrimSpace(fields[1])
			continue
		}
		if strings.HasPrefix(first, "UP") || first == "STARTED" || first == "SINCE" || first == "UPTIME" {
			if len(fields) >= 2 {
				st.UpSince = strings.TrimSpace(fields[1])
			}
			continue
		}

		if first == "TOTAL" {
			// TOTAL,<in>,<out>
			if len(fields) >= 3 {
				st.TotalBytesIn = atoi(fields[1])
				st.TotalBytesOut = atoi(fields[2])
				continue
			}
			// TOTAL IN <n> / TOTAL OUT <n> style: first=TOTAL, second=IN/OUT
			if len(fields) >= 2 {
				kind := strings.ToUpper(strings.TrimSpace(fields[1]))
				if kind == "IN" || kind == "RX" || kind == "RECEIVED" {
					if len(fields) >= 3 {
						st.TotalBytesIn = atoi(fields[2])
					} else {
						st.TotalBytesIn = extractFirstInt(line)
					}
					continue
				}
				if kind == "OUT" || kind == "TX" || kind == "SENT" {
					if len(fields) >= 3 {
						st.TotalBytesOut = atoi(fields[2])
					} else {
						st.TotalBytesOut = extractFirstInt(line)
					}
					continue
				}
			}
			// Fallback: grab first two integers on the line.
			nums := extractInts(line)
			if len(nums) >= 2 {
				st.TotalBytesIn = nums[0]
				st.TotalBytesOut = nums[1]
			} else if len(nums) == 1 {
				st.TotalBytesIn = nums[0]
			}
			continue
		}

		if strings.HasPrefix(first, "TOTAL") {
			// e.g. "TOTAL_IN,123" or "TOTAL-OUT 456"
			joined := strings.ToUpper(line)
			// Prefer keyword-anchored numbers (handles "TOTAL RX 3 TX 4").
			if v, ok := numberAfterKeyword(line, "RX"); ok {
				st.TotalBytesIn = v
			} else if v, ok := numberAfterKeyword(line, "IN"); ok {
				st.TotalBytesIn = v
			} else if v, ok := numberAfterKeyword(line, "RECEIVED"); ok {
				st.TotalBytesIn = v
			}
			if v, ok := numberAfterKeyword(line, "TX"); ok {
				st.TotalBytesOut = v
			} else if v, ok := numberAfterKeyword(line, "OUT"); ok {
				st.TotalBytesOut = v
			} else if v, ok := numberAfterKeyword(line, "SENT"); ok {
				st.TotalBytesOut = v
			}
			if st.TotalBytesIn == 0 && st.TotalBytesOut == 0 {
				n := extractFirstInt(line)
				if strings.Contains(joined, "IN") || strings.Contains(joined, "RX") || strings.Contains(joined, "RECEIVED") {
					st.TotalBytesIn = n
				}
				if strings.Contains(joined, "OUT") || strings.Contains(joined, "TX") || strings.Contains(joined, "SENT") {
					st.TotalBytesOut = n
				}
			}
			continue
		}
	}
	return st
}

// --- helpers ---

func normLine(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, ">")
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "\r")
	return s
}

func splitCSV(s string) []string {
	return strings.Split(s, ",")
}

func trimFields(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.TrimSpace(v)
	}
	return out
}

func atoi(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func extractFirstInt(s string) int64 {
	nums := extractInts(s)
	if len(nums) == 0 {
		return 0
	}
	return nums[0]
}

func extractInts(s string) []int64 {
	var out []int64
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			if n, err := strconv.ParseInt(buf.String(), 10, 64); err == nil {
				out = append(out, n)
			} else {
				// Preserve positional alignment on parse failure.
				out = append(out, 0)
			}
			buf.Reset()
		}
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			buf.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func defaultClientHeader(n int) []string {
	switch {
	case n >= 6:
		return []string{"Common Name", "Real Address", "Virtual Address", "Bytes Received", "Bytes Sent", "Connected Since"}
	case n == 5:
		return []string{"Common Name", "Real Address", "Bytes Received", "Bytes Sent", "Connected Since"}
	case n == 4:
		return []string{"Common Name", "Real Address", "Bytes Received", "Bytes Sent"}
	case n == 3:
		return []string{"Common Name", "Real Address", "Connected Since"}
	case n == 2:
		return []string{"Common Name", "Real Address"}
	case n == 1:
		return []string{"Common Name"}
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("col%d", i)
	}
	return out
}

func defaultRoutingHeader(n int) []string {
	switch {
	case n >= 3:
		return []string{"Virtual Address", "Common Name", "Real Address"}
	case n == 2:
		return []string{"Virtual Address", "Common Name"}
	case n == 1:
		return []string{"Virtual Address"}
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("col%d", i)
	}
	return out
}

func parseClientRow(hdr, data []string) Client {
	c := Client{}
	for i, name := range hdr {
		if i >= len(data) {
			break
		}
		val := strings.TrimSpace(data[i])
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "common"):
			c.CommonName = val
		case strings.Contains(lower, "real"):
			c.RealAddress = val
		case strings.Contains(lower, "virtual") && !strings.Contains(lower, "ipv6"):
			c.VirtualAddress = val
		case strings.Contains(lower, "received") || strings.Contains(lower, "bytes in") || strings.Contains(lower, "rx") || strings.Contains(lower, "in bytes"):
			c.BytesReceived = atoi(val)
		case strings.Contains(lower, "sent") || strings.Contains(lower, "bytes out") || strings.Contains(lower, "tx") || strings.Contains(lower, "out bytes"):
			// avoid matching "received" already handled above; "sent" distinct
			c.BytesSent = atoi(val)
		case strings.Contains(lower, "connect"):
			c.ConnectedSince = val
		}
	}
	// received is handled before sent by switch order.
	return c
}

func numberAfterKeyword(s, kw string) (int64, bool) {
	upper := strings.ToUpper(s)
	kwU := strings.ToUpper(kw)
	idx := 0
	for {
		i := strings.Index(upper[idx:], kwU)
		if i < 0 {
			return 0, false
		}
		pos := idx + i
		// word boundary check
		leftOK := pos == 0 || !isAlnum(rune(s[pos-1]))
		rightPos := pos + len(kwU)
		rightOK := rightPos >= len(s) || !isAlnum(rune(s[rightPos]))
		if leftOK && rightOK {
			// find digit after keyword
			j := rightPos
			for j < len(s) && !(s[j] >= '0' && s[j] <= '9') {
				j++
			}
			if j >= len(s) {
				return 0, false
			}
			start := j
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			n, err := strconv.ParseInt(s[start:j], 10, 64)
			if err != nil {
				return 0, false
			}
			return n, true
		}
		idx = pos + 1
		if idx >= len(s) {
			return 0, false
		}
	}
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func fieldByName(hdr, data []string, key string) string {
	for i, name := range hdr {
		if i >= len(data) {
			break
		}
		if strings.Contains(strings.ToLower(name), key) {
			return strings.TrimSpace(data[i])
		}
	}
	return ""
}
