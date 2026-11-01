package cloud

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AgentConfig 是设备侧（被管理的路由终端）的配置。
type AgentConfig struct {
	CloudURL    string // 例如 https://1.1ai.best
	EnrollToken string // 首次注册用，一次性
	StatePath   string // 保存 device_id/secret 的文件（0600）
	LocalBase   string // 本机 mgmtd，例如 http://127.0.0.1:8080
	LocalUser   string
	LocalPass   string
	Name        string
	Model       string
	Version     string
	InsecureTLS bool // 自签证书时用（生产不建议）
}

type agentState struct {
	DeviceID string `json:"device_id"`
	Secret   string `json:"secret"`
	CloudURL string `json:"cloud_url"`
}

// Agent 是设备侧常驻进程：注册一次，之后长连接 + 断线重连。
type Agent struct {
	cfg AgentConfig
	hc  *http.Client
	st  agentState

	mu     sync.Mutex
	jwt    string
	jwtExp time.Time
}

// NewAgent 载入（或注册）设备身份。
func NewAgent(ctx context.Context, cfg AgentConfig) (*Agent, error) {
	if cfg.CloudURL == "" || cfg.LocalBase == "" {
		return nil, errors.New("CloudURL 与 LocalBase 必填")
	}
	cfg.CloudURL = strings.TrimRight(cfg.CloudURL, "/")
	if cfg.StatePath == "" {
		cfg.StatePath = "/etc/router-re/cloud-agent.json"
	}
	a := &Agent{cfg: cfg, hc: &http.Client{Timeout: 30 * time.Second}}
	if err := a.loadState(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if a.st.Secret == "" || a.st.DeviceID == "" {
		if cfg.EnrollToken == "" {
			return nil, fmt.Errorf("未注册且没有提供注册令牌（%s 不存在）", cfg.StatePath)
		}
		if err := a.enroll(ctx); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *Agent) loadState() error {
	b, err := os.ReadFile(a.cfg.StatePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &a.st)
}

func (a *Agent) saveState() error {
	if err := os.MkdirAll(filepath.Dir(a.cfg.StatePath), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(a.st, "", "  ")
	tmp := a.cfg.StatePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.cfg.StatePath)
}

// reportVersion 返回要上报给云端的版本号。
//
// 优先问本机 mgmtd 的 /api/v1/health，因为"被升级的是本机 mgmtd"，
// 它的版本才是地面真相；手工配的 --version 会随着一次次在线升级而变旧，
// 只能当兜底。
//
// 每次心跳都现读（不缓存）：在线升级重启的是 mgmtd 而不是 agent 进程，
// 若在启动时读一次并缓存，升级后 agent 会一直报旧版本，界面上永远看不到
// 升级成功。现读的代价是一次本地 HTTP，相对心跳间隔可以忽略。
func (a *Agent) reportVersion(ctx context.Context) string {
	if v := a.localVersion(ctx); v != "" {
		return v
	}
	return a.cfg.Version
}

// localVersion 从本机 mgmtd 的 /api/v1/health 读取版本号。读不到就返回空串。
func (a *Agent) localVersion(ctx context.Context) string {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, "GET", a.cfg.LocalBase+"/api/v1/health", nil)
	if err != nil {
		return ""
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return ""
	}
	return body.Version
}

func (a *Agent) enroll(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"token": a.cfg.EnrollToken, "name": a.cfg.Name,
		"model": a.cfg.Model, "version": a.reportVersion(ctx),
	})
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.CloudURL+"/cloud/v1/agent/enroll", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.hc.Do(req)
	if err != nil {
		return fmt.Errorf("注册请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != 200 {
		return fmt.Errorf("注册失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		DeviceID string `json:"device_id"`
		Secret   string `json:"secret"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	a.st = agentState{DeviceID: out.DeviceID, Secret: out.Secret, CloudURL: a.cfg.CloudURL}
	if err := a.saveState(); err != nil {
		return fmt.Errorf("保存设备身份失败: %w", err)
	}
	log.Printf("cloud-agent: 注册成功 device_id=%s", out.DeviceID)
	return nil
}

// errCredentialRejected 表示云端拒绝了这个设备身份（设备被删除或密钥作废）。
var errCredentialRejected = errors.New("设备凭据被云端拒绝")

// credentialRetryDelay 是身份被拒后的重试间隔。
// 用小时级而不是秒级：被删的设备重试再快也连不上，而 10 分钟一次足以在
// 运维重新注册后自动恢复。
const credentialRetryDelay = 10 * time.Minute

// Run 一直跑到 ctx 结束；断线自动重连（退避 + 抖动）。
func (a *Agent) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		err := a.streamOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errCredentialRejected) {
			log.Printf("cloud-agent: %v；设备可能已被删除，需要重新注册。"+
				"（%s 后重试，不会退出进程）", err, credentialRetryDelay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(credentialRetryDelay):
			}
			continue
		}
		// 连上超过 30 秒说明网络是通的，退避重置，避免正常断线后越等越久。
		if time.Since(start) > 30*time.Second {
			backoff = time.Second
		}
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		log.Printf("cloud-agent: 隧道断开: %v；%s 后重连", err, (backoff + jitter).Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff + jitter):
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}

func (a *Agent) streamOnce(ctx context.Context) error {
	// 流是长连接：不能设整体 Timeout，只能靠 ctx 和读超时。
	req, err := http.NewRequestWithContext(ctx, "GET", a.cfg.CloudURL+"/cloud/v1/agent/stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Device-ID", a.st.DeviceID)
	req.Header.Set("X-Device-Secret", a.st.Secret)
	req.Header.Set("Accept", "application/x-ndjson")
	hc := &http.Client{Transport: &http.Transport{
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       0,
		DisableCompression:    true,
	}}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusUnauthorized {
			// 401 不是"网络抖了一下"，是云端明确否认这个身份：
			// 设备被删除、或状态文件里的 secret 已经作废。继续按秒级重试
			// 只会永远撞同一堵墙，还顺带把云端审计刷满。
			return fmt.Errorf("%w: %s", errCredentialRejected, strings.TrimSpace(string(b)))
		}
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	log.Printf("cloud-agent: 隧道已建立 device_id=%s", a.st.DeviceID)
	if err := a.report(context.Background()); err != nil {
		log.Printf("cloud-agent: 上报元数据失败: %v", err)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var f Frame
		if err := json.Unmarshal(line, &f); err != nil {
			log.Printf("cloud-agent: 帧解析失败: %v", err)
			continue
		}
		switch f.Type {
		case "ping":
			// 保活：顺带刷新 LastSeen。
			_ = a.report(context.Background())
		case "hello":
			// 云端握手完成
		case "exec":
			go a.handleExec(f)
		}
	}
	return sc.Err()
}

func (a *Agent) report(ctx context.Context) error {
	up := int64(0)
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			var f float64
			_, _ = fmt.Sscanf(fields[0], "%f", &f)
			up = int64(f)
		}
	}
	body, _ := json.Marshal(map[string]interface{}{
		"name": a.cfg.Name, "model": a.cfg.Model, "version": a.reportVersion(ctx), "uptime_sec": up,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.CloudURL+"/cloud/v1/agent/report", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", a.st.DeviceID)
	req.Header.Set("X-Device-Secret", a.st.Secret)
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<14))
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// handleExec 在设备本地执行一条白名单请求，并把结果回传云端。
func (a *Agent) handleExec(f Frame) {
	res := Frame{Type: "result", ID: f.ID}
	// 设备侧再校验一次：云端被攻破时，这里把爆炸半径限制在只读接口。
	// 代理模式（handleDeviceUI）跳过策略检查，因为代理需要转发所有 API。
	if !f.Proxy && !PathAllowed(f.Method, f.Path) {
		res.Status, res.Error = 403, "被设备侧策略拒绝: "+f.Method+" "+f.Path
		a.postResult(res)
		return
	}
	body := io.Reader(nil)
	if f.Body != "" {
		if raw, err := base64.StdEncoding.DecodeString(f.Body); err == nil {
			body = bytes.NewReader(raw)
		}
	}
	status, data, err := a.localCall(f.Method, f.Path, body)
	if err != nil {
		res.Error = err.Error()
		ans, _ := json.Marshal(map[string]string{"error": err.Error()})
		res.Status, res.Data = 502, base64.StdEncoding.EncodeToString(ans)
		a.postResult(res)
		return
	}
	res.Status = status
	res.Data = base64.StdEncoding.EncodeToString(data)
	a.postResult(res)
}

func (a *Agent) postResult(f Frame) {
	b, _ := json.Marshal(f)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.CloudURL+"/cloud/v1/agent/result", bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", a.st.DeviceID)
	req.Header.Set("X-Device-Secret", a.st.Secret)
	resp, err := a.hc.Do(req)
	if err != nil {
		log.Printf("cloud-agent: 回传结果失败: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
}

// localToken 拿本机 mgmtd 的会话令牌（15 分钟 TTL，提前 1 分钟刷新）。
func (a *Agent) localToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jwt != "" && time.Now().Before(a.jwtExp) {
		return a.jwt, nil
	}
	b, _ := json.Marshal(map[string]string{"username": a.cfg.LocalUser, "password": a.cfg.LocalPass})
	req, err := http.NewRequestWithContext(ctx, "POST", a.cfg.LocalBase+"/api/v1/auth/login", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("本机登录失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Token == "" {
		return "", errors.New("本机登录未返回 token")
	}
	a.jwt = out.Token
	a.jwtExp = time.Now().Add(14 * time.Minute)
	return a.jwt, nil
}

func (a *Agent) localCall(method, path string, body io.Reader) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := a.localToken(ctx)
		if err != nil {
			return 0, nil, err
		}
		var rd io.Reader
		if body != nil {
			// body 是 io.Reader，重试时要重新构造，这里只在第一次读，失败重试就放弃 body。
			if b, ok := body.(*bytes.Reader); ok {
				b.Seek(0, io.SeekStart)
				rd = b
			} else {
				rd = body
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, a.cfg.LocalBase+path, rd)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if rd != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := a.hc.Do(req)
		if err != nil {
			return 0, nil, err
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode == 401 && attempt == 0 {
			// 令牌失效（本机改过密码/重启）：重新登录再来一次。
			a.mu.Lock()
			a.jwt = ""
			a.mu.Unlock()
			continue
		}
		return resp.StatusCode, data, nil
	}
	return 0, nil, errors.New("本机接口拒绝：凭据无效")
}
