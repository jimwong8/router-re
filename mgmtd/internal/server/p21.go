package server

import (
	"net/http"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/fwlog"
)

// ---------- 防火墙日志分析 ----------

func collectFwLogLines() ([]string, string) {
	// 优先从内核日志抓含 SRC= 的行
	out, err := (execx.DefaultRunner{}).Run("journalctl", "-k", "-n", "2000", "--no-pager")
	if err == nil {
		lines := []string{}
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "SRC=") {
				lines = append(lines, l)
			}
		}
		if len(lines) > 0 {
			return lines, ""
		}
	}
	// 兜底：nft ruleset 里不会有 SRC=，但至少确认 nft 可用；不可用则如实报 unavailable
	if _, err := (execx.DefaultRunner{}).Run("nft", "list", "ruleset"); err != nil {
		reason := "journalctl/nft 均不可用"
		if err != nil {
			reason = err.Error()
		}
		return nil, reason
	}
	return nil, "未找到含 SRC= 的防火墙日志行"
}

// handleFwLogAnalysis GET /api/v1/firewall/log/analysis
func (s *Server) handleFwLogAnalysis(w http.ResponseWriter, r *http.Request) {
	lines, reason := collectFwLogLines()
	if lines == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"reason":    reason,
		})
		return
	}
	evts := fwlog.ParseLines(lines)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available":   true,
		"count":       len(evts),
		"summary":     fwlog.Summarize(evts),
		"top_talkers": fwlog.TopTalkers(evts, 10),
		"suspicious":  fwlog.Suspicious(evts, 60),
		"suggestions": fwlog.SuggestBlock(evts, 50),
	})
}

// handleFwLogSuggestions GET /api/v1/firewall/log/suggestions
func (s *Server) handleFwLogSuggestions(w http.ResponseWriter, r *http.Request) {
	lines, reason := collectFwLogLines()
	if lines == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"reason":    reason,
			"applied":   false,
			"note":      "这些只是建议，不会自动封禁",
		})
		return
	}
	evts := fwlog.ParseLines(lines)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available":   true,
		"suggestions": fwlog.SuggestBlock(evts, 50),
		"applied":     false,
		"note":        "这些只是建议，不会自动封禁",
	})
}
