package firewall

import (
    "fmt"
    "strings"
)

func RenderNftables(rules []Rule) string {
    var b strings.Builder
    b.WriteString("table inet router {\n")
    b.WriteString("  chain input { type filter hook input priority 0; policy accept; }\n")
    b.WriteString("  chain forward { type filter hook forward priority 0; policy accept; }\n")
    for _, r := range rules {
        match := []string{}
        if r.Src != "" {
            match = append(match, fmt.Sprintf("ip saddr %s", r.Src))
        }
        if r.Dst != "" {
            match = append(match, fmt.Sprintf("ip daddr %s", r.Dst))
        }
        if r.Proto != "" {
        	if r.DPort != "" {
        		match = append(match, fmt.Sprintf("%s dport %s", r.Proto, r.DPort))
        	} else {
        		match = append(match, r.Proto)
        	}
        }
        cond := ""
        if len(match) > 0 {
            cond = strings.Join(match, " ") + " "
        }
        b.WriteString(fmt.Sprintf("    %s%s %s\n", cond, r.Action, ""))
    }
    b.WriteString("}\n")
    return b.String()
}
