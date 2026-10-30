// Package appid 是应用识别特征库：把域名/SNI 映射到具体的应用与分类。
//
// 背景：底层的 DPI 只能到协议层——它告诉你这是 TLS，不告诉你这是微信还是
// 抖音。真正的"应用识别"靠一张 域名 -> 应用 的映射表，这正是商业路由
// （如 iKuai）所谓"3000+ 应用特征库"的核心资产。
//
// 这里不搬运任何厂商的私有特征库（那是其付费许可下的商业资产，且格式
// 加密不可解析），而是提供一个可热更新、可导入、可自己抓包补充的引擎，
// 数据来自公开的域名分类项目或自建。
//
// 匹配语义（沿用 v2ray domain-list 的约定，便于直接导入现成数据）：
//
//	example.com     -> 精确匹配 example.com，同时匹配其所有子域
//	full:example.com-> 只匹配 example.com 本身
//	keyword:abc     -> 子串匹配（谨慎使用，误判与开销都大）
//	include:other   -> 引用另一组，导入时展开
//	regexp:...      -> 显式拒绝：不可控的正则会在每包路径上变成 DoS 入口
package appid

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Category 是应用分类。参考通用的大类划分，便于在界面上做分组统计。
type Category string

const (
	CatVideo    Category = "视频"
	CatMusic    Category = "音乐"
	CatGame     Category = "游戏"
	CatSocial   Category = "社交"
	CatIM       Category = "即时通讯"
	CatShopping Category = "购物"
	CatPayment  Category = "支付"
	CatNews     Category = "新闻"
	CatOffice   Category = "办公"
	CatEdu      Category = "教育"
	CatFinance  Category = "金融"
	CatTravel   Category = "旅游"
	CatLife     Category = "生活"
	CatDownload Category = "下载"
	CatP2P      Category = "P2P"
	CatTool     Category = "工具"
	CatAI       Category = "AI"
	CatAd       Category = "广告"
	CatUpdate   Category = "系统更新"
	CatOther    Category = "其他"
)

// App 是一个被识别的应用。
type App struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category Category `json:"category"`
	Domains  int      `json:"domains,omitempty"`
}

// Entry 是一条特征（域名 -> 应用）。
type Entry struct {
	Domain string
	AppID  string
}

// Library 是不可变快照，可整体替换以实现热更新。
type Library struct {
	Version   string            `json:"version"`
	UpdatedAt time.Time         `json:"updated_at"`
	Source    string            `json:"source,omitempty"`
	Apps      map[string]App    `json:"apps"`
	Exact     map[string]string `json:"exact"`  // 完整域名 -> appID（full: 与普通域名的自身匹配）
	Suffix    map[string]string `json:"suffix"` // 后缀 -> appID
}

// New 创建一个空库。
func New() *Library {
	return &Library{
		Apps:   map[string]App{},
		Exact:  map[string]string{},
		Suffix: map[string]string{},
	}
}

// NormalizeHost 统一 host 形态：小写、去端口、去尾部点。
//
// 从 SNI 拿到的通常是裸域名，但 HTTP Host 可能带端口（example.com:8080），
// 两者都要能查。
func NormalizeHost(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	if h == "" {
		return ""
	}
	// 去掉端口：只在最后一个冒号后全是数字时才算端口，
	// 避免把 IPv6 字面量误切（IPv6 不走域名匹配，直接返回空）
	if i := strings.LastIndexByte(h, ':'); i > 0 {
		if isAllDigits(h[i+1:]) {
			h = h[:i]
		}
	}
	h = strings.TrimSuffix(h, ".")
	// IPv4 / IPv6 字面量不参与域名匹配
	if h == "" || (h[0] >= '0' && h[0] <= '9' && strings.Contains(h, ".") && isIPv4Like(h)) {
		return ""
	}
	if strings.Contains(h, ":") {
		return ""
	}
	return h
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isIPv4Like(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if !isAllDigits(p) || len(p) > 3 {
			return false
		}
	}
	return true
}

// Lookup 按 host 查应用。
//
// 顺序：full 精确表 -> 逐级后缀（www.qq.com → qq.com → com）。
// 逐级剥离天然满足"越具体越优先"，不需要额外排序。
func (l *Library) Lookup(host string) (App, bool) {
	h := NormalizeHost(host)
	if h == "" || l == nil {
		return App{}, false
	}
	if id, ok := l.Exact[h]; ok {
		if a, ok2 := l.Apps[id]; ok2 && !isAggregateGroup(a.ID) {
			return a, true
		}
	}
	for {
		if id, ok := l.Suffix[h]; ok {
			if a, ok2 := l.Apps[id]; ok2 && !isAggregateGroup(a.ID) {
				return a, true
			}
		}
		i := strings.IndexByte(h, '.')
		if i < 0 {
			break
		}
		h = h[i+1:]
		if h == "" {
			break
		}
	}
	return App{}, false
}

// Add 加入一条特征。app 不存在时会创建（分类为 CatOther）。
func (l *Library) Add(e Entry, app App) {
	e.Domain = NormalizeHost(e.Domain)
	if e.Domain == "" || e.AppID == "" {
		return
	}
	app.ID = e.AppID
	if app.Name == "" {
		app.Name = e.AppID
	}
	if app.Category == "" {
		app.Category = CatOther
	}
	if old, ok := l.Apps[app.ID]; ok {
		// 保留已有分类，只补名称
		if old.Category != CatOther && app.Category == CatOther {
			app.Category = old.Category
		}
		app.Domains = old.Domains
	}
	l.Apps[app.ID] = app
	l.Suffix[e.Domain] = e.AppID
	l.Exact[e.Domain] = e.AppID
}

// AddFull 只做精确匹配（对应 full: 前缀）。
func (l *Library) AddFull(domain, appID string, app App) {
	d := NormalizeHost(domain)
	if d == "" || appID == "" {
		return
	}
	app.ID = appID
	if app.Name == "" {
		app.Name = appID
	}
	if app.Category == "" {
		app.Category = CatOther
	}
	if old, ok := l.Apps[appID]; ok {
		if old.Category != CatOther && app.Category == CatOther {
			app.Category = old.Category
		}
		app.Domains = old.Domains
	}
	l.Apps[appID] = app
	l.Exact[d] = appID
}

// Recount 重新统计每个应用的域名数。导入完成后调用一次。
func (l *Library) Recount() {
	counts := map[string]int{}
	for _, id := range l.Exact {
		counts[id]++
	}
	for _, id := range l.Suffix {
		counts[id]++
	}
	for id, a := range l.Apps {
		a.Domains = counts[id]
		l.Apps[id] = a
	}
}

// Stats 是库的概览。
type Stats struct {
	Version     string           `json:"version"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Source      string           `json:"source,omitempty"`
	Apps        int              `json:"apps"`
	Domains     int              `json:"domains"`
	ByCategory  map[Category]int `json:"by_category"`
	TopApps     []App            `json:"top_apps"`
	Rejected    Rejected         `json:"rejected"`
	EmptyReason string           `json:"empty_reason,omitempty"`
}

// Rejected 记录导入时被跳过的条目。静默丢弃是最难排查的一类问题，
// 所以这些数字要能展示出来。
type Rejected struct {
	Regexp  int `json:"regexp"`
	Keyword int `json:"keyword"`
	Invalid int `json:"invalid"`
	Missing int `json:"missing_include"`
}

func (l *Library) Stats() Stats {
	s := Stats{
		Version: l.Version, UpdatedAt: l.UpdatedAt, Source: l.Source,
		Apps: len(l.Apps), Domains: len(l.Suffix) + len(l.Exact),
		ByCategory: map[Category]int{},
	}
	for _, a := range l.Apps {
		s.ByCategory[a.Category]++
	}
	top := make([]App, 0, len(l.Apps))
	for _, a := range l.Apps {
		top = append(top, a)
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Domains != top[j].Domains {
			return top[i].Domains > top[j].Domains
		}
		return top[i].ID < top[j].ID
	})
	if len(top) > 10 {
		top = top[:10]
	}
	s.TopApps = top
	if s.Apps == 0 {
		s.EmptyReason = "特征库为空：导入一份域名分类数据后才会开始识别应用"
	}
	return s
}

// ---------- 导入 ----------

// groupFile 是导入过程中的中间结构：一个文件（一组）的原始行。
type groupFile struct {
	name     string
	domains  []string
	fulls    []string
	includes []string
}

// ParseDomainList 解析 v2ray domain-list 文本。
//
// 返回值是"组名 -> 组内容"，include 由 Resolve 阶段展开（因为可能存在
// 前向引用：A include B，而 B 在读取顺序上位于 A 之后）。
func ParseDomainList(name string, r io.Reader) (groupFile, Rejected, error) {
	g := groupFile{name: name}
	var rej Rejected
	sc := bufio.NewScanner(r)
	// 单行可能很长（有项目一行放多个域名），放宽到 1MB
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		// 行尾注释
		if i := strings.Index(line, " #"); i > 0 {
			line = strings.TrimSpace(line[:i])
		}
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "regexp:"):
			// 不可控正则会变成每包路径上的 DoS 入口，明确拒绝
			rej.Regexp++
			continue
		case strings.HasPrefix(lower, "keyword:"):
			rej.Keyword++
			continue
		case strings.HasPrefix(lower, "include:"):
			// domain-list 允许 "include:bilibili @-!cn" —— @ 后面是属性
			// （区域/子集限定）。组名是第一个 token；把整串当组名会
			// 产生大量"引用了不存在的组"的假象。
			target := strings.TrimSpace(line[len("include:"):])
			if f := strings.Fields(target); len(f) > 0 {
				target = f[0]
			}
			if target != "" {
				g.includes = append(g.includes, target)
			}
			continue
		case strings.HasPrefix(lower, "full:"):
			d := NormalizeHost(line[len("full:"):])
			if d == "" {
				rej.Invalid++
				continue
			}
			g.fulls = append(g.fulls, d)
			continue
		default:
			// 跳过带 colon 的未知前缀（如 ext:）
			if i := strings.IndexByte(line, ':'); i > 0 && !strings.Contains(line[:i], ".") {
				rej.Invalid++
				continue
			}
			d := NormalizeHost(line)
			if d == "" {
				rej.Invalid++
				continue
			}
			g.domains = append(g.domains, d)
		}
	}
	if err := sc.Err(); err != nil {
		return g, rej, err
	}
	return g, rej, nil
}

// isAggregateGroup 判断一个组是"具体应用"还是"聚合索引"。
//
// domain-list 里混着两类东西：具体服务（bilibili、tencent）和聚合组
// （category-cn、geolocation-cn、tld-cn、cn —— 后者把整个 .cn 或某个
// 国家/分类下的所有域名拢在一起）。聚合组不能和具体应用抢域名：
// 否则 taobao.com 会被 cn 这类聚合组覆盖，识别结果就变成"cn"。
// IsAggregateGroup 对外暴露聚合组判定：category-*、tld-*、geolocation-*
// 以及 cn/private 这类索引组，它们不是"应用"，不应出现在应用列表里。
func IsAggregateGroup(name string) bool { return isAggregateGroup(name) }

func isAggregateGroup(name string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, "category-") || strings.HasPrefix(n, "tld-") || strings.HasPrefix(n, "geolocation-") {
		return true
	}
	switch n {
	case "cn", "private", "test", "gfw", "geoip", "ir", "ru", "jp", "hk", "mm", "!cn", "-!cn", "@!cn":
		return true
	}
	return false
}

// isReservedDomain 排除特殊用途域：它们不是"应用"，把它们塞进特征库
// 只会让内网/测试流量被识别成莫名其妙的分类。
func isReservedDomain(d string) bool {
	switch d {
	case "lan", "localdomain", "localhost", "test", "invalid", "example",
		"onion", "home", "corp", "internal":
		return true
	}
	return strings.HasSuffix(d, ".local") || strings.HasSuffix(d, ".localhost") ||
		strings.HasSuffix(d, ".test") || strings.HasSuffix(d, ".invalid") ||
		strings.HasSuffix(d, ".example") || strings.HasSuffix(d, ".onion")
}

// ImportDomainList 把多组 domain-list 数据装载进库。
//
// groups: 组名（通常就是应用名）-> 该组的文本内容
// meta:   组名 -> 应用名与分类（可选，缺省用组名当应用名）
//
// include 会被展开；无法解析的 include 计入 Rejected.Missing，不中断导入。
func (l *Library) ImportDomainList(groups map[string][]byte, meta map[string]App) (Rejected, error) {
	parsed := map[string]groupFile{}
	var total Rejected

	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		g, rej, err := ParseDomainList(n, strings.NewReader(string(groups[n])))
		if err != nil {
			return total, fmt.Errorf("解析 %s 失败: %w", n, err)
		}
		total.Regexp += rej.Regexp
		total.Keyword += rej.Keyword
		total.Invalid += rej.Invalid
		parsed[n] = g
	}

	// 展开 include。用 visited 防止 A→B→A 的环。
	resolved := map[string]bool{}
	var resolve func(name string, depth int) ([]string, []string)
	var missRej int
	resolve = func(name string, depth int) ([]string, []string) {
		if depth > 8 {
			return nil, nil
		}
		g, ok := parsed[name]
		if !ok {
			missRej++
			return nil, nil
		}
		if resolved[name] {
			return g.domains, g.fulls
		}
		resolved[name] = true
		domains := append([]string{}, g.domains...)
		fulls := append([]string{}, g.fulls...)
		for _, inc := range g.includes {
			d, f := resolve(inc, depth+1)
			domains = append(domains, d...)
			fulls = append(fulls, f...)
		}
		return domains, fulls
	}

	// 分类反推：category-video include 了 bilibili，说明 bilibili 属于视频。
	// 这比靠组名猜关键词准得多——1551 个组里大部分是服务名（zte、yuewen），
	// 靠关键词只能认出不到 10%。
	inferred := map[string]Category{}
	for _, n := range names {
		if !strings.HasPrefix(strings.ToLower(n), "category-") {
			continue
		}
		cat := CategoryForGroup(n)
		if cat == CatOther {
			continue
		}
		g := parsed[n]
		domains, fulls := resolve(n, 0)
		_ = domains
		_ = fulls
		for _, inc := range g.includes {
			old, exists := inferred[inc]
			// 广告分类优先级最低：tencent 既在 category-ads 也在
			// category-media-cn 里，整组标成"广告"没有意义
			if !exists || (old == CatAd && cat != CatAd) {
				inferred[inc] = cat
			}
		}
	}

	// 分两轮导入：先具体应用，再聚合组。
	// 聚合组只补"还没人认领"的域名，绝不覆盖具体应用的归属。
	var concrete, aggregate []string
	for _, n := range names {
		if isAggregateGroup(n) {
			aggregate = append(aggregate, n)
		} else {
			concrete = append(concrete, n)
		}
	}

	load := func(n string, allowOverwrite bool) {
		domains, fulls := resolve(n, 0)
		app := meta[n]
		if app.Name == "" && app.ID == "" {
			cat := inferred[n]
			if cat == "" {
				cat = CategoryForGroup(n)
			}
			app = App{ID: n, Name: n, Category: cat}
		}
		if app.ID == "" {
			app.ID = n
		}
		if app.Category == "" || app.Category == CatOther {
			if cat := inferred[n]; cat != "" {
				app.Category = cat
			}
		}
		// 人工校准优先于任何自动推断
		if ov, ok := appOverrides[n]; ok {
			app.Name = ov.Name
			app.Category = ov.Category
		}
		for _, d := range domains {
			if isReservedDomain(d) {
				continue
			}
			if !allowOverwrite {
				if _, exists := l.Suffix[d]; exists {
					continue
				}
				if _, exists := l.Exact[d]; exists {
					continue
				}
			}
			l.Add(Entry{Domain: d, AppID: app.ID}, app)
		}
		for _, d := range fulls {
			if !allowOverwrite {
				if _, exists := l.Exact[d]; exists {
					continue
				}
			}
			l.AddFull(d, app.ID, app)
		}
	}

	for _, n := range concrete {
		load(n, true)
	}
	for _, n := range aggregate {
		load(n, false)
	}
	total.Missing = missRej
	l.Recount()
	return total, nil
}

// ---------- 序列化 ----------

// MarshalJSON 之外的显式快照方法：库是 map 密集型结构，
// 直接 json.Marshal 也能用，但版本化快照更适合热更新（先写文件再原子替换）。
func (l *Library) Snapshot() ([]byte, error) {
	return json.Marshal(l)
}

// LoadSnapshot 从 JSON 快照恢复。
func LoadSnapshot(b []byte) (*Library, error) {
	var l Library
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if l.Apps == nil {
		l.Apps = map[string]App{}
	}
	if l.Exact == nil {
		l.Exact = map[string]string{}
	}
	if l.Suffix == nil {
		l.Suffix = map[string]string{}
	}
	return &l, nil
}

// appOverrides 是知名应用的人工校准表。
//
// 为什么需要它：domain-list 的组名是英文标识，而且很多大厂组里混着广告/统计
// 域名（tencent 组被 category-ads 引用过），自动推断会把它归到"广告"。
// 这里对高频应用直接给定名称与分类，优先级最高。
var appOverrides = map[string]App{
	"tencent":       {ID: "tencent", Name: "腾讯", Category: CatIM},
	"qq":            {ID: "qq", Name: "QQ", Category: CatIM},
	"wechat":        {ID: "wechat", Name: "微信", Category: CatIM},
	"weixin":        {ID: "weixin", Name: "微信", Category: CatIM},
	"alibaba":       {ID: "alibaba", Name: "阿里巴巴", Category: CatShopping},
	"taobao":        {ID: "taobao", Name: "淘宝", Category: CatShopping},
	"tmall":         {ID: "tmall", Name: "天猫", Category: CatShopping},
	"jd":            {ID: "jd", Name: "京东", Category: CatShopping},
	"pdd":           {ID: "pdd", Name: "拼多多", Category: CatShopping},
	"alipay":        {ID: "alipay", Name: "支付宝", Category: CatPayment},
	"baidu":         {ID: "baidu", Name: "百度", Category: CatNews},
	"bilibili":      {ID: "bilibili", Name: "哔哩哔哩", Category: CatVideo},
	"douyin":        {ID: "douyin", Name: "抖音", Category: CatVideo},
	"kuaishou":      {ID: "kuaishou", Name: "快手", Category: CatVideo},
	"iqiyi":         {ID: "iqiyi", Name: "爱奇艺", Category: CatVideo},
	"youku":         {ID: "youku", Name: "优酷", Category: CatVideo},
	"tencent-video": {ID: "tencent-video", Name: "腾讯视频", Category: CatVideo},
	"netease":       {ID: "netease", Name: "网易", Category: CatNews},
	"netflix":       {ID: "netflix", Name: "Netflix", Category: CatVideo},
	"youtube":       {ID: "youtube", Name: "YouTube", Category: CatVideo},
	"google":        {ID: "google", Name: "Google", Category: CatTool},
	"apple":         {ID: "apple", Name: "Apple", Category: CatTool},
	"microsoft":     {ID: "microsoft", Name: "微软", Category: CatOffice},
	"github":        {ID: "github", Name: "GitHub", Category: CatOffice},
	"telegram":      {ID: "telegram", Name: "Telegram", Category: CatIM},
	"whatsapp":      {ID: "whatsapp", Name: "WhatsApp", Category: CatIM},
	"weibo":         {ID: "weibo", Name: "微博", Category: CatSocial},
	"sina":          {ID: "sina", Name: "新浪", Category: CatNews},
	"zhihu":         {ID: "zhihu", Name: "知乎", Category: CatSocial},
	"xiaohongshu":   {ID: "xiaohongshu", Name: "小红书", Category: CatSocial},
	"meituan":       {ID: "meituan", Name: "美团", Category: CatLife},
	"didi":          {ID: "didi", Name: "滴滴", Category: CatLife},
	"openai":        {ID: "openai", Name: "OpenAI", Category: CatAI},
	"anthropic":     {ID: "anthropic", Name: "Anthropic", Category: CatAI},
	"opencode":      {ID: "opencode", Name: "OpenCode", Category: CatAI},
	"cloudflare":    {ID: "cloudflare", Name: "Cloudflare", Category: CatTool},
	"amazon":        {ID: "amazon", Name: "Amazon", Category: CatShopping},
	"netflix-cdn":   {ID: "netflix-cdn", Name: "Netflix CDN", Category: CatVideo},
}

// CategoryForGroup 把 domain-list 的组名/分类文件名映射到中文分类。
//
// 组名里带 category- 前缀的是分类聚合文件，其余按关键词判断。
// 这个函数是启发式的——判错的会落到"其他"，可以在 meta 里显式覆盖。
func CategoryForGroup(name string) Category {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, "category-") {
		bare := strings.TrimPrefix(n, "category-")
		for _, r := range categoryRules {
			if strings.HasPrefix(bare, r.prefix) {
				return r.cat
			}
		}
		return CatOther
	}
	switch {
	case strings.Contains(n, "bilibili"), strings.Contains(n, "iqiyi"), strings.Contains(n, "youku"),
		strings.Contains(n, "douyin"), strings.Contains(n, "kuaishou"), strings.Contains(n, "vimeo"),
		strings.Contains(n, "netflix"), strings.Contains(n, "youtube"), strings.Contains(n, "tv"):
		return CatVideo
	case strings.Contains(n, "music"), strings.Contains(n, "netease"), strings.Contains(n, "spotify"),
		strings.Contains(n, "kugou"), strings.Contains(n, "kuwo"):
		return CatMusic
	case strings.Contains(n, "game"), strings.Contains(n, "steam"), strings.Contains(n, "epic"):
		return CatGame
	case strings.Contains(n, "wechat"), strings.Contains(n, "weixin"), strings.Contains(n, "qq"),
		strings.Contains(n, "telegram"), strings.Contains(n, "whatsapp"), strings.Contains(n, "line"):
		return CatIM
	case strings.Contains(n, "weibo"), strings.Contains(n, "twitter"), strings.Contains(n, "facebook"),
		strings.Contains(n, "instagram"), strings.Contains(n, "reddit"), strings.Contains(n, "zhihu"):
		return CatSocial
	case strings.Contains(n, "taobao"), strings.Contains(n, "tmall"), strings.Contains(n, "jd"),
		strings.Contains(n, "pdd"), strings.Contains(n, "amazon"), strings.Contains(n, "shop"):
		return CatShopping
	case strings.Contains(n, "alipay"), strings.Contains(n, "pay"), strings.Contains(n, "unionpay"):
		return CatPayment
	case strings.Contains(n, "github"), strings.Contains(n, "gitlab"), strings.Contains(n, "office"),
		strings.Contains(n, "slack"), strings.Contains(n, "notion"):
		return CatOffice
	case strings.Contains(n, "ai"), strings.Contains(n, "openai"), strings.Contains(n, "claude"),
		strings.Contains(n, "gemini"):
		return CatAI
	case strings.Contains(n, "-ads"), strings.Contains(n, "telemetry"):
		return CatAd
	}
	return CatOther
}

// categoryRules 把 domain-list 的 category-* 文件名映射到中文分类。
// 顺序敏感：更具体的前缀必须排在更宽泛的前面。
var categoryRules = []struct {
	prefix string
	cat    Category
}{
	{"ads-and-telemetry", CatAd},
	{"ads-all", CatAd},
	{"ads", CatAd},
	{"telemetry", CatAd},
	{"consent-management", CatAd},
	{"ai-chat", CatAI},
	{"ai-", CatAI},
	{"chat", CatAI},
	{"game-accelerator", CatGame},
	{"game-platforms-download", CatGame},
	{"flight-simulator", CatGame},
	{"enhance-gaming", CatGame},
	{"games", CatGame},
	{"media", CatVideo},
	{"entertainment", CatVideo},
	{"emby", CatVideo},
	{"acg", CatVideo},
	{"novel", CatVideo},
	{"music", CatMusic},
	{"social-media", CatSocial},
	{"forums", CatSocial},
	{"blog", CatSocial},
	{"wiki", CatSocial},
	{"communication", CatIM},
	{"voip", CatIM},
	{"ecommerce", CatShopping},
	{"shopping", CatShopping},
	{"retail", CatShopping},
	{"bank", CatFinance},
	{"bourse", CatFinance},
	{"securities", CatFinance},
	{"finance", CatFinance},
	{"insurance", CatFinance},
	{"cryptocurrency", CatFinance},
	{"payment", CatPayment},
	{"education", CatEdu},
	{"mooc", CatEdu},
	{"scholar", CatEdu},
	{"olympiad", CatEdu},
	{"documents", CatOffice},
	{"collaborate", CatOffice},
	{"password-management", CatOffice},
	{"enterprise-query", CatOffice},
	{"number-verification", CatOffice},
	{"remote-control", CatTool},
	{"proxy-tunnels", CatTool},
	{"vpnservices", CatTool},
	{"anticensorship", CatTool},
	{"network-security", CatTool},
	{"antivirus", CatTool},
	{"browser", CatTool},
	{"cas", CatTool},
	{"container", CatTool},
	{"dev", CatTool},
	{"doh", CatTool},
	{"ddns", CatTool},
	{"ntp", CatTool},
	{"speedtest", CatTool},
	{"stun", CatTool},
	{"urlshortner", CatTool},
	{"web-archive", CatTool},
	{"ip-geo-detect", CatTool},
	{"tech-media", CatNews},
	{"news", CatNews},
	{"tech", CatNews},
	{"gov", CatNews},
	{"orgs", CatNews},
	{"ipfs", CatP2P},
	{"public-tracker", CatP2P},
	{"pt", CatP2P},
	{"android-app-download", CatDownload},
	{"netdisk", CatDownload},
	{"cdn", CatTool},
	{"httpdns", CatTool},
	{"travel", CatTravel},
	{"flight", CatTravel},
	{"logistics", CatLife},
	{"food", CatLife},
	{"hospital", CatLife},
	{"medicine", CatLife},
	{"automobile", CatLife},
	{"mobile-repair", CatLife},
	{"companies", CatOther},
}
