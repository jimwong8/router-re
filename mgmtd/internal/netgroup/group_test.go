// Package netgroup 单元测试
package netgroup

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateGroup(t *testing.T) {
	tests := []struct {
		name    string
		group   Group
		wantErr bool
	}{
		// 名称非空测试
		{
			name: "空名称应返回错误",
			group: Group{
				Name:  "",
				Kind:  KindIP,
				Items: []string{"192.168.1.1"},
			},
			wantErr: true,
		},
		// 名称过长测试 (65 chars > MaxNameLen=64)
		{
			name: "过长名称应返回错误",
			group: Group{
				Name:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 65 chars
				Kind:  KindIP,
				Items: []string{"192.168.1.1"},
			},
			wantErr: true,
		},
		// Kind 非法测试
		{
			name: "非法 Kind 应返回错误",
			group: Group{
				Name:  "test",
				Kind:  "invalid",
				Items: []string{"192.168.1.1"},
			},
			wantErr: true,
		},
		// IP 条目非法测试
		{
			name: "非法 IP 条目应返回错误",
			group: Group{
				Name:  "test",
				Kind:  KindIP,
				Items: []string{"not-an-ip"},
			},
			wantErr: true,
		},
		// MAC 条目非法测试
		{
			name: "非法 MAC 条目应返回错误",
			group: Group{
				Name:  "test",
				Kind:  KindMAC,
				Items: []string{"not-a-mac"},
			},
			wantErr: true,
		},
		// 条目数量超限测试
		{
			name: "过多数量应返回错误",
			group: Group{
				Name: "test",
				Kind: KindIP,
				Items: func() []string {
					a := make([]string, 513)
					for i := range a {
						a[i] = fmt.Sprintf("192.168.%d.1", i+1)
					}
					return a
				}(),
			},
			wantErr: true,
		},
		// 合法测试
		{
			name: "合法 IP 分组应通过",
			group: Group{
				Name:  "lan",
				Kind:  KindIP,
				Items: []string{"192.168.1.1", "10.0.0.0/8"},
			},
			wantErr: false,
		},
		// 合法 MAC 分组测试
		{
			name: "合法 MAC 分组应通过",
			group: Group{
				Name:  " wifi",
				Kind:  KindMAC,
				Items: []string{"AA:BB:CC:DD:EE:FF", "00:11:22:33:44:55"},
			},
			wantErr: false,
		},
		// 合法终端分组测试
		{
			name: "合法终端分组应通过",
			group: Group{
				Name:  "servers",
				Kind:  KindTerminal,
				Items: []string{"192.168.1.1", "AA:BB:CC:DD:EE:FF", "server1"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateGroup(tt.group); (err != nil) != tt.wantErr {
				t.Errorf("ValidateGroup() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateItem(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		item    string
		wantErr bool
	}{
		// IP 条目测试
		{
			name:    "合法单 IP",
			kind:    KindIP,
			item:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "合法 CIDR",
			kind:    KindIP,
			item:    "10.0.0.0/8",
			wantErr: false,
		},
		{
			name:    "非法 IP 字符串",
			kind:    KindIP,
			item:    "not-an-ip",
			wantErr: true,
		},
		// MAC 条目测试
		{
			name:    "合法 MAC",
			kind:    KindMAC,
			item:    "AA:BB:CC:DD:EE:FF",
			wantErr: false,
		},
		{
			name:    "合法 MAC (小写)",
			kind:    KindMAC,
			item:    "aa:bb:cc:dd:ee:ff",
			wantErr: false,
		},
		{
			name:    "非法 MAC 字符串",
			kind:    KindMAC,
			item:    "not-a-mac",
			wantErr: true,
		},
		// 终端条目测试
		{
			name:    "合法 MAC (终端)",
			kind:    KindTerminal,
			item:    "AA:BB:CC:DD:EE:FF",
			wantErr: false,
		},
		{
			name:    "合法 IP (终端)",
			kind:    KindTerminal,
			item:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "合法 主机名 (终端)",
			kind:    KindTerminal,
			item:    "server1",
			wantErr: false,
		},
		{
			name:    "非法 终端 条目",
			kind:    KindTerminal,
			item:    "invalid@name",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateItem(tt.kind, tt.item); (err != nil) != tt.wantErr {
				t.Errorf("validateItem() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeItems(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		items []string
		want  []string
	}{
		{
			name:  "MAC 统一小写并去重",
			kind:  KindMAC,
			items: []string{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
			want:  []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"},
		},
		{
			name:  "IP 条目保持原样",
			kind:  KindIP,
			items: []string{"192.168.1.1", "10.0.0.0/8"},
			want:  []string{"192.168.1.1", "10.0.0.0/8"},
		},
		{
			name:  "丢弃空条目",
			kind:  KindIP,
			items: []string{"  ", "192.168.1.1", "", " 10.0.0.1 "},
			want:  []string{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:  "保持首次出现顺序",
			kind:  KindIP,
			items: []string{"1.1.1.1", "2.2.2.2", "1.1.1.1"},
			want:  []string{"1.1.1.1", "2.2.2.2"},
		},
		{
			name:  "终端条目混合去重",
			kind:  KindTerminal,
			items: []string{"192.168.1.1", "AA:BB:CC:DD:EE:FF", "192.168.1.1", "server1"},
			want:  []string{"192.168.1.1", "AA:BB:CC:DD:EE:FF", "server1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeItems(tt.kind, tt.items)
			if len(got) != len(tt.want) {
				t.Errorf("NormalizeItems() len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("NormalizeItems()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		group     Group
		value     string
		wantMatch bool
	}{
		{
			name: "IP 组：单 IP 相等",
			kind: KindIP,
			group: Group{
				Name:  "lan",
				Kind:  KindIP,
				Items: []string{"192.168.1.1"},
			},
			value:     "192.168.1.1",
			wantMatch: true,
		},
		{
			name: "IP 组：CIDR 包含",
			kind: KindIP,
			group: Group{
				Name:  "private",
				Kind:  KindIP,
				Items: []string{"10.0.0.0/8"},
			},
			value:     "10.255.255.255",
			wantMatch: true,
		},
		{
			name: "IP 组：不匹配",
			kind: KindIP,
			group: Group{
				Name:  "lan",
				Kind:  KindIP,
				Items: []string{"192.168.1.1"},
			},
			value:     "10.0.0.1",
			wantMatch: false,
		},
		{
			name: "MAC 组：字符串相等（大小写）",
			kind: KindMAC,
			group: Group{
				Name:  "wifi",
				Kind:  KindMAC,
				Items: []string{"aa:bb:cc:dd:ee:ff"},
			},
			value:     "AA:BB:CC:DD:EE:FF",
			wantMatch: true,
		},
		{
			name: "MAC 组：不匹配",
			kind: KindMAC,
			group: Group{
				Name:  "wifi",
				Kind:  KindMAC,
				Items: []string{"aa:bb:cc:dd:ee:ff"},
			},
			value:     "11:22:33:44:55:66",
			wantMatch: false,
		},
		{
			name: "终端组：MAC 命中",
			kind: KindTerminal,
			group: Group{
				Name:  "servers",
				Kind:  KindTerminal,
				Items: []string{"AA:BB:CC:DD:EE:FF"},
			},
			value:     "aa:bb:cc:dd:ee:ff",
			wantMatch: true,
		},
		{
			name: "终端组：IP 命中",
			kind: KindTerminal,
			group: Group{
				Name:  "servers",
				Kind:  KindTerminal,
				Items: []string{"192.168.1.1"},
			},
			value:     "192.168.1.1",
			wantMatch: true,
		},
		{
			name: "终端组：主机名命中",
			kind: KindTerminal,
			group: Group{
				Name:  "servers",
				Kind:  KindTerminal,
				Items: []string{"server1"},
			},
			value:     "Server1",
			wantMatch: true,
		},
		{
			name: "终端组：不匹配",
			kind: KindTerminal,
			group: Group{
				Name:  "servers",
				Kind:  KindTerminal,
				Items: []string{"192.168.1.1"},
			},
			value:     "10.0.0.1",
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Match(tt.group, tt.value)
			if got != tt.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestSetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "普通名称",
			input: "my-set",
			want:  "my_set",
		},
		{
			name:  "含特殊字符",
			input: "my set!",
			want:  "my_set_",
		},
		{
			name:  "数字开头",
			input: "123set",
			want:  "s_123set",
		},
		{
			name:  "全特殊字符",
			input: "!@#$",
			want:  "____",
		},
		{
			name:  "空字符串",
			input: "",
			want:  "rr_set",
		},
		{
			name:  "混合内容",
			input: "LAN 123 !@#",
			want:  "LAN_123____",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetName(tt.input)
			if got != tt.want {
				t.Errorf("SetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderNftSet(t *testing.T) {
	tests := []struct {
		name    string
		group   Group
		wantSet string
		wantErr bool
	}{
		{
			name: "IP 分组渲染",
			group: Group{
				Name:  "lan",
				Kind:  KindIP,
				Items: []string{"192.168.1.1", "10.0.0.0/8"},
			},
			wantSet: "set ip_lan { type ipv4_addr; flags interval; elements = { 192.168.1.1, 10.0.0.0/8 } }",
			wantErr: false,
		},
		{
			name: "MAC 分组渲染",
			group: Group{
				Name:  "wifi",
				Kind:  KindMAC,
				Items: []string{"aa:bb:cc:dd:ee:ff"},
			},
			wantSet: "set mac_wifi { type ether_addr; elements = { aa:bb:cc:dd:ee:ff } }",
			wantErr: false,
		},
		{
			name: "终端分组应返回错误",
			group: Group{
				Name:  "terminals",
				Kind:  KindTerminal,
				Items: []string{"192.168.1.1"},
			},
			wantErr: true,
		},
		{
			name: "非法类型应返回错误",
			group: Group{
				Name:  "invalid",
				Kind:  "unknown",
				Items: []string{"192.168.1.1"},
			},
			wantErr: true,
		},
		{
			name: "IPv6 条目应返回错误",
			group: Group{
				Name:  "ipv6-set",
				Kind:  KindIP,
				Items: []string{"::1"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderNftSet(tt.group)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderNftSet() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantSet {
				t.Errorf("RenderNftSet() = %q, want %q", got, tt.wantSet)
			}
		})
	}
}

func TestIsIPv6Item(t *testing.T) {
	tests := []struct {
		item string
		want bool
	}{
		{
			item: "::1",
			want: true,
		},
		{
			item: "2001:db8::1",
			want: true,
		},
		{
			item: "192.168.1.1",
			want: false,
		},
		{
			item: "fd00::",
			want: true,
		},
		{
			item: "fe80::1",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.item, func(t *testing.T) {
			got := isIPv6Item(tt.item)
			if got != tt.want {
				t.Errorf("isIPv6Item(%q) = %v, want %v", tt.item, got, tt.want)
			}
		})
	}
}

func TestGroupIDChineseNamesDistinct(t *testing.T) {
	// 回归：SetName 会把中文逐个替换成下划线，"办公网""财务网"都变成 ip____，
	// 三个分组抢同一个 ID，引用方就会拿错成员。
	a := GroupID(KindIP, "办公网")
	b := GroupID(KindIP, "财务网")
	c := GroupID(KindIP, "仓库网")
	if a == b || b == c || a == c {
		t.Fatalf("不同中文名必须生成不同 ID: %s / %s / %s", a, b, c)
	}
	for _, id := range []string{a, b, c} {
		if !strings.HasPrefix(id, KindIP+"_") {
			t.Fatalf("ID 应带类型前缀: %s", id)
		}
		if strings.Contains(id, "____") {
			t.Fatalf("ID 不该是一串下划线: %s", id)
		}
	}
}

func TestGroupIDStable(t *testing.T) {
	// 同一 (kind,name) 多次调用结果必须一致，否则每次保存引用都会失效
	if GroupID(KindIP, "办公网") != GroupID(KindIP, "办公网") {
		t.Fatal("ID 必须稳定")
	}
	// 不同类型下同名分组可以共用一个 tail，但前缀不同
	if GroupID(KindIP, "office") == GroupID(KindMAC, "office") {
		t.Fatal("不同类型应生成不同 ID")
	}
}

func TestGroupIDKeepsReadableASCII(t *testing.T) {
	if got := GroupID(KindIP, "office"); got != "ip_office" {
		t.Fatalf("纯 ASCII 名应保持可读，实际 %q", got)
	}
	if got := GroupID(KindMAC, "printer1"); got != "mac_printer1" {
		t.Fatalf("纯 ASCII 名应保持可读，实际 %q", got)
	}
	// 数字开头要加前缀，避免 nft 集合名非法
	if got := GroupID(KindIP, "192lan"); !strings.HasPrefix(got, "ip_") {
		t.Fatalf("数字开头应加前缀: %q", got)
	}
}

func TestGroupIDSpecialCharsUseHash(t *testing.T) {
	// 空格和短横线会被替换成下划线，信息丢失 → 必须走哈希，
	// 否则 "server farm" 和 "server-farm" 会共用 server_farm
	x := GroupID(KindTerminal, "server farm")
	y := GroupID(KindTerminal, "server-farm")
	if x == y {
		t.Fatalf("含空格的名称应走哈希，避免与短横线版本撞车: %s", x)
	}
}

func TestRenderNftSetNamesUnique(t *testing.T) {
	// 回归："办公网""财务网"用 SetName 都变成 ___，两条 nft 集合定义撞名。
	a := Group{Kind: KindIP, Name: "办公网", Items: []string{"192.168.10.0/24"}}
	b := Group{Kind: KindIP, Name: "财务网", Items: []string{"192.168.20.0/24"}}
	a.ID = GroupID(a.Kind, a.Name)
	b.ID = GroupID(b.Kind, b.Name)
	sa, err := RenderNftSet(a)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := RenderNftSet(b)
	if err != nil {
		t.Fatal(err)
	}
	if sa == sb {
		t.Fatalf("两个不同分组渲染出了相同的集合定义:\n%s", sa)
	}
	if !strings.Contains(sa, a.ID) {
		t.Fatalf("集合名应使用分组 ID: %s", sa)
	}
	if strings.Contains(sa, "set ___") {
		t.Fatalf("集合名不应是一串下划线: %s", sa)
	}
}

func TestRenderNftSetWithoutID(t *testing.T) {
	// 没有 ID 时也要能从名字推出稳定集合名
	g := Group{Kind: KindMAC, Name: "打印机", Items: []string{"aa:bb:cc:dd:ee:ff"}}
	out, err := RenderNftSet(g)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "set ___") {
		t.Fatalf("无 ID 时也应生成可区分的集合名: %s", out)
	}
	if !strings.Contains(out, "ether_addr") {
		t.Fatalf("MAC 分组类型错误: %s", out)
	}
}
