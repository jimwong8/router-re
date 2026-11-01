package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 在线升级的云端一半：发布包分发 + 升级编排。
//
// 为什么把编排放在云端而不是浏览器：
//   - 浏览器不该拿到"给设备的下载地址"这种凭据，否则任何人打开 devtools 都能把
//     一个任意 URL 塞给设备的升级接口
//   - 校验和必须由云端从真实文件算出来，不能由前端填。前端填的 checksum 等于没有
//   - 下载地址用短时签名令牌，设备拿到的是 10 分钟内有效的一次性链接，
//     即使日志泄露也不会变成长期可用的下载入口

// Release 是一个可下发的版本。
type Release struct {
	Version   string `json:"version"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Notes     string `json:"notes,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

// releaseCache 缓存一次校验结果，避免每次 /latest 都把 20MB 读一遍算哈希。
type releaseCache struct {
	mu   sync.Mutex
	key  string // path|size|mtime
	rel  Release
	err  error
	when time.Time
}

func (s *Service) releaseDir() string {
	if d := strings.TrimSpace(s.cfg.ReleaseDir); d != "" {
		return d
	}
	return "/var/lib/router-cloud/releases"
}

// resolveRelease 找出当前要下发的版本。
//
// 优先读 latest.json（运维显式指定，可控）；没有就退化成"目录里最新的 mgmtd* 文件"，
// 这样临时拷一个文件上去也能用，不需要额外写清单。
func (s *Service) resolveRelease() (Release, error) {
	dir := s.releaseDir()

	// 清单优先
	manifest := filepath.Join(dir, "latest.json")
	if b, err := os.ReadFile(manifest); err == nil {
		var rel Release
		if err := json.Unmarshal(b, &rel); err != nil {
			return Release{}, fmt.Errorf("latest.json 解析失败: %w", err)
		}
		if rel.File == "" {
			rel.File = "mgmtd"
		}
		// 清单里的 file 只允许是目录内的普通文件名，挡掉 ../ 穿越
		if strings.ContainsAny(rel.File, `/\`) {
			return Release{}, errors.New("latest.json 的 file 不能带路径分隔符")
		}
		return s.fillRelease(dir, rel)
	}

	// 退化路径：扫目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Release{}, fmt.Errorf("发布目录不可用（%s）: %w", dir, err)
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "mgmtd") {
			continue
		}
		// 排除清单、校验和、备份、临时文件
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".sha256") ||
			strings.HasSuffix(name, ".bak") || strings.HasSuffix(name, ".new") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestMod) {
			bestMod = info.ModTime()
			best = name
		}
	}
	if best == "" {
		return Release{}, fmt.Errorf("发布目录 %s 里没有找到 mgmtd 发布包", dir)
	}
	return s.fillRelease(dir, Release{File: best, Version: best})
}

// fillRelease 补齐 size / sha256 / version，并做一次缓存。
func (s *Service) fillRelease(dir string, rel Release) (Release, error) {
	path := filepath.Join(dir, rel.File)
	info, err := os.Stat(path)
	if err != nil {
		return Release{}, fmt.Errorf("发布包不存在: %w", err)
	}
	if info.IsDir() {
		return Release{}, errors.New("发布包路径是个目录")
	}
	key := fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())

	s.rc.mu.Lock()
	defer s.rc.mu.Unlock()
	if s.rc.key == key && s.rc.err == nil && s.rc.rel.File != "" {
		out := s.rc.rel
		// 清单里的 version/notes 以最新读取的为准
		if rel.Version != "" {
			out.Version = rel.Version
		}
		if rel.Notes != "" {
			out.Notes = rel.Notes
		}
		return out, nil
	}

	sum, err := sha256File(path)
	if err != nil {
		// 注意：这里不能写 s.rc = releaseCache{...}。rc 里含 mutex，
		// 整体赋值会把正在持有的锁换成一把全新的零值锁，defer 的 Unlock
		// 就会去解一把没上锁的互斥量，直接 fatal: unlock of unlocked mutex。
		s.rc.key, s.rc.err, s.rc.rel = key, err, Release{}
		return Release{}, err
	}
	if rel.SHA256 != "" && !strings.EqualFold(rel.SHA256, sum) {
		return Release{}, fmt.Errorf("latest.json 里的 sha256 与文件实际内容不符（清单 %s / 实际 %s），拒绝下发",
			rel.SHA256, sum)
	}
	rel.SHA256 = sum
	rel.Size = info.Size()
	rel.UpdatedAt = info.ModTime().Unix()
	if rel.Version == "" {
		rel.Version = strings.TrimPrefix(rel.File, "mgmtd-")
		if rel.Version == rel.File {
			rel.Version = rel.File
		}
	}
	s.rc.key, s.rc.err, s.rc.rel = key, nil, rel
	return rel, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------------------------------------------------------------- 下载令牌

// downloadTokenTTL 是一次下载链接的有效期。
// 够设备排队下载，又短到即使泄露也很快失效。
const downloadTokenTTL = 10 * time.Minute

// mintDownloadToken 生成 "base64(payload).base64(hmac)" 形式的令牌。
func (s *Service) mintDownloadToken(deviceID string) string {
	exp := time.Now().Add(downloadTokenTTL).Unix()
	payload := deviceID + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, s.cfg.JWTSecret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyDownloadToken 校验签名与有效期，返回设备 ID。
func (s *Service) verifyDownloadToken(tok string) (string, error) {
	dot := strings.LastIndex(tok, ".")
	if dot <= 0 {
		return "", errors.New("令牌格式错误")
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(tok[:dot])
	if err != nil {
		return "", errors.New("令牌格式错误")
	}
	rawSig, err := base64.RawURLEncoding.DecodeString(tok[dot+1:])
	if err != nil {
		return "", errors.New("令牌格式错误")
	}
	mac := hmac.New(sha256.New, s.cfg.JWTSecret)
	mac.Write(rawPayload)
	if !hmac.Equal(rawSig, mac.Sum(nil)) {
		return "", errors.New("令牌签名无效")
	}
	parts := strings.SplitN(string(rawPayload), "|", 2)
	if len(parts) != 2 {
		return "", errors.New("令牌内容错误")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", errors.New("令牌内容错误")
	}
	if time.Now().Unix() > exp {
		return "", errors.New("下载链接已过期")
	}
	return parts[0], nil
}

// ---------------------------------------------------------------- HTTP 处理

// handleReleaseLatest 返回当前可下发版本（控制台用来显示"有新版本"）。
func (s *Service) handleReleaseLatest(w http.ResponseWriter, r *http.Request) {
	rel, err := s.resolveRelease()
	if err != nil {
		writeErr(w, 404, 20301, "no release", err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"release": rel,
		"dir":     s.releaseDir(),
	})
}

// handleReleaseDownload 下发发布包。用短时签名令牌认证，不需要控制台会话
// （设备侧的 mgmtd 没有控制台凭据，只有下载链接本身）。
func (s *Service) handleReleaseDownload(w http.ResponseWriter, r *http.Request) {
	devID, err := s.verifyDownloadToken(r.URL.Query().Get("t"))
	if err != nil {
		s.store.AddAudit(AuditEntry{Actor: "release:" + clientIP(r), Action: "release.download", Result: "deny", Detail: err.Error()})
		_ = s.store.Save()
		writeErr(w, 401, 20302, "invalid token", err.Error())
		return
	}
	rel, err := s.resolveRelease()
	if err != nil {
		writeErr(w, 404, 20301, "no release", err.Error())
		return
	}
	path := filepath.Join(s.releaseDir(), rel.File)
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 500, 20303, "open release", err.Error())
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeErr(w, 500, 20304, "stat release", err.Error())
		return
	}
	// 内容按文件给，长度由 http.ServeContent 处理（它支持 Range 断点续传）
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Release-Version", rel.Version)
	w.Header().Set("X-Release-SHA256", rel.SHA256)
	w.Header().Set("Content-Disposition", `attachment; filename="`+rel.File+`"`)
	s.store.AddAudit(AuditEntry{Actor: "device:" + devID, Action: "release.download", Result: "ok", Detail: rel.Version})
	_ = s.store.Save()
	http.ServeContent(w, r, rel.File, info.ModTime(), f)
}

// handleDeviceUpgrade 编排一次在线升级：解析版本 → 生成下载令牌 → 下发升级指令。
//
// 全流程在云端完成，前端只负责"点一下 + 填设备名确认"。
func (s *Service) handleDeviceUpgrade(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, ok := s.store.GetDevice(id)
	if !ok {
		writeErr(w, 404, 20201, "device not found", id)
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		writeErr(w, 400, 20001, "bad json", err.Error())
		return
	}
	// 升级会替换二进制并重启服务，属于最重的一次写操作，必须带设备名确认
	if strings.TrimSpace(req.Confirm) != dev.Name {
		writeErr(w, 400, 20204, "confirm required",
			"升级需要在 confirm 里填入设备名「"+dev.Name+"」")
		return
	}
	rel, err := s.resolveRelease()
	if err != nil {
		writeErr(w, 404, 20301, "no release", err.Error())
		return
	}
	if !s.hub.Online(id) {
		writeErr(w, 409, 20205, "device offline", "设备当前没有隧道连接")
		return
	}

	base := strings.TrimRight(s.publicBase(r), "/")
	url := fmt.Sprintf("%s/cloud/v1/releases/download?t=%s", base, s.mintDownloadToken(dev.ID))

	body, _ := json.Marshal(map[string]interface{}{
		"url":      url,
		"checksum": rel.SHA256,
		"confirm":  true,
	})
	res, err := s.hub.Call(id, Frame{
		Type:   "exec",
		Method: "POST",
		Path:   "/api/v1/system/upgrade",
		Body:   base64.StdEncoding.EncodeToString(body),
	}, s.cfg.ExecTimeout)
	if err != nil {
		result := "fail"
		if errors.Is(err, ErrTimeout) {
			result = "timeout"
		}
		s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.upgrade", Result: result,
			Detail: rel.Version + " " + err.Error()})
		_ = s.store.Save()
		code := 504
		if errors.Is(err, ErrOffline) {
			code = 409
		}
		writeErr(w, code, 20206, "upgrade failed", err.Error())
		return
	}
	if res.Status >= 400 {
		s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.upgrade", Result: "fail",
			Detail: rel.Version + " HTTP " + strconv.Itoa(res.Status)})
		_ = s.store.Save()
		writeErr(w, 502, 20305, "device rejected upgrade",
			"设备拒绝升级（HTTP "+strconv.Itoa(res.Status)+"）")
		return
	}
	s.store.AddAudit(AuditEntry{Actor: actor(r), Device: dev.Name, Action: "device.upgrade", Result: "ok",
		Detail: rel.Version + " sha256=" + rel.SHA256[:12]})
	_ = s.store.Save()
	writeJSON(w, 200, map[string]interface{}{
		"ok":      true,
		"version": rel.Version,
		"sha256":  rel.SHA256,
		"size":    rel.Size,
		"status":  res.Status,
	})
}

// publicBase 推断设备可访问的云端根地址。
// 优先用配置的 PublicBase（自建反代时最可靠），否则按请求头推断。
func (s *Service) publicBase(r *http.Request) string {
	if b := strings.TrimSpace(s.cfg.PublicBase); b != "" {
		return b
	}
	scheme := "https"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}
