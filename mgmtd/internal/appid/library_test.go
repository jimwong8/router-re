package appid

import (
	"strings"
	"testing"
	"time"
)

func libWith(t *testing.T) *Library {
	t.Helper()
	l := New()
	l.ImportDomainList(map[string][]byte{
		"qq":       []byte("# 腾讯 QQ\nqq.com\nqq.com.cn\nfull:exact.qq.com\n"),
		"wechat":   []byte("weixin.qq.com\nwechat.com\ninclude:qq-cdn\n"),
		"qq-cdn":   []byte("qq-cdn.example.net\n"),
		"bilibili": []byte("bilibili.com\nb23.tv\n"),
	}, map[string]App{
		"qq":       {ID: "qq", Name: "QQ", Category: CatIM},
		"wechat":   {ID: "wechat", Name: "微信", Category: CatIM},
		"qq-cdn":   {ID: "qq-cdn", Name: "QQ CDN"},
		"bilibili": {ID: "bilibili", Name: "哔哩哔哩", Category: CatVideo},
	})
	return l
}

func TestLookupSuffixAndExact(t *testing.T) {
	l := libWith(t)

	cases := []struct{ host, want string }{
		{"qq.com", "qq"},
		{"www.qq.com", "qq"},
		{"a.b.qq.com", "qq"},
		{"QQ.COM", "qq"},
		{"www.qq.com.", "qq"},
		{"www.qq.com:443", "qq"},
		{"exact.qq.com", "qq"},
		{"b23.tv", "bilibili"},
		{"www.bilibili.com", "bilibili"},
	}
	for _, c := range cases {
		a, ok := l.Lookup(c.host)
		if !ok || a.ID != c.want {
			t.Fatalf("%s 应识别为 %s，实际 %+v ok=%v", c.host, c.want, a, ok)
		}
	}

	// 不相关的域名不能误判
	for _, h := range []string{"example.org", "notqq.com", "qq.com.evil.net"} {
		if a, ok := l.Lookup(h); ok {
			t.Fatalf("%s 不该命中，实际 %+v", h, a)
		}
	}
}

func TestLookupFullIsExactOnly(t *testing.T) {
	l := New()
	l.ImportDomainList(map[string][]byte{
		"app": []byte("full:only.example.com\nsub.example.com\n"),
	}, map[string]App{"app": {ID: "app", Name: "App"}})

	if _, ok := l.Lookup("only.example.com"); !ok {
		t.Fatal("full: 应匹配自身")
	}
	// full: 的关键语义：子域不匹配
	if _, ok := l.Lookup("x.only.example.com"); ok {
		t.Fatal("full: 不该匹配子域")
	}
	// 普通条目反过来要匹配子域
	if _, ok := l.Lookup("a.sub.example.com"); !ok {
		t.Fatal("普通条目应匹配子域")
	}
}

func TestIncludeExpansion(t *testing.T) {
	l := libWith(t)
	// wechat include 了 qq-cdn，展开后 wechat 组里也应含 qq-cdn 的域名
	if a, ok := l.Lookup("x.qq-cdn.example.net"); !ok || a.ID == "" {
		t.Fatalf("include 未展开: %+v ok=%v", a, ok)
	}
}

func TestIncludeCycleDoesNotHang(t *testing.T) {
	done := make(chan bool, 1)
	go func() {
		l := New()
		_, err := l.ImportDomainList(map[string][]byte{
			"a": []byte("a.com\ninclude:b\n"),
			"b": []byte("b.com\ninclude:a\n"),
		}, map[string]App{"a": {ID: "a"}, "b": {ID: "b"}})
		if err != nil {
			t.Error(err)
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("循环 include 必须被截断，不能死循环")
	}
}

func TestMissingIncludeCounted(t *testing.T) {
	l := New()
	rej, err := l.ImportDomainList(map[string][]byte{
		"a": []byte("a.com\ninclude:doesnotexist\n"),
	}, map[string]App{"a": {ID: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if rej.Missing == 0 {
		t.Fatal("缺失的 include 必须被计数——静默丢弃最难排查")
	}
}

func TestRegexpAndKeywordRejected(t *testing.T) {
	l := New()
	rej, err := l.ImportDomainList(map[string][]byte{
		"a": []byte("a.com\nregexp:.*evil.*\nkeyword:tracker\n"),
	}, map[string]App{"a": {ID: "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if rej.Regexp != 1 || rej.Keyword != 1 {
		t.Fatalf("应各拒绝 1 条，实际 regexp=%d keyword=%d", rej.Regexp, rej.Keyword)
	}
	if _, ok := l.Lookup("a.com"); !ok {
		t.Fatal("正常条目应保留")
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"WWW.QQ.COM":     "www.qq.com",
		"www.qq.com.":    "www.qq.com",
		"www.qq.com:443": "www.qq.com",
		"  a.com  ":      "a.com",
		"":               "",
		"1.2.3.4":        "",
		"::1":            "",
		"[::1]:443":      "",
		"2001:db8::1":    "",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Fatalf("%q -> %q，期望 %q", in, got, want)
		}
	}
}

func TestIPv4LikeHostNotMatched(t *testing.T) {
	l := New()
	l.ImportDomainList(map[string][]byte{"a": []byte("1.2.3.4\n")}, nil)
	// 即便有人把 IP 写进域名库，也不该在 IP 连接上命中（那是另一套逻辑）
	if _, ok := l.Lookup("1.2.3.4"); ok {
		t.Fatal("IP 字面量不该参与域名匹配")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	l := libWith(t)
	l.Version = "20261030"
	b, err := l.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	l2, err := LoadSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := l2.Lookup("www.qq.com"); !ok || a.Name != "QQ" {
		t.Fatalf("快照恢复后匹配失败: %+v ok=%v", a, ok)
	}
	if l2.Version != "20261030" {
		t.Fatal("版本号应保留")
	}
	if l2.Stats().Apps != l.Stats().Apps {
		t.Fatal("应用数应一致")
	}
}

func TestStats(t *testing.T) {
	l := libWith(t)
	s := l.Stats()
	if s.Apps == 0 || s.Domains == 0 {
		t.Fatalf("统计不应为空: %+v", s)
	}
	if s.ByCategory[CatIM] < 2 {
		t.Fatalf("即时通讯应有 2 个应用，实际 %d", s.ByCategory[CatIM])
	}
	if len(s.TopApps) > 10 {
		t.Fatal("TopApps 最多 10 个")
	}
}

func TestEmptyLibraryExplainsItself(t *testing.T) {
	s := New().Stats()
	if s.EmptyReason == "" {
		t.Fatal("空库要给出可展示的原因，而不是让界面显示 0 个应用")
	}
}

func TestCategoryForGroup(t *testing.T) {
	cases := map[string]Category{
		"bilibili":           CatVideo,
		"douyin":             CatVideo,
		"tencent-games":      CatGame,
		"wechat":             CatIM,
		"weibo":              CatSocial,
		"taobao":             CatShopping,
		"alipay":             CatPayment,
		"github":             CatOffice,
		"category-ads-all":   CatAd,
		"category-ai-!cn":    CatAI,
		"some-unknown-thing": CatOther,
		"category-media":     CatVideo,
	}
	for in, want := range cases {
		if got := CategoryForGroup(in); got != want {
			t.Fatalf("%s -> %s，期望 %s", in, got, want)
		}
	}
}

func TestParseDomainListSkipsComments(t *testing.T) {
	g, rej, err := ParseDomainList("x", strings.NewReader("# 注释\n// 另一种注释\n\nqq.com # 行尾注释\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.domains) != 1 || g.domains[0] != "qq.com" {
		t.Fatalf("应只解析出 qq.com，实际 %+v (rejected %+v)", g.domains, rej)
	}
}
