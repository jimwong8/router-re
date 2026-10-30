package ovpn

import (
	"strings"
	"testing"
)

const sampleClientList = `TITLE,OpenVPN 2.4.4
TIME,Thu May 23 10:00:00 2024,1716450000
HEADER,CLIENT_LIST,Common Name,Real Address,Virtual Address,Bytes Received,Bytes Sent,Connected Since
CLIENT_LIST,alice,1.2.3.4:1234,10.8.0.2,123,456,Thu May 23 09:00:00 2024
CLIENT_LIST,bob,5.6.7.8:5678,10.8.0.3,789,12,Thu May 23 09:30:00 2024
HEADER,ROUTING_TABLE,Virtual Address,Common Name,Real Address,Last Ref
ROUTING_TABLE,10.8.0.2,alice,1.2.3.4:1234,Thu May 23 09:59:00 2024
ROUTING_TABLE,10.8.0.3,bob,5.6.7.8:5678,Thu May 23 09:59:30 2024
GLOBAL_STATS,Max bcast/mcast queue length,0
END
`

const sampleClientListNoHeader = `CLIENT_LIST,carol,9.9.9.9:9999,200,300,Thu May 23 09:45:00 2024
HEADER,ROUTING_TABLE,Virtual Address,Common Name,Real Address,Last Ref
ROUTING_TABLE,10.8.0.4,carol,9.9.9.9:9999,Thu May 23 09:59:45 2024
`

const sampleStatus = `>STATUS
TIME,Thu May 23 10:00:00 2024,1716450000
UP SINCE,Thu May 23 08:00:00 2024
TOTAL,100000,200000
END
`

func TestParseClientListBasic(t *testing.T) {
	st := ParseClientList(sampleClientList)
	if len(st.Clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(st.Clients))
	}
	c0 := st.Clients[0]
	if c0.CommonName != "alice" {
		t.Errorf("cn0=%q", c0.CommonName)
	}
	if c0.RealAddress != "1.2.3.4:1234" {
		t.Errorf("ra0=%q", c0.RealAddress)
	}
	if c0.VirtualAddress != "10.8.0.2" {
		t.Errorf("va0=%q", c0.VirtualAddress)
	}
	if c0.BytesReceived != 123 {
		t.Errorf("br0=%d", c0.BytesReceived)
	}
	if c0.BytesSent != 456 {
		t.Errorf("bs0=%d", c0.BytesSent)
	}
	if c0.ConnectedSince != "Thu May 23 09:00:00 2024" {
		t.Errorf("cs0=%q", c0.ConnectedSince)
	}
	c1 := st.Clients[1]
	if c1.CommonName != "bob" || c1.VirtualAddress != "10.8.0.3" || c1.BytesReceived != 789 || c1.BytesSent != 12 {
		t.Errorf("client1 mismatch: %+v", c1)
	}
	if st.UpSince != "Thu May 23 10:00:00 2024" {
		t.Errorf("upsince=%q", st.UpSince)
	}
}

func TestParseClientListNoHeader(t *testing.T) {
	st := ParseClientList(sampleClientListNoHeader)
	if len(st.Clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(st.Clients))
	}
	c := st.Clients[0]
	if c.CommonName != "carol" {
		t.Errorf("cn=%q", c.CommonName)
	}
	if c.VirtualAddress != "10.8.0.4" {
		t.Errorf("va=%q (routing merge)", c.VirtualAddress)
	}
	if c.BytesReceived != 200 || c.BytesSent != 300 {
		t.Errorf("bytes mismatch: %+v", c)
	}
}

func TestParseClientListPrefixed(t *testing.T) {
	text := ">CLIENT_LIST,alice,1.2.3.4:1234,10.8.0.2,123,456,Thu May 23 09:00:00 2024\n"
	st := ParseClientList(text)
	// no header -> default 6 fields; first field treated as data? Actually line starts with CLIENT_LIST keyword after strip '>'.
	// Our parser expects first field CLIENT_LIST. Here first data field is alice? Wait prefixed line ">CLIENT_LIST,alice,..."
	// After strip '>' -> "CLIENT_LIST,alice,...": first=CLIENT_LIST, data=[alice,...]. Good.
	if len(st.Clients) != 1 {
		t.Fatalf("expected 1, got %d", len(st.Clients))
	}
	if st.Clients[0].CommonName != "alice" {
		t.Errorf("cn=%q", st.Clients[0].CommonName)
	}
}

func TestParseStatusBasic(t *testing.T) {
	st := ParseStatus(sampleStatus)
	if st.TotalBytesIn != 100000 {
		t.Errorf("in=%d", st.TotalBytesIn)
	}
	if st.TotalBytesOut != 200000 {
		t.Errorf("out=%d", st.TotalBytesOut)
	}
	if st.UpSince != "Thu May 23 08:00:00 2024" {
		t.Errorf("upsince=%q", st.UpSince)
	}
}

func TestParseStatusVariants(t *testing.T) {
	cases := []struct {
		text string
		in   int64
		out  int64
	}{
		{"TOTAL IN,5\nTOTAL OUT,7\n", 5, 7},
		{"TOTAL,10,20\n", 10, 20},
		{">STATUS\nTOTAL RX 3 TX 4\n", 3, 4},
	}
	for _, c := range cases {
		st := ParseStatus(c.text)
		if st.TotalBytesIn != c.in || st.TotalBytesOut != c.out {
			t.Errorf("text=%q: got in=%d out=%d want in=%d out=%d", c.text, st.TotalBytesIn, st.TotalBytesOut, c.in, c.out)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	st := ParseClientList("")
	if len(st.Clients) != 0 {
		t.Errorf("expected empty clients")
	}
	st2 := ParseStatus("")
	if st2.TotalBytesIn != 0 || st2.TotalBytesOut != 0 {
		t.Errorf("expected zero totals")
	}
}

func TestParseTruncated(t *testing.T) {
	// truncated client line should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	st := ParseClientList("CLIENT_LIST,alice,1.2.3.4:1234\n")
	if len(st.Clients) != 1 {
		t.Fatalf("expected 1, got %d", len(st.Clients))
	}
	// truncated status
	st2 := ParseStatus("TOTAL,1000\n")
	if st2.TotalBytesIn != 1000 {
		t.Errorf("in=%d", st2.TotalBytesIn)
	}
}

func TestParseBadInput(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	_ = ParseClientList("garbage line\n,,,\nCLIENT_LIST,\n")
	_ = ParseStatus("not a status\nTOTAL,abc,def\n")
}

var _ = strings.TrimSpace

// TestParseStatusFileFormat 用 openvpn --status 写出的真实文件内容。
//
// 这条用例的存在意义：此前的用例全是 management 接口格式（每行带 >CLIENT_LIST 前缀），
// 解析器在真实 status.log 上返回 0 个客户端而单测全绿。样本逐字取自
// 10.100.1.18 上实际运行的一份 status.log，不要"美化"它。
func TestParseStatusFileFormat(t *testing.T) {
	raw := `OpenVPN CLIENT LIST
Updated,2026-10-30 23:05:23
Common Name,Real Address,Bytes Received,Bytes Sent,Connected Since
alice,172.30.99.2:54089,5059,7748,2026-10-30 23:04:39
ROUTING TABLE
Virtual Address,Common Name,Real Address,Last Ref
10.8.0.2,alice,172.30.99.2:54089,2026-10-30 23:05:11
GLOBAL STATS
Max bcast/mcast queue length,1
END
`
	st := ParseClientList(raw)
	if len(st.Clients) != 1 {
		t.Fatalf("客户端数 = %d, 期望 1", len(st.Clients))
	}
	c := st.Clients[0]
	if c.CommonName != "alice" {
		t.Errorf("CN = %q", c.CommonName)
	}
	if c.VirtualAddress != "10.8.0.2" {
		t.Errorf("虚拟地址 = %q, 期望 10.8.0.2（来自 ROUTING TABLE 段）", c.VirtualAddress)
	}
	if c.BytesReceived != 5059 || c.BytesSent != 7748 {
		t.Errorf("流量 = %d/%d, 期望 5059/7748", c.BytesReceived, c.BytesSent)
	}
	if c.RealAddress != "172.30.99.2:54089" {
		t.Errorf("真实地址 = %q", c.RealAddress)
	}
}
