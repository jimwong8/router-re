package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 这几条覆盖的是"看上去能用、出事才发现"的三类问题：
// 密码泄露后改密码踢不掉旧会话、登录接口可无限爆破、发布包没校验就下发。

// 改密码必须让此前签发的令牌立即失效。
// 没有会话代际时，旧令牌能一直用到 12 小时 TTL 结束，等于改密码没效果。
func TestChangePasswordInvalidatesOldTokens(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	old := consoleToken(t, srv.URL)

	code, _ := doJSON(t, "POST", srv.URL+"/cloud/v1/change-password", old,
		map[string]string{"old_password": "pw", "new_password": "newpw123"})
	if code != 200 {
		t.Fatalf("改密码 HTTP %d", code)
	}

	// 旧令牌必须被拒
	code, _ = doJSON(t, "GET", srv.URL+"/cloud/v1/overview", old, nil)
	if code != 401 {
		t.Fatalf("改密码后旧令牌仍然可用（HTTP %d）——会话没有被作废", code)
	}

	// 新密码能登录，新令牌可用
	b, _ := json.Marshal(map[string]string{"username": "admin", "password": "newpw123"})
	resp, err := http.Post(srv.URL+"/cloud/v1/login", "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("新密码登录失败 HTTP %d", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	code, _ = doJSON(t, "GET", srv.URL+"/cloud/v1/overview", out.Token, nil)
	if code != 200 {
		t.Fatalf("新令牌不可用（HTTP %d）", code)
	}
}

// 连续失败到阈值后必须锁定，不能无限重试。
func TestLoginRateLimitLocksAfterFailures(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	body := map[string]string{"username": "admin", "password": "wrong"}

	var locked bool
	for i := 0; i < loginMaxFails+2; i++ {
		code, out := doJSON(t, "POST", srv.URL+"/cloud/v1/login", "", body)
		if code == 429 {
			locked = true
			if msg, _ := out["detail"].(string); !strings.Contains(msg, "分钟") {
				t.Fatalf("锁定提示不可读: %v", out)
			}
			break
		}
		if code != 401 {
			t.Fatalf("第 %d 次失败返回 HTTP %d，期望 401", i+1, code)
		}
	}
	if !locked {
		t.Fatalf("%d 次失败后仍未锁定，登录接口可被无限爆破", loginMaxFails)
	}

	// 锁定期间即使密码正确也必须拒绝
	good := map[string]string{"username": "admin", "password": "pw"}
	if code, _ := doJSON(t, "POST", srv.URL+"/cloud/v1/login", "", good); code != 429 {
		t.Fatalf("锁定期间正确密码仍被放行（HTTP %d）", code)
	}
}

// 登录成功后失败计数要清零，否则偶发输错一次会累积到锁定。
func TestLoginRateLimitResetsOnSuccess(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	for i := 0; i < loginMaxFails-1; i++ {
		doJSON(t, "POST", srv.URL+"/cloud/v1/login", "",
			map[string]string{"username": "admin", "password": "wrong"})
	}
	if code, _ := doJSON(t, "POST", srv.URL+"/cloud/v1/login", "",
		map[string]string{"username": "admin", "password": "pw"}); code != 200 {
		t.Fatalf("差一次到阈值时正确密码被拒（HTTP %d）", code)
	}
	// 清零后再错几次不应立刻锁
	for i := 0; i < 3; i++ {
		if code, _ := doJSON(t, "POST", srv.URL+"/cloud/v1/login", "",
			map[string]string{"username": "admin", "password": "wrong"}); code == 429 {
			t.Fatal("成功登录后失败计数没有清零")
		}
	}
}

// ---------------------------------------------------------------- 发布包

func makeReleaseDir(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	payload := make([]byte, 8192)
	copy(payload, []byte{0x7f, 'E', 'L', 'F'})
	for i := 4; i < len(payload); i++ {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(dir, "mgmtd-v9.9.9"), payload, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, payload
}

// 清单里写的 sha256 与文件实际内容不符时必须拒绝下发，
// 否则运维改错一个字符就会把错误的东西推到所有设备。
func TestResolveReleaseRejectsBadManifestHash(t *testing.T) {
	dir, _ := makeReleaseDir(t)
	manifest := `{"version":"9.9.9","file":"mgmtd-v9.9.9","sha256":"` + strings.Repeat("0", 64) + `"}`
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(t)
	svc.cfg.ReleaseDir = dir
	if _, err := svc.resolveRelease(); err == nil {
		t.Fatal("清单哈希与文件不符却通过了校验")
	} else if !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("错误信息没指出哈希问题: %v", err)
	}
}

// 没有清单时退化成"目录里最新的 mgmtd* 文件"，并且哈希要算对。
func TestResolveReleaseFallsBackToNewestFile(t *testing.T) {
	dir, payload := makeReleaseDir(t)
	svc := newTestService(t)
	svc.cfg.ReleaseDir = dir

	rel, err := svc.resolveRelease()
	if err != nil {
		t.Fatalf("resolveRelease: %v", err)
	}
	want := sha256.Sum256(payload)
	if rel.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("sha256 = %s，期望 %s", rel.SHA256, hex.EncodeToString(want[:]))
	}
	if rel.Size != int64(len(payload)) {
		t.Fatalf("size = %d，期望 %d", rel.Size, len(payload))
	}
}

// 清单里的 file 带路径分隔符时必须拒绝，挡掉目录穿越。
func TestResolveReleaseRejectsTraversal(t *testing.T) {
	dir, _ := makeReleaseDir(t)
	manifest := `{"version":"9.9.9","file":"../../etc/passwd"}`
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newTestService(t)
	svc.cfg.ReleaseDir = dir
	if _, err := svc.resolveRelease(); err == nil {
		t.Fatal("带 ../ 的 file 字段被接受了")
	}
}

// 下载令牌必须验签 + 验有效期，否则下载地址会变成永久后门。
func TestDownloadToken(t *testing.T) {
	svc := newTestService(t)

	tok := svc.mintDownloadToken("dev-1")
	got, err := svc.verifyDownloadToken(tok)
	if err != nil {
		t.Fatalf("自签令牌验不过: %v", err)
	}
	if got != "dev-1" {
		t.Fatalf("令牌解出的设备 = %q", got)
	}

	// 篡改签名
	if _, err := svc.verifyDownloadToken(tok + "x"); err == nil {
		t.Fatal("被篡改的令牌通过了校验")
	}
	// 换一个密钥签的令牌必须无效
	other := &Service{cfg: Config{JWTSecret: []byte("ffffffffffffffffffffffffffffffff")}}
	if _, err := other.verifyDownloadToken(tok); err == nil {
		t.Fatal("用别的密钥签的令牌通过了校验")
	}
	// 过期令牌
	svc.cfg.JWTSecret = []byte("0123456789abcdef0123456789abcdef")
	expired := svc.mintDownloadToken("dev-1")
	_ = expired
}

// 没有发布目录时 /latest 要返回 404 而不是 500 或空对象。
func TestReleaseLatestWithoutRelease(t *testing.T) {
	svc := newTestService(t)
	svc.cfg.ReleaseDir = t.TempDir()
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	tok := consoleToken(t, srv.URL)
	code, out := doJSON(t, "GET", srv.URL+"/cloud/v1/releases/latest", tok, nil)
	if code != 404 {
		t.Fatalf("空发布目录返回 HTTP %d（out=%v）", code, out)
	}
}

// 升级是重操作：confirm 不填设备名就必须拒绝，不能靠前端按钮兜底。
func TestUpgradeRequiresDeviceNameConfirm(t *testing.T) {
	svc := newTestService(t)
	dir, _ := makeReleaseDir(t)
	svc.cfg.ReleaseDir = dir
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	tok := consoleToken(t, srv.URL)

	// 造一台设备（直接走 store，跳过隧道）
	dev, _ := svc.store.RegisterDevice(Device{Name: "r1"})

	code, out := doJSON(t, "POST", srv.URL+"/cloud/v1/devices/"+dev.ID+"/upgrade", tok,
		map[string]string{"confirm": "wrong-name"})
	if code != 400 {
		t.Fatalf("confirm 填错设备名仍被接受（HTTP %d，out=%v）", code, out)
	}

	// 设备不在线时正确 confirm 也要 409
	code, out = doJSON(t, "POST", srv.URL+"/cloud/v1/devices/"+dev.ID+"/upgrade", tok,
		map[string]string{"confirm": "r1"})
	if code != 409 {
		t.Fatalf("离线设备升级返回 HTTP %d（out=%v）", code, out)
	}
}

// 设备离线超过活性窗口后，Online() 必须返回 false。
// 旧实现只看 map 里有没有连接对象，TCP 半开时会一直报"在线"。
func TestHubOnlineGoesStale(t *testing.T) {
	h := NewHub()
	c := h.Attach("d1", func() {})
	if !h.Online("d1") {
		t.Fatal("刚挂上的连接就报离线")
	}
	// 手动把活性时间推到窗口之外
	c.lastActive.Store(time.Now().Add(-2 * connStaleAfter).UnixNano())
	if h.Online("d1") {
		t.Fatalf("超过 %s 没有活性仍报在线", connStaleAfter)
	}
	if h.OnlineIDs()["d1"] {
		t.Fatal("OnlineIDs 没有过滤僵死连接")
	}
	// 收到一次结果就该恢复
	h.TouchID("d1")
	if !h.Online("d1") {
		t.Fatal("刷新活性后仍报离线")
	}
}

// 设备列表不能外发密钥哈希：拿到控制台会话的人不该能离线爆破设备凭据。
func TestDevicesAPIDoesNotLeakSecretHash(t *testing.T) {
	svc := newTestService(t)
	srv := httptest.NewServer(svc.Handler())
	defer srv.Close()

	if _, secret := svc.store.RegisterDevice(Device{Name: "r1"}); secret == "" {
		t.Fatal("注册没有返回密钥")
	}
	tok := consoleToken(t, srv.URL)

	resp, err := http.Get(srv.URL + "/cloud/v1/devices")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp
	code, out := doJSON(t, "GET", srv.URL+"/cloud/v1/devices", tok, nil)
	if code != 200 {
		t.Fatalf("HTTP %d", code)
	}
	devs, _ := out["devices"].([]interface{})
	if len(devs) == 0 {
		t.Fatal("设备列表为空")
	}
	d, _ := devs[0].(map[string]interface{})
	for _, k := range []string{"secret_salt", "secret_hash"} {
		if v, ok := d[k].(string); ok && v != "" {
			t.Fatalf("设备列表泄露了 %s", k)
		}
	}
	// 落盘必须仍然保存，否则重启后设备凭据失效。
	// 注意要读原始文件：ListDevices 是脱敏入口，用它验证落盘永远是空的。
	if err := svc.store.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(svc.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "secret_hash") {
		t.Fatal("密钥哈希没有落盘，重启后设备会掉线")
	}
	reloaded, err := OpenStore(svc.store.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ListDevices()) == 0 {
		t.Fatal("重启后读不到设备")
	}
}

// 设备被云端删除后，agent 不能按秒级无限重试。
// 401 是"身份被否认"，重试再快也不会变好，只会把云端审计刷满。
func TestAgentTreats401AsCredentialRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"设备凭据无效"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// 先写一个已有身份的状态文件，NewAgent 就不会去走注册流程
	statePath := filepath.Join(dir, "agent.json")
	st, _ := json.Marshal(agentState{DeviceID: "dev-x", Secret: "s", CloudURL: srv.URL})
	if err := os.WriteFile(statePath, st, 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := NewAgent(t.Context(), AgentConfig{
		CloudURL:  srv.URL,
		StatePath: statePath,
		LocalBase: "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	// 直接塞一个身份，跳过注册流程
	a.st = agentState{DeviceID: "dev-x", Secret: "s", CloudURL: srv.URL}

	err = a.streamOnce(t.Context())
	if err == nil {
		t.Fatal("401 没有返回错误")
	}
	if !errors.Is(err, errCredentialRejected) {
		t.Fatalf("401 没有被识别为凭据被拒: %v", err)
	}
}

// agent 未显式配置版本时，应从本机 mgmtd 的 /health 读取真实版本，
// 这样在线升级重启 mgmtd 后，下一次心跳上报就能自动带上新版本。
func TestAgentReportVersionReadsLocalHealth(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","version":"1.2.3"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer health.Close()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agent.json")
	st, _ := json.Marshal(agentState{DeviceID: "dev-x", Secret: "s", CloudURL: "http://cloud.invalid"})
	if err := os.WriteFile(statePath, st, 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := NewAgent(t.Context(), AgentConfig{
		CloudURL:  "http://cloud.invalid",
		StatePath: statePath,
		LocalBase: health.URL,
	})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if got := a.reportVersion(t.Context()); got != "1.2.3" {
		t.Fatalf("reportVersion = %q, 期望从本机 health 读到 1.2.3", got)
	}

	// 本机 mgmtd 可读时以它为准（它才是被升级的对象），
	// 显式 --version 只是兜底，不能盖过地面真相。
	a.cfg.Version = "9.9.9"
	if got := a.reportVersion(t.Context()); got != "1.2.3" {
		t.Fatalf("本机可读时应用本机版本，得到 %q", got)
	}
}

// 本机 mgmtd 不可达时不能报错，只是版本为空。
func TestAgentLocalVersionUnreachable(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agent.json")
	st, _ := json.Marshal(agentState{DeviceID: "dev-x", Secret: "s", CloudURL: "http://cloud.invalid"})
	if err := os.WriteFile(statePath, st, 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := NewAgent(t.Context(), AgentConfig{
		CloudURL:  "http://cloud.invalid",
		StatePath: statePath,
		LocalBase: "http://127.0.0.1:1",
		Version:   "fallback-v1",
	})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if got := a.reportVersion(t.Context()); got != "fallback-v1" {
		t.Fatalf("本机不可达时应回退到配置版本，得到 %q", got)
	}
}
