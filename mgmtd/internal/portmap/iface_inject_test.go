package portmap

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// runner 记录所有被执行的命令，用于断言"到底跑出了什么 argv"。
type runner struct{ cmds [][]string }

func (r *runner) Run(name string, args ...string) (string, error) {
	r.cmds = append(r.cmds, append([]string{name}, args...))
	return "", nil
}

func (r *runner) RunWithInput(input, name string, args ...string) (string, error) {
	return r.Run(name, args...)
}

// RenderCommands 把 token 拼成字符串，ApplyPolicy 再用 strings.Fields 切回
// argv —— 接口名带空格就等于追加任意 ip route 参数。
func TestApplyPolicyRejectsInjectedIface(t *testing.T) {
	payloads := []string{
		"eth0 mtu 9000",
		"eth0 table 253",
		"eth0\n---",
		"eth0;reboot",
		"eth0 && reboot",
		"$(reboot)",
		"",
	}
	ps := []Policy{{ID: 1, Table: 100, Priority: 1000, Enabled: true}}
	for _, p := range payloads {
		r := &runner{}
		err := ApplyPolicy(r, ps, map[int]string{100: p})
		if err == nil {
			t.Errorf("接口名 %q 未被拒绝", p)
		}
		if len(r.cmds) != 0 {
			t.Errorf("接口名 %q 被拒后仍执行了命令: %v", p, r.cmds)
		}
	}
}

func TestValidateTableIfacesAcceptsLegit(t *testing.T) {
	ok := map[int]string{
		100: "eth0",
		101: "br-lan",
		102: "bond0",
		103: "pppoe-wan",
		104: "eth0.100",
		105: "wg0",
		106: "eth0:1",
	}
	if err := ValidateTableIfaces(ok); err != nil {
		t.Fatalf("合法接口名被误拒: %v", err)
	}
}

// 合法路径必须照常生成两条命令，且 dev 后面就是我们给的接口名。
func TestApplyPolicyLegitStillWorks(t *testing.T) {
	r := &runner{}
	ps := []Policy{{ID: 1, Table: 100, Priority: 1000, Enabled: true}}
	if err := ApplyPolicy(r, ps, map[int]string{100: "pppoe-wan"}); err != nil {
		t.Fatalf("合法调用失败: %v", err)
	}
	var joined []string
	for _, c := range r.cmds {
		joined = append(joined, strings.Join(c, " "))
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "ip rule add lookup 100 priority 1000") {
		t.Errorf("缺少 ip rule 命令:\n%s", all)
	}
	if !strings.Contains(all, "ip route replace default dev pppoe-wan table 100") {
		t.Errorf("缺少 ip route 命令或接口名不正确:\n%s", all)
	}
	// 必须正好是 ip route 的参数序列，不能多出任何 argv
	for _, c := range r.cmds {
		if len(c) > 1 && c[1] == "route" {
			want := []string{"ip", "route", "replace", "default", "dev", "pppoe-wan", "table", "100"}
			if strings.Join(c, " ") != strings.Join(want, " ") {
				t.Errorf("argv 不符\nwant: %v\ngot:  %v", want, c)
			}
		}
	}
}

// 空映射是合法输入（不生成 ip route 行），不能因为校验而失败。
func TestApplyPolicyEmptyIfaces(t *testing.T) {
	r := &runner{}
	ps := []Policy{{ID: 1, Table: 100, Priority: 1000, Enabled: true}}
	if err := ApplyPolicy(r, ps, map[int]string{}); err != nil {
		t.Fatalf("空 table_ifaces 不应失败: %v", err)
	}
	if len(r.cmds) != 1 {
		t.Fatalf("应只有 1 条 ip rule 命令，实际 %d: %v", len(r.cmds), r.cmds)
	}
}

var _ execx.Runner = (*runner)(nil)
