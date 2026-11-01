package cloud

import (
	"compress/gzip"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/auth"
)

//go:embed web
var webFS embed.FS

// Config 是云管服务的启动参数。
type Config struct {
	StorePath     string        // 状态文件
	AdminUser     string        // 控制台账号
	AdminPassword string        // 控制台密码（生产必须改）
	JWTSecret     []byte        // 会话签名密钥
	SessionTTL    time.Duration // 会话有效期
	ExecTimeout   time.Duration // 单条命令超时
	ReleaseDir    string        // 在线升级的发布包目录
	PublicBase    string        // 设备可访问的云端根地址（反代场景必填）
}

// Service 是云端控制面：HTTP API + 隧道 + 审计。
type Service struct {
	cfg     Config
	store   *Store
	hub     *Hub
	tm      *auth.TokenManager
	started time.Time
	limiter loginLimiter
	rc      releaseCache
}

// NewService 打开状态文件并装配服务。
func NewService(cfg Config) (*Service, error) {
	if len(cfg.JWTSecret) == 0 {
		return nil, errors.New("JWTSecret 不能为空")
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.ExecTimeout == 0 {
		cfg.ExecTimeout = 15 * time.Second
	}
	st, err := OpenStore(cfg.StorePath)
	if err != nil {
		return nil, err
	}
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	return &Service{
		cfg:     cfg,
		store:   st,
		hub:     NewHub(),
		tm:      auth.NewTokenManager(cfg.JWTSecret, cfg.SessionTTL),
		started: time.Now(),
	}, nil
}

// Store 暴露给测试用。
func (s *Service) Store() *Store { return s.store }

// Hub 暴露给测试用。
func (s *Service) Hub() *Hub { return s.hub }

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code, ecode int, msg, detail string) {
	writeJSON(w, code, map[string]interface{}{"code": ecode, "message": msg, "detail": detail})
}

// Handler 返回完整路由。
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()

	// 控制台
	mux.HandleFunc("/", s.handleConsole)
	mux.HandleFunc("POST /cloud/v1/login", s.handleLogin)
	mux.HandleFunc("POST /cloud/v1/change-password", s.authed(s.handleChangePassword))
	mux.HandleFunc("GET /cloud/v1/overview", s.authed(s.handleOverview))
	mux.HandleFunc("GET /cloud/v1/devices", s.authed(s.handleDevices))
	mux.HandleFunc("DELETE /cloud/v1/devices/{id}", s.authed(s.handleDeleteDevice))
	mux.HandleFunc("POST /cloud/v1/devices/{id}/exec", s.authed(s.handleExec))
	// UI 代理在 handleConsole 中处理，避免与 GET / 冲突
	mux.HandleFunc("GET /cloud/v1/tokens", s.authed(s.handleListTokens))
	mux.HandleFunc("POST /cloud/v1/tokens", s.authed(s.handleCreateToken))
	mux.HandleFunc("DELETE /cloud/v1/tokens/{id}", s.authed(s.handleDeleteToken))
	mux.HandleFunc("GET /cloud/v1/audit", s.authed(s.handleAudit))
	mux.HandleFunc("GET /cloud/v1/policy", s.authed(s.handlePolicy))

	// 在线升级
	mux.HandleFunc("GET /cloud/v1/releases/latest", s.authed(s.handleReleaseLatest))
	mux.HandleFunc("GET /cloud/v1/releases/download", s.handleReleaseDownload)
	mux.HandleFunc("POST /cloud/v1/devices/{id}/upgrade", s.authed(s.handleDeviceUpgrade))

	// 设备侧
	mux.HandleFunc("POST /cloud/v1/agent/enroll", s.handleEnroll)
	mux.HandleFunc("POST /cloud/v1/agent/report", s.handleReport)
	mux.HandleFunc("POST /cloud/v1/agent/result", s.handleResult)
	mux.HandleFunc("GET /cloud/v1/agent/stream", s.handleStream)

	return &gzipHandler{handler: mux}
}

type gzipHandler struct {
	handler http.Handler
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer    *gzip.Writer
	wroteHead bool
	skip      bool // 该响应不该有压缩体（204/304、或已自带 Content-Encoding）
}

// WriteHeader 在写头之前决定这条响应到底压不压。
// 旧实现是在 handler 之前就把 Content-Encoding: gzip 塞进头里，于是 204/304
// 这种"按规范不允许有消息体"的响应也会被 Close 写入 gzip 头尾，产生非法响应；
// 同时 handler 自己设的 Content-Length 会被 gzip 破坏（长度对不上，客户端截断）。
func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHead {
		return
	}
	g.wroteHead = true
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified {
		g.skip = true
	}
	h := g.ResponseWriter.Header()
	if g.skip {
		h.Del("Content-Encoding")
	} else {
		h.Set("Content-Encoding", "gzip")
	}
	h.Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHead {
		g.WriteHeader(http.StatusOK)
	}
	if g.skip {
		return g.ResponseWriter.Write(b)
	}
	return g.writer.Write(b)
}

func (g *gzipResponseWriter) Flush() {
	if !g.skip {
		g.writer.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (gh *gzipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Vary 必须无条件发：同一 URL 对支持/不支持 gzip 的客户端返回的字节不同，
	// 少了它，中间的缓存/CDN 会把压缩体发给不认 gzip 的客户端（乱码）。
	w.Header().Add("Vary", "Accept-Encoding")

	accept := r.Header.Get("Accept-Encoding")
	if !strings.Contains(accept, "gzip") || r.Method == http.MethodHead {
		gh.handler.ServeHTTP(w, r)
		return
	}
	// 长连接流不能压缩（会破坏逐帧 flush）；二进制发布包压缩纯属浪费 CPU，
	// 而且会破坏 Range 断点续传。
	if strings.HasPrefix(r.URL.Path, "/cloud/v1/agent/stream") ||
		strings.HasPrefix(r.URL.Path, "/cloud/v1/releases/download") {
		gh.handler.ServeHTTP(w, r)
		return
	}
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		gh.handler.ServeHTTP(w, r)
		return
	}
	grw := &gzipResponseWriter{ResponseWriter: w, writer: gz}
	gh.handler.ServeHTTP(grw, r)
	// 没真正写过体（204/304）就不要 Close：Close 会写出 gzip 头尾，
	// 给一个本该没有体的响应凭空加上 20 字节，客户端会报协议错误。
	if !grw.skip {
		_ = gz.Close()
	}
}

func (s *Service) handleConsole(w http.ResponseWriter, r *http.Request) {
	// UI 代理路径：委托给 handleDeviceUI
	if strings.HasPrefix(r.URL.Path, "/cloud/v1/devices/") && strings.Contains(r.URL.Path, "/ui") {
		s.handleDeviceUI(w, r)
		return
	}
	// 如果有 proxy_device cookie，且路径不是云管理路由，转发到设备 UI
	if c, err := r.Cookie("proxy_device"); err == nil && c.Value != "" {
		if !strings.HasPrefix(r.URL.Path, "/cloud/v1/") && !strings.HasPrefix(r.URL.Path, "/assets/") {
			// 重写为代理路径
			r.URL.Path = "/cloud/v1/devices/" + c.Value + "/ui" + r.URL.Path
			s.handleDeviceUI(w, r)
			return
		}
	}
	if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/cloud.html" {
		b, err := webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "console missing", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(b)
		return
	}
	// 静态资源（vite 构建产物）：/assets/xxx.js 等。
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		b, err := webFS.ReadFile("web" + r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ct := "application/octet-stream"
		switch {
		case strings.HasSuffix(r.URL.Path, ".js"):
			ct = "application/javascript"
		case strings.HasSuffix(r.URL.Path, ".css"):
			ct = "text/css"
		case strings.HasSuffix(r.URL.Path, ".svg"):
			ct = "image/svg+xml"
		case strings.HasSuffix(r.URL.Path, ".png"):
			ct = "image/png"
		case strings.HasSuffix(r.URL.Path, ".ico"):
			ct = "image/x-icon"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(b)
		return
	}
	http.NotFound(w, r)
}

// ---------------------------------------------------------------- 控制台鉴权

func (s *Service) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			writeErr(w, 401, 20100, "unauthorized", "缺少会话令牌")
			return
		}
		sub, _, err := s.tm.VerifyWithIssuedAt(tok)
		if err != nil {
			writeErr(w, 401, 20100, "unauthorized", "会话已失效，请重新登录")
			return
		}
		user, epoch, ok := splitSubject(sub)
		if !ok || epoch != s.store.GetSessionEpoch() {
			// 改过密码：此前签发的令牌一律作废，强制重新登录
			writeErr(w, 401, 20100, "unauthorized", "会话已失效，请重新登录")
			return
		}
		r.Header.Set("X-Actor", user)
		next(w, r)
	}
}

// splitSubject 把 JWT 的 sub 拆成「用户名 + 会话代际」。
// 旧格式（没有 #）视为代际 0，与未改过密码的库一致，不会把老会话一脚踢掉。
func splitSubject(sub string) (user string, epoch int64, ok bool) {
	i := strings.LastIndex(sub, "#")
	if i < 0 {
		if sub == "" {
			return "", 0, false
		}
		return sub, 0, true
	}
	n, err := strconv.ParseInt(sub[i+1:], 10, 64)
	if err != nil || i == 0 {
		return "", 0, false
	}
	return sub[:i], n, true
}

// loginLimiter 是登录失败节流：按来源 IP 计数，超阈值后锁定一段时间。
//
// 没有它的话，/cloud/v1/login 是个公开的 HTTPS 端点，配合弱口令就是一台
// 可无限重试的密码机。计数器只放内存：进程重启后清零，代价远小于维护落盘。
type loginLimiter struct {
	mu   sync.Mutex
	m    map[string]*loginAttempt
	last time.Time
}

type loginAttempt struct {
	fails      int
	lockedTill time.Time
	lastSeen   time.Time
}

const (
	loginMaxFails = 8
	loginLockFor  = 10 * time.Minute
	loginFailTTL  = 30 * time.Minute
)

func (l *loginLimiter) ensure() {
	if l.m == nil {
		l.m = make(map[string]*loginAttempt)
		l.last = time.Now()
	}
	// 每 10 分钟顺手清一次过期条目，避免被随机 IP 打爆内存
	if time.Since(l.last) > 10*time.Minute {
		now := time.Now()
		for k, a := range l.m {
			if now.Sub(a.lastSeen) > loginFailTTL && now.After(a.lockedTill) {
				delete(l.m, k)
			}
		}
		l.last = now
	}
}

// blocked 返回剩余锁定时间；0 表示放行。
func (l *loginLimiter) blocked(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	a := l.m[ip]
	if a == nil {
		return 0
	}
	if d := time.Until(a.lockedTill); d > 0 {
		return d
	}
	return 0
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	a := l.m[ip]
	if a == nil {
		a = &loginAttempt{}
		l.m[ip] = a
	}
	a.fails++
	a.lastSeen = time.Now()
	if a.fails >= loginMaxFails {
		a.lockedTill = time.Now().Add(loginLockFor)
		a.fails = 0 // 锁定期满重新计数，避免一次锁死永久封禁
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	delete(l.m, ip)
}

func actor(r *http.Request) string {
	if a := r.Header.Get("X-Actor"); a != "" {
		return a
	}
	return "unknown"
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	ip := clientIP(r)
	if wait := s.limiter.blocked(ip); wait > 0 {
		s.store.AddAudit(AuditEntry{Actor: "console:" + ip, Action: "console.login", Result: "deny", Detail: "锁定中 " + wait.Round(time.Second).String()})
		_ = s.store.Save()
		writeErr(w, 429, 20103, "too many attempts",
			fmt.Sprintf("失败次数过多，请 %d 分钟后再试", int(wait.Minutes())+1))
		return
	}
	uOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.cfg.AdminUser)) == 1
	// 使用存储的密码（如果有），否则使用配置默认值
	adminPw := s.cfg.AdminPassword
	if stored := s.store.GetAdminPassword(); stored != "" {
		adminPw = stored
	}
	pOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(adminPw)) == 1
	if !uOK || !pOK {
		s.limiter.fail(ip)
		s.store.AddAudit(AuditEntry{Actor: "console:" + ip, Action: "console.login", Result: "fail", Detail: req.Username})
		_ = s.store.Save()
		writeErr(w, 401, 20101, "invalid credentials", "账号或密码错误")
		return
	}
	s.limiter.reset(ip)
	// sub 里带会话代际：改密码后 +1，旧令牌立刻失效
	tok, err := s.tm.Issue(req.Username + "#" + strconv.FormatInt(s.store.GetSessionEpoch(), 10))
	if err != nil {
		writeErr(w, 500, 20102, "token issue failed", err.Error())
		return
	}
	s.store.AddAudit(AuditEntry{Actor: req.Username, Action: "console.login", Result: "ok", Detail: clientIP(r)})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{"token": tok, "user": req.Username})
}

// ---------------------------------------------------------------- 控制台 API

func (s *Service) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	if len(req.NewPassword) < 4 {
		writeErr(w, 400, 20002, "password too short", "新密码至少 4 位")
		return
	}
	// 验证旧密码
	adminPw := s.cfg.AdminPassword
	if stored := s.store.GetAdminPassword(); stored != "" {
		adminPw = stored
	}
	if subtle.ConstantTimeCompare([]byte(req.OldPassword), []byte(adminPw)) != 1 {
		writeErr(w, 401, 20101, "invalid old password", "当前密码错误")
		return
	}
	// 更新密码
	if err := s.store.SetAdminPassword(req.NewPassword); err != nil {
		writeErr(w, 500, 20102, "save failed", err.Error())
		return
	}
	// 改密成功即作废所有已签发令牌：密码泄露场景下，不踢掉旧会话等于没改。
	if err := s.store.BumpSessionEpoch(); err != nil {
		log.Printf("cloud: 递增会话代际失败: %v", err)
	}
	s.store.AddAudit(AuditEntry{Actor: actor(r), Action: "console.change_password", Result: "ok", Detail: clientIP(r)})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{"ok": true, "reauth": true})
}

func (s *Service) handleOverview(w http.ResponseWriter, r *http.Request) {
	devs := s.store.ListDevices()
	online := s.hub.OnlineIDs()
	on := 0
	for _, d := range devs {
		if online[d.ID] {
			on++
		}
	}
	reads, writes := AllowedList()
	writeJSON(w, 200, map[string]interface{}{
		"devices_total":  len(devs),
		"devices_online": on,
		"server_uptime":  int64(time.Since(s.started).Seconds()),
		"now":            time.Now().Unix(),
		"policy":         map[string]interface{}{"reads": reads, "writes": writes},
	})
}

func (s *Service) handleDevices(w http.ResponseWriter, r *http.Request) {
	devs := s.store.ListDevices()
	online := s.hub.OnlineIDs()
	for i := range devs {
		devs[i].Online = online[devs[i].ID]
	}
	writeJSON(w, 200, map[string]interface{}{"devices": devs})
}

func (s *Service) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, ok := s.store.GetDevice(id)
	if !ok {
		writeErr(w, 404, 20201, "device not found", id)
		return
	}
	if err := s.store.DeleteDevice(id); err != nil {
		writeErr(w, 500, 20202, "delete failed", err.Error())
		return
	}
	// 断开隧道：删了设备还留着连接，等于凭据注销不生效。
	s.hub.mu.RLock()
	c := s.hub.conns[id]
	s.hub.mu.RUnlock()
	if c != nil {
		c.close()
	}
	s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.delete", Result: "ok", Detail: id})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

func (s *Service) handleExec(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, ok := s.store.GetDevice(id)
	if !ok {
		writeErr(w, 404, 20201, "device not found", id)
		return
	}
	var req struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		Body    string `json:"body"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if !PathAllowed(req.Method, req.Path) {
		s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.exec", Result: "deny", Detail: req.Method + " " + req.Path})
		_ = s.store.Save()
		writeErr(w, 403, 20203, "path not allowed", "该接口不在白名单内："+req.Method+" "+req.Path)
		return
	}
	if NeedConfirm(req.Method, req.Path) {
		if strings.TrimSpace(req.Confirm) != dev.Name {
			writeErr(w, 400, 20204, "confirm required", "写操作需要在 confirm 里填入设备名「"+dev.Name+"」")
			return
		}
	}
	if !s.hub.Online(id) {
		writeErr(w, 409, 20205, "device offline", "设备当前没有隧道连接")
		return
	}
	res, err := s.hub.Call(id, Frame{Type: "exec", Method: strings.ToUpper(req.Method), Path: req.Path, Body: req.Body}, s.cfg.ExecTimeout)
	if err != nil {
		result := "fail"
		if errors.Is(err, ErrTimeout) {
			result = "timeout"
		}
		s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.exec", Result: result, Detail: req.Method + " " + req.Path + " " + err.Error()})
		_ = s.store.Save()
		code := 504
		if errors.Is(err, ErrOffline) {
			code = 409
		}
		writeErr(w, code, 20206, "exec failed", err.Error())
		return
	}
	// 响应体用 base64 传（可能是任意字节），控制台再解。
	status := res.Status
	if status == 0 {
		status = 200
	}
	s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.exec", Result: "ok", Detail: req.Method + " " + req.Path})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{
		"status": status,
		"body":   res.Data,
		"error":  res.Error,
	})
}

func (s *Service) handleDeviceUI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		// 从路径中手动提取 device ID（handleConsole 委托时 PathValue 不可用）
		parts := strings.Split(r.URL.Path, "/")
		for i, p := range parts {
			if p == "devices" && i+1 < len(parts) {
				id = parts[i+1]
				break
			}
		}
	}
	dev, ok := s.store.GetDevice(id)
	if !ok {
		writeErr(w, 404, 20201, "device not found", id)
		return
	}
	if !s.hub.Online(id) {
		writeErr(w, 409, 20205, "device offline", "设备当前没有隧道连接")
		return
	}
	// 从 URL 路径中提取要代理的设备端路径。
	uiPrefix := "/cloud/v1/devices/" + id + "/ui"
	devPath := strings.TrimPrefix(r.URL.Path, uiPrefix)
	if devPath == "" {
		devPath = "/"
	}
	if !strings.HasPrefix(devPath, "/") {
		devPath = "/" + devPath
	}
	// 判断是否为静态资源（JS/CSS/图片/字体/woff）或登录端点或 SPA 路由
	isStatic := false
	lowerPath := strings.ToLower(devPath)
	for _, ext := range []string{".js", ".css", ".png", ".jpg", ".jpeg", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".map", ".json"} {
		if strings.HasSuffix(lowerPath, ext) {
			isStatic = true
			break
		}
	}
	// 登录端点也不需要认证
	isLogin := devPath == "/api/v1/auth/login"
	// SPA 路由（不以 /api/ 开头的非静态路径）也不需要代理认证（设备端自行认证）
	isSPARoute := !strings.HasPrefix(devPath, "/api/")
	// HTML 页面和 API 代理需要认证；静态资源、登录端点和 SPA 路由不需要
	// 但如果有 proxy_device cookie（已通过云控制台认证），跳过认证
	hasProxyCookie := false
	if c, err := r.Cookie("proxy_device"); err == nil && c.Value == id {
		hasProxyCookie = true
	}
	if !isStatic && !isLogin && !isSPARoute && !hasProxyCookie {
		tok := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tok = strings.TrimPrefix(h, "Bearer ")
		}
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok == "" {
			writeErr(w, 401, 20100, "unauthorized", "缺少会话令牌")
			return
		}
		sub, _, err := s.tm.VerifyWithIssuedAt(tok)
		if err != nil {
			writeErr(w, 401, 20100, "unauthorized", "会话已失效，请重新登录")
			return
		}
		// 与 authed 一致：改密后旧令牌一律失效，设备页代理也不能例外
		if _, epoch, ok := splitSubject(sub); !ok || epoch != s.store.GetSessionEpoch() {
			writeErr(w, 401, 20100, "unauthorized", "会话已失效，请重新登录")
			return
		}
	}
	// 读取请求体（POST/PUT/PATCH/DELETE）
	reqBody := ""
	if r.Method != "GET" && r.Method != "HEAD" {
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if len(bodyBytes) > 0 {
			reqBody = base64.StdEncoding.EncodeToString(bodyBytes)
		}
	}
	// 设置 cookie，以便后续请求（如 /login）能被路由到设备
	http.SetCookie(w, &http.Cookie{
		Name:     "proxy_device",
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// 通过隧道获取设备响应
	res, err := s.hub.Call(id, Frame{Type: "exec", Method: r.Method, Path: devPath, Body: reqBody, Proxy: true}, s.cfg.ExecTimeout)
	if err != nil {
		writeErr(w, 502, 20206, "proxy failed", err.Error())
		return
	}
	status := res.Status
	if status == 0 {
		status = 200
	}
	data := []byte{}
	if res.Data != "" {
		raw, derr := base64.StdEncoding.DecodeString(res.Data)
		if derr == nil {
			data = raw
		}
	}
	// 根据扩展名或路径推断 Content-Type
	ct := "application/octet-stream"
	if strings.HasPrefix(devPath, "/api/") {
		ct = "application/json"
	} else if strings.HasSuffix(lowerPath, ".html") || devPath == "/" {
		ct = "text/html; charset=utf-8"
	} else if strings.HasSuffix(lowerPath, ".js") {
		ct = "application/javascript"
	} else if strings.HasSuffix(lowerPath, ".css") {
		ct = "text/css"
	} else if strings.HasSuffix(lowerPath, ".svg") {
		ct = "image/svg+xml"
	} else if strings.HasSuffix(lowerPath, ".png") {
		ct = "image/png"
	} else if strings.HasSuffix(lowerPath, ".ico") {
		ct = "image/x-icon"
	} else if strings.HasSuffix(lowerPath, ".json") {
		ct = "application/json"
	} else if strings.HasSuffix(lowerPath, ".woff2") {
		ct = "font/woff2"
	} else if strings.HasSuffix(lowerPath, ".woff") {
		ct = "font/woff"
	}
	w.Header().Set("Content-Type", ct)
	// HTML 页面：重写资源 URL 并注入 fetch/XHR 拦截脚本（仅 GET 请求）
	if r.Method == "GET" && ct == "text/html; charset=utf-8" {
		html := string(data)
		proxyBase := "/cloud/v1/devices/" + id + "/ui"
		// 重写 /assets/ 为代理路径
		html = strings.ReplaceAll(html, `href="/assets/`, `href="`+proxyBase+`/assets/`)
		html = strings.ReplaceAll(html, `src="/assets/`, `src="`+proxyBase+`/assets/`)
		// 注入 <base> 标签和 fetch/XHR 拦截脚本
		baseTag := `<base href="` + proxyBase + `/">`
		proxyScript := `<script>
(function(){
  var P="` + proxyBase + `";
  console.log("PROXY SCRIPT LOADED", P);
  // 拦截 history.pushState/replaceState
  var _push=window.history.pushState;
  var _replace=window.history.replaceState;
  Object.defineProperty(window.history, "pushState", {value: function(s,t,u){
    if(typeof u==="string" && u.startsWith("/") && !u.startsWith(P)){u=P+u;}
    return _push.call(window.history,s,t,u);
  }, writable: false, configurable: false});
  Object.defineProperty(window.history, "replaceState", {value: function(s,t,u){
    if(typeof u==="string" && u.startsWith("/") && !u.startsWith(P)){u=P+u;}
    return _replace.call(window.history,s,t,u);
  }, writable: false, configurable: false});
  // fetch/XHR 拦截
  var origFetch=window.fetch;
  window.fetch=function(input,init){
    if(typeof input==="string" && input.startsWith("/api/")){input=P+input;}
    else if(input instanceof Request){var u=new URL(input.url);if(u.pathname.startsWith("/api/"))input=new Request(P+u.pathname+u.search,input);}
    return origFetch(input,init);
  };
  var origOpen=XMLHttpRequest.prototype.open;
  Object.defineProperty(XMLHttpRequest.prototype, "open", {value: function(method,url,async,user,pass){
    if(typeof url==="string" && url.startsWith("/api/")){url=P+url; console.log("XHR rewritten:", url);}
    return origOpen.call(this,method,url,async,user,pass);
  }, writable: false, configurable: false});
})();
</script>`
		if strings.Contains(html, "<head>") {
			html = strings.Replace(html, "<head>", "<head>"+baseTag+proxyScript, 1)
		} else if strings.Contains(html, "<HEAD>") {
			html = strings.Replace(html, "<HEAD>", "<HEAD>"+baseTag+proxyScript, 1)
		}
		data = []byte(html)
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	_, _ = w.Write(data)
	_ = dev
}

func (s *Service) handleListTokens(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"tokens": s.store.ListTokens()})
}

func (s *Service) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note       string `json:"note"`
		TTLMinutes int    `json:"ttl_minutes"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req)
	if req.TTLMinutes <= 0 {
		req.TTLMinutes = 60
	}
	tok, raw := s.store.CreateToken(req.Note, time.Duration(req.TTLMinutes)*time.Minute)
	s.store.AddAudit(AuditEntry{Actor: actor(r), Action: "token.create", Result: "ok", Detail: tok.ID + " " + req.Note})
	_ = s.store.Save()
	// 明文只在这里返回一次。
	writeJSON(w, 200, map[string]interface{}{
		"id": tok.ID, "token": raw, "expires_at": tok.ExpiresAt, "note": tok.Note,
	})
}

func (s *Service) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteToken(id); err != nil {
		writeErr(w, 404, 20207, "token not found", id)
		return
	}
	s.store.AddAudit(AuditEntry{Actor: actor(r), Action: "token.delete", Result: "ok", Detail: id})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

func (s *Service) handleAudit(w http.ResponseWriter, r *http.Request) {
	n := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		var x int
		if _, err := fmtSscan(v, &x); err == nil && x > 0 {
			n = x
		}
	}
	writeJSON(w, 200, map[string]interface{}{"entries": s.store.ListAudit(n)})
}

func (s *Service) handlePolicy(w http.ResponseWriter, r *http.Request) {
	reads, writes := AllowedList()
	writeJSON(w, 200, map[string]interface{}{"reads": reads, "writes": writes})
}

// 小工具：避免为一次 Atoi 引入 strconv 的歧义（保持与 writeErr 一致的可读性）。
func fmtSscan(s string, out *int) (int, error) {
	n := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(c-'0')
		if n > 5000 {
			n = 5000
			break
		}
	}
	*out = n
	return 1, nil
}

var _ = base64.RawURLEncoding
var _ = log.Printf
