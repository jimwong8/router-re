package threat

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

func TestParseFeedBasic(t *testing.T) {
	text := `203.0.113.5
198.51.100.0/24
10.0.0.7   # 末尾注释
 192.0.2.9  
`
	entries, err := ParseFeed(text)
	if err != nil {
		t.Fatalf("无坏行的纯列表不应报错, got %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("应解析 4 条, got %d", len(entries))
	}
	ips := map[string]bool{}
	for _, e := range entries {
		ips[e.IP] = true
	}
	for _, want := range []string{"203.0.113.5", "198.51.100.0/24", "10.0.0.7", "192.0.2.9"} {
		if !ips[want] {
			t.Fatalf("缺 %s, got %v", want, ips)
		}
	}
}

func TestParseFeedCommentLine(t *testing.T) {
	text := `# 这是注释
203.0.113.5
# another comment
198.51.100.9`
	entries, err := ParseFeed(text)
	if err != nil {
		t.Fatalf("注释行不应导致错误, got %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("注释行应被忽略, got %d", len(entries))
	}
}

// TestParseFeedSkipsBadLines 坏行被跳过但必须通过 error 说明跳过了多少行。
func TestParseFeedSkipsBadLines(t *testing.T) {
	text := `203.0.113.5
totally-garbage
198.51.100.9
also_bad
999.1.1.1   # 非法但格式像 IP
`
	entries, err := ParseFeed(text)
	if err == nil {
		t.Fatal("存在坏行时返回的 error 不能为 nil（不能静默丢弃）")
	}
	if len(entries) != 2 {
		t.Fatalf("好行应保留 2 条, got %d", len(entries))
	}
	if !strings.Contains(err.Error(), "跳过") {
		t.Fatalf("error 应说明跳过了多少行, got %v", err)
	}
	// 跳过 3 行坏行
	if !containsCount(err.Error(), "3") && !strings.Contains(err.Error(), "3 个原因") {
		t.Fatalf("应指明跳过 3 行, got %v", err)
	}
}

func containsCount(s, sub string) bool { return strings.Contains(s, sub) }

func TestParseFeedEmpty(t *testing.T) {
	entries, err := ParseFeed("")
	if err != nil {
		t.Fatalf("空输入应返回空列表无错误, got %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("空输入应 0 条, got %d", len(entries))
	}
}

func TestParseFeedOnlyComments(t *testing.T) {
	entries, err := ParseFeed("# only\n# comments\n")
	if err != nil {
		t.Fatalf("纯注释不应报错, got %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("纯注释应 0 条, got %d", len(entries))
	}
}

// structRunner 记录 curl 调用并返回固定文本。
type structRunner struct {
	out      string
	commands [][]string
}

func (s *structRunner) Run(name string, args ...string) (string, error) {
	s.commands = append(s.commands, append([]string{name}, args...))
	return s.out, nil
}
func (s *structRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.out, nil
}

func TestFetchFeedOK(t *testing.T) {
	r := &structRunner{out: "203.0.113.5\n198.51.100.0/24\n"}
	entries, err := FetchFeed(r, "https://feeds.example.com/list.txt")
	if err != nil {
		t.Fatalf("FetchFeed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("应解析 2 条, got %d", len(entries))
	}
	// 应调用 curl -m 15 -fsSL
	if len(r.commands) != 1 || r.commands[0][0] != "curl" {
		t.Fatalf("应调用 curl, got %v", r.commands)
	}
	joined := ""
	if len(r.commands) > 0 {
		joined = strings.Join(r.commands[0], " ")
	}
	if !strings.Contains(joined, "-m 15") || !strings.Contains(joined, "-fsSL") {
		t.Fatalf("curl 参数应含 -m 15 -fsSL, got %v", r.commands)
	}
}

// TestFetchFeedRejectsDangerousSchemes —— SSRF 红线。
func TestFetchFeedRejectsDangerousSchemes(t *testing.T) {
	r := &structRunner{out: "203.0.113.5\n"}
	for _, bad := range []string{
		"file:///etc/passwd",
		"gopher://internal:70/x",
		"file://localhost/etc/shadow",
		"ftp://host/file",
		"dict://host",
	} {
		_, err := FetchFeed(r, bad)
		if err == nil {
			t.Fatalf("危险 scheme %q 必须被拒绝", bad)
		}
		if !strings.Contains(err.Error(), "SSRF") && !strings.Contains(err.Error(), "http/https") {
			t.Fatalf("错误信息应说明 SSRF 原因, got %v", err)
		}
	}
	// 不应发任何 curl
	if len(r.commands) != 0 {
		t.Fatalf("危险 scheme 不应触发 curl, got %v", r.commands)
	}
}

func TestFetchFeedRejectsMissingHost(t *testing.T) {
	r := &structRunner{}
	for _, u := range []string{"http://", "https:///path"} {
		_, err := FetchFeed(r, u)
		if err == nil {
			t.Fatalf("缺失 host 的 url %q 应被拒绝（防 SSRF）", u)
		}
	}
	if len(r.commands) != 0 {
		t.Fatalf("不应触发 curl, got %v", r.commands)
	}
}

func TestFetchFeedNilRunner(t *testing.T) {
	// nil runner 走真实 DefaultRunner，这里只验证 scheme 校验先行拒绝
	_, err := FetchFeed(nil, "file:///etc/passwd")
	if err == nil {
		t.Fatal("file:// 应在触碰 runner 前被拒绝")
	}
}

var _ execx.Runner = (*structRunner)(nil)

func TestParseFeedCountReturnsSkipped(t *testing.T) {
	text := "203.0.113.5\ntotally-bad\n198.51.100.9\nalso-bad\n"
	entries, skipped, err := ParseFeedCount(text)
	if skipped != 2 {
		t.Fatalf("skipped=%d (want 2)", skipped)
	}
	if err == nil {
		t.Fatal("有坏行时 err 不能为 nil")
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d (want 2)", len(entries))
	}
}

func TestParseFeedCountNoBadLines(t *testing.T) {
	entries, skipped, err := ParseFeedCount("203.0.113.5\n198.51.100.0/24\n")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("skipped=%d (want 0)", skipped)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d (want 2)", len(entries))
	}
}
