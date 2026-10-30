// Package ovpn: ParseClientList implementation (see status.go for types/helpers).
package ovpn

import (
	"strings"
)

func looksLikeClientHeader(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	joined := strings.ToLower(strings.Join(fields, ","))
	if !(strings.Contains(joined, "address") || strings.Contains(joined, "bytes") || strings.Contains(joined, "virtual") || strings.Contains(joined, "common")) {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(fields[0]))
	return strings.Contains(lower, "common") || strings.Contains(lower, "name") || strings.Contains(lower, "client")
}

func looksLikeRoutingHeader(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	joined := strings.ToLower(strings.Join(fields, ","))
	return strings.Contains(joined, "virtual") && (strings.Contains(joined, "routing") || strings.Contains(joined, "last ref"))
}

// ParseClientList parses OpenVPN management or status.log output into a Status.
func ParseClientList(text string) Status {
	st := Status{Clients: []Client{}}
	var clientHeader, routingHeader []string
	routingByCN := map[string]string{}
	routingByReal := map[string]string{}
	section := "" // "client" or "routing"

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
		joinedUpper := strings.ToUpper(line)

		// Section markers
		if (first == "ROUTING TABLE" || first == "ROUTING_TABLE") && len(fields) == 1 {
			section = "routing"
			continue
		}
		if (first == "GLOBAL STATS" || first == "GLOBAL_STATS" || first == "END") && len(fields) == 1 {
			section = ""
			continue
		}
		if (first == "OPENVPN CLIENT LIST" || first == "TITLE") && len(fields) == 1 {
			section = ""
			continue
		}
		if first == "UPDATED" {
			if len(fields) >= 2 && st.UpSince == "" {
				st.UpSince = strings.TrimSpace(fields[1])
			}
			continue
		}
		if first == "TIME" {
			if len(fields) >= 2 && st.UpSince == "" {
				st.UpSince = strings.TrimSpace(fields[1])
			}
			continue
		}

		if first == "HEADER" {
			if len(fields) >= 3 {
				sec := strings.ToUpper(strings.TrimSpace(fields[1]))
				names := trimFields(fields[2:])
				if sec == "CLIENT_LIST" {
					clientHeader = names
					section = "client"
				} else if sec == "ROUTING_TABLE" {
					routingHeader = names
					section = "routing"
				}
			}
			continue
		}

		// Bare header detection
		if clientHeader == nil && looksLikeClientHeader(fields) {
			clientHeader = trimFields(fields)
			section = "client"
			continue
		}
		if routingHeader == nil && looksLikeRoutingHeader(fields) {
			routingHeader = trimFields(fields)
			section = "routing"
			continue
		}

		// Management-format keyword rows
		if first == "CLIENT_LIST" {
			data := trimFields(fields[1:])
			hdr := clientHeader
			if hdr == nil {
				hdr = defaultClientHeader(len(data))
			}
			c := parseClientRow(hdr, data)
			st.Clients = append(st.Clients, c)
			section = "client"
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
			section = "routing"
			continue
		}

		// Bare data rows (real status.log format)
		if section == "client" && clientHeader != nil && len(fields) == len(clientHeader) {
			// ensure it's not a header itself
			if !looksLikeClientHeader(fields) {
				c := parseClientRow(clientHeader, trimFields(fields))
				st.Clients = append(st.Clients, c)
				continue
			}
		}
		if section == "routing" && routingHeader != nil && len(fields) == len(routingHeader) {
			if !looksLikeRoutingHeader(fields) {
				data := trimFields(fields)
				va := fieldByName(routingHeader, data, "virtual")
				cn := fieldByName(routingHeader, data, "common")
				ra := fieldByName(routingHeader, data, "real")
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
		_ = joinedUpper
	}

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
