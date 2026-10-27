package quota

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// stubRunner 记录调用并存 output。
type stubRunner struct {
	runSeq        map[string]int
	lastInput     string
	lastWithInput bool
	runCalls      []string
	runWithInputs []string
	listOutput    string
	nftDeleteErr  error
	nftListErr    error
}

func newStub() *stubRunner {
	return &stubRunner{listOutput: ""}
}

func (s *stubRunner) Run(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	s.runCalls = append(s.runCalls, key)
	switch {
	case strings.HasPrefix(key, "nft delete table"):
		if s.nftDeleteErr != nil {
			return "", s.nftDeleteErr
		}
		return "", nil
	case strings.HasPrefix(key, "nft list table"):
		if s.nftListErr != nil {
			return "", s.nftListErr
		}
		return s.listOutput, nil
	}
	return "", nil
}

func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	s.lastInput = input
	s.lastWithInput = true
	s.runWithInputs = append(s.runWithInputs, name+" "+strings.Join(args, " "))
	return "", nil
}

var _ execx.Runner = (*stubRunner)(nil)

// TestApplyUsesRunWithInput -- 下发必须经 RunWithInput（nft -f - 走 stdin）。
func TestApplyUsesRunWithInput(t *testing.T) {
	s := newStub()
	ps := []Policy{validPolicy()}
	// 回读文本直接用渲染结果，保证规则数与策略一致（countDropRules 匹配）。
	s.listOutput = RenderNft(ps)
	ok, msg := Apply(s, ps)
	if !ok {
		t.Fatalf("apply 失败: %s", msg)
	}
	if !s.lastWithInput {
		t.Fatal("nft -f - 必须用 RunWithInput 走 stdin")
	}
	found := false
	for _, c := range s.runWithInputs {
		if strings.Contains(c, "nft -f -") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应调用 nft -f -, got %v", s.runWithInputs)
	}
	if !strings.Contains(s.lastInput, "table inet router_quota") {
		t.Fatalf("stdin 应含表声明, got input:\n%s", s.lastInput)
	}
	// 且应先 delete 再下发
	hasDelete := false
	for _, c := range s.runCalls {
		if strings.HasPrefix(c, "nft delete table inet router_quota") {
			hasDelete = true
		}
	}
	if !hasDelete {
		t.Fatalf("apply 应先 delete 旧表（幂等）, got %v", s.runCalls)
	}
}

func renderWithOneDrop(ps []Policy) string {
	// 构造一个带注释和 1 条 drop 的 list 输出。
	out := strings.Builder{}
	out.WriteString("table inet router_quota {\n")
	out.WriteString("\tchain forward {\n")
	out.WriteString("\t\ttype filter hook forward priority 20;\n")
	out.WriteString("\t\tct state established,related accept\n")
	for _, p := range ps {
		if p.Enabled && p.RateMbit > 0 {
			out.WriteString("\t\tip saddr " + p.Target + " limit rate over 1/second burst 5 packets drop\n")
		}
	}
	out.WriteString("\t}\n}\n")
	return out.String()
}

// TestApplyValidatesAll -- 有非法策略应立即失败，不触发 nft。
func TestApplyValidatesAll(t *testing.T) {
	s := newStub()
	bad := validPolicy()
	bad.TargetType = TargetCIDR
	bad.Target = "0.0.0.0/0" // 红线
	ok, msg := Apply(s, []Policy{bad})
	if ok {
		t.Fatal("/0 策略 apply 应失败")
	}
	if !strings.Contains(msg, "校验失败") {
		t.Fatalf("错误应说明校验失败, got %q", msg)
	}
	if len(s.runCalls) != 0 && len(s.runWithInputs) != 0 {
		t.Fatalf("校验失败不应触发 nft 调用, runCalls=%v withInputs=%v", s.runCalls, s.runWithInputs)
	}
}

// TestApplyReadbackMismatch -- 回读规则数不匹配应报错。
func TestApplyReadbackMismatch(t *testing.T) {
	s := newStub()
	ps := []Policy{validPolicy()} // 期望 1 条 drop
	// 回读给 0 条 drop（不匹配）
	s.listOutput = "table inet router_quota {\n}\n"
	ok, msg := Apply(s, ps)
	if ok {
		t.Fatal("回读规则数不匹配应失败")
	}
	if !strings.Contains(msg, "不匹配") {
		t.Fatalf("错误应说明规则数不匹配, got %q", msg)
	}
}

func TestCleanupIgnoresMissingTable(t *testing.T) {
	s := newStub()
	s.nftDeleteErr = errTableMissing()
	if err := Cleanup(s); err != nil {
		t.Fatalf("表不存在时 Cleanup 不应报错, got %v", err)
	}
}

func errTableMissing() error {
	return &fakeErr{}
}

type fakeErr struct{}

func (*fakeErr) Error() string { return "No such file or directory" }

// TestCleanupDeletesTable -- Cleanup 应 delete 表。
func TestCleanupDeletesTable(t *testing.T) {
	s := newStub()
	if err := Cleanup(s); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	found := false
	for _, c := range s.runCalls {
		if strings.HasPrefix(c, "nft delete table inet router_quota") {
			found = true
		}
	}
	if !found {
		t.Fatalf("cleanup 应 delete 表, got %v", s.runCalls)
	}
}
