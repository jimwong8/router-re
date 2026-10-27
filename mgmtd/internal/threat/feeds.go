package threat

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// ParseFeed 解析纯文本 IP/CIDR 列表。每行一个条目，支持：
//   - `#` 注释行
//   - 行尾 `# 注释`
//   - CIDR
//   - 前后空格
//
// 无法解析的行会被跳过，但**不会静默丢弃**——跳过数量通过返回的 error 说明
// （成功时 error 为 nil；只要有跳过的行，就返回一个说明跳过了多少行的非 nil error）。
// 这样调用方既能看到解析成功的条目，也能知道数据里有多少噪音。
func ParseFeed(text string) ([]Entry, error) {
	entries, skipped, skipErr := ParseFeedCount(text)
	if skipErr != nil {
		return entries, skipErr
	}
	if skipped > 0 {
		// 理论不可达（ParseFeedCount 在 skipped>0 时必返回非 nil err），防御兜底。
		return entries, fmt.Errorf("解析 %d 行失败被跳过", skipped)
	}
	return entries, nil
}

// ParseFeedCount 解析纯文本 IP/CIDR 列表，显式返回被跳过的行数。
// 返回 (成功条目, 跳过数, 说明跳过的 error)。有跳过的行时 err 非 nil，
// 但 entries 仍含所有成功解析的条目——调用方可据此精确上报坏行数。
func ParseFeedCount(text string) ([]Entry, int, error) {
	var entries []Entry
	skipped := 0
	var skipReasons []string
	lines := strings.Split(text, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// 行尾注释：去掉第一个 '#' 之后的内容（注意 '#' 前需是空格或行首才算注释，
		// 但本设计按"整行为纯列表"约定，直接切分即可）。
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			// 整行是注释
			continue
		}
		e := Entry{IP: line}
		if err := validateIPPure(line); err != nil {
			skipped++
			skipReasons = append(skipReasons, fmt.Sprintf("line %d: %v", i+1, err))
			continue
		}
		entries = append(entries, e)
	}
	if skipped > 0 {
		return entries, skipped, fmt.Errorf("解析 %d 行失败被跳过（共 %d 个原因）: %s",
			skipped, len(skipReasons), strings.Join(shortenReasons(skipReasons), "; "))
	}
	return entries, skipped, nil
}

func shortenReasons(reasons []string) []string {
	const max = 3
	if len(reasons) <= max {
		return reasons
	}
	return append(reasons[:max], fmt.Sprintf("... 等共 %d 条", len(reasons)))
}

// validateIPPure 只校验 IP/CIDR 语法，不做业务校验（如封全网）。
func validateIPPure(s string) error {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return nil
	}
	return fmt.Errorf("既不是合法 IP 也不是合法 CIDR")
}

// FetchFeed 从远端 URL 拉取情报源文本并走 ParseFeed 解析。
//
// **SSRF 防护**：url 必须先解析并校验 scheme 为 http/https 且 host 非空——
// file:// 能读本机任意文件、gopher:// 能打内网服务，二者都是 SSRF 出口。这里在
// 代码里写死白名单，防止把管理员配置的一个"脏 URL"变成攻击本机的通道。
func FetchFeed(r execx.Runner, rawURL string) ([]Entry, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("url 解析失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("拒绝非 http/https 的情报源 scheme %q（防 SSRF，仅允许 http/https）", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("url 缺少 host（防 SSRF，必须显式指定目标主机）")
	}
	if r == nil {
		r = execx.DefaultRunner{}
	}
	out, err := r.Run("curl", "-m", "15", "-fsSL", rawURL)
	if err != nil {
		return nil, fmt.Errorf("拉取情报源失败: %w", err)
	}
	return ParseFeed(out)
}
