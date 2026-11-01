package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/appid"
)

// 应用特征库（appid）管理接口。
//
// 这是"DPI 到底认不认得出微信和抖音"的那一层：底层 DPI 只到协议层
// （TLS / QUIC / HTTP），把域名映射到具体应用靠这里。

// maxAppidUpload 限制导入体积：一份完整域名库解压后几十 MB 已经很多了。
const maxAppidUpload = 64 << 20

// zip 安全上限，防 zip bomb
const (
	maxZipEntries   = 2000     // 单 zip 最大条目数
	maxZipEntrySize = 16 << 20 // 单条目最大 16 MB（解压后）
	maxZipDepth     = 3        // 目录深度上限（防 ../ 链）
)

// 校验 zip entry 名：禁止路径穿越、Windows 分隔符、绝对路径、过深目录
func safeZipEntryName(name string) error {
	if name == "" {
		return errors.New("empty entry name")
	}
	// 统一分隔符，防 \ 绕过
	name = filepath.ToSlash(name)
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute path not allowed: %s", name)
	}
	// 禁止 .. 穿越（path.Clean 会归一化，但我们要显式拦截并报错）
	if strings.Contains(name, "..") {
		return fmt.Errorf("path traversal not allowed: %s", name)
	}
	// 禁止隐藏文件/目录（以 . 开头的段）
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return fmt.Errorf("hidden segment not allowed: %s", name)
		}
	}
	// 目录深度上限
	depth := strings.Count(strings.Trim(name, "/"), "/") + 1
	if depth > maxZipDepth {
		return fmt.Errorf("zip entry depth %d exceeds limit %d: %s", depth, maxZipDepth, name)
	}
	return nil
}

func (s *Server) handleAppidStats(w http.ResponseWriter, r *http.Request) {
	l := appid.Get()
	if l == nil {
		writeJSON(w, 200, map[string]interface{}{
			"loaded": false,
			"path":   appid.Path(),
			"note":   "特征库尚未加载",
		})
		return
	}
	st := l.Stats()
	writeJSON(w, 200, map[string]interface{}{
		"loaded": true,
		"stats":  st,
		"path":   appid.Path(),
		"note": "特征库只做「域名/SNI -> 应用」映射。" +
			"库为空时所有流量都会显示未识别——导入一份域名分类数据即可，不用改代码。",
	})
}

func (s *Server) handleAppidLookup(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, 422, errBody(11900, "host required", "请提供 host 参数，例如 ?host=www.bilibili.com"))
		return
	}
	l := appid.Get()
	if l == nil {
		writeJSON(w, 503, errBody(11901, "library not loaded", "特征库尚未加载"))
		return
	}
	app, ok := l.Lookup(host)
	if !ok {
		writeJSON(w, 200, map[string]interface{}{
			"host": host, "matched": false,
			"normalized": appid.NormalizeHost(host),
		})
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"host": host, "matched": true, "app": app,
		"normalized": appid.NormalizeHost(host),
	})
}

// handleAppidImport 导入特征库。
//
// 支持两种来源：
//  1. 直接上传 zip（内含多个 domain-list 文本文件，文件名即应用名）
//  2. JSON 提交 {files: {"bilibili": "内容..."}, version: "..."}
//
// 导入是"构建新库再替换"，失败时旧库保持不变——不能让一次失败的升级
// 把线上识别清空。
func (s *Server) handleAppidImport(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	groups := map[string][]byte{}
	version := ""
	source := "upload"

	// multipart 必须优先于 zip 判断（multipart/form-data 也会被 HasPrefix("application/zip") 命中）
	if strings.HasPrefix(ct, "multipart/form-data") {
		// 解析 multipart，取每个文件 part 的内容，文件名作 key
		mr, err := r.MultipartReader()
		if err != nil {
			writeJSON(w, 400, errBody(11910, "multipart parse failed", err.Error()))
			return
		}
		entryCount := 0
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				writeJSON(w, 400, errBody(11910, "multipart read failed", err.Error()))
				return
			}
			if part.FileName() == "" {
				// 非文件字段忽略（或可选读 version 字段）
				continue
			}
			// part.FileName() 只返回 basename，需从 Content-Disposition 取原始 filename 以防路径穿越
			rawFilename := part.FileName()
			if cd := part.Header.Get("Content-Disposition"); cd != "" {
				// 解析 filename="..."，支持 RFC 5987 编码（filename*=utf-8''...）
				// 简化：取最后一个 filename= 后的值，去引号
				if idx := strings.LastIndex(cd, "filename="); idx >= 0 {
					val := cd[idx+len("filename="):]
					val = strings.TrimSpace(val)
					if strings.HasPrefix(val, "\"") {
						end := strings.IndexByte(val[1:], '"')
						if end >= 0 {
							val = val[1 : end+1]
						}
					}
					val = strings.Trim(val, "\"")
					if val != "" {
						rawFilename = val
					}
				}
			}
			entryCount++
			if entryCount > maxZipEntries {
				writeJSON(w, 422, errBody(11907, "too many entries", fmt.Sprintf("文件数 %d 超过上限 %d", entryCount, maxZipEntries)))
				return
			}
			if err := safeZipEntryName(rawFilename); err != nil {
				writeJSON(w, 422, errBody(11908, "unsafe entry name", err.Error()))
				return
			}
			name := path.Base(rawFilename)
			if strings.HasPrefix(name, ".") {
				continue
			}
			b, err := io.ReadAll(io.LimitReader(part, maxZipEntrySize+1))
			if err != nil {
				continue
			}
			if int64(len(b)) > maxZipEntrySize {
				writeJSON(w, 422, errBody(11909, "entry too large", fmt.Sprintf("文件 %s 超过 %d MB", name, maxZipEntrySize>>20)))
				return
			}
			groups[name] = b
		}
		if len(groups) == 0 {
			writeJSON(w, 422, errBody(11905, "no files", "multipart 中未找到有效文件 part"))
			return
		}
		source = "multipart"
	} else if strings.HasPrefix(ct, "application/zip") {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxAppidUpload))
		if err != nil {
			writeJSON(w, 400, errBody(11902, "read body failed", err.Error()))
			return
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			writeJSON(w, 422, errBody(11903, "invalid zip", err.Error()))
			return
		}
		if len(zr.File) > maxZipEntries {
			writeJSON(w, 422, errBody(11907, "too many entries", fmt.Sprintf("zip 条目数 %d 超过上限 %d", len(zr.File), maxZipEntries)))
			return
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			// 安全校验 entry 名
			if err := safeZipEntryName(f.Name); err != nil {
				writeJSON(w, 422, errBody(11908, "unsafe entry name", err.Error()))
				return
			}
			name := path.Base(f.Name)
			if strings.HasPrefix(name, ".") || strings.Contains(f.Name, "__MACOSX") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			// 单条目大小上限（解压流读取时限制）
			b, err := io.ReadAll(io.LimitReader(rc, maxZipEntrySize+1))
			rc.Close()
			if err != nil {
				continue
			}
			if int64(len(b)) > maxZipEntrySize {
				writeJSON(w, 422, errBody(11909, "entry too large", fmt.Sprintf("zip 条目 %s 解压后超过 %d MB", name, maxZipEntrySize>>20)))
				return
			}
			groups[name] = b
		}
		source = "zip"
	} else {
		var req struct {
			Files   map[string]string `json:"files"`
			Version string            `json:"version"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxAppidUpload)).Decode(&req); err != nil {
			writeJSON(w, 400, errBody(11904, "json decode fail", err.Error()))
			return
		}
		for k, v := range req.Files {
			if err := safeZipEntryName(k); err != nil {
				writeJSON(w, 422, errBody(11908, "unsafe key name", err.Error()))
				return
			}
			groups[k] = []byte(v)
		}
		version = req.Version
		source = "json"
	}

	if len(groups) == 0 {
		writeJSON(w, 422, errBody(11905, "no files", "没有解析到任何数据文件（每个文件一组，文件名即应用名）"))
		return
	}

	meta := map[string]appid.App{}
	for name := range groups {
		meta[name] = appid.App{ID: name, Name: name, Category: appid.CategoryForGroup(name)}
	}

	stats, rej, err := appid.BuildAndInstall(groups, meta, version, source, appid.Path())
	if err != nil {
		// 构建失败时全局库仍是旧的（BuildAndInstall 只在成功后 Set）
		writeJSON(w, 500, errBody(11906, "import failed", err.Error()))
		return
	}
	log.Printf("appid imported: %d apps, %d domains, version %s", stats.Apps, stats.Domains, stats.Version)
	writeJSON(w, 200, map[string]interface{}{
		"imported": true, "stats": stats, "rejected": rej,
		"note": "导入后立即生效，无需重启。库为空或域名太少时这里会显示对应警告。",
	})
}

// handleAppidCategories 列出分类与每类的应用（界面左侧树用）。
func (s *Server) handleAppidCategories(w http.ResponseWriter, r *http.Request) {
	l := appid.Get()
	if l == nil {
		writeJSON(w, 200, map[string]interface{}{"categories": []any{}})
		return
	}
	st := l.Stats()
	type catView struct {
		Category appid.Category `json:"category"`
		Count    int            `json:"count"`
	}
	list := make([]catView, 0, len(st.ByCategory))
	for c, n := range st.ByCategory {
		list = append(list, catView{Category: c, Count: n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return string(list[i].Category) < string(list[j].Category)
	})

	// 分类下钻：按分类返回应用（最多 200 个，避免大分类把响应撑爆）
	q := r.URL.Query().Get("category")
	apps := []appid.App{}
	if q != "" {
		for _, a := range l.Apps {
			// 聚合组（category-media-cn、cn、tld-cn 之类）不是应用，
			// 混在列表里会让用户以为"识别出个 category-media-cn"
			if appid.IsAggregateGroup(a.ID) {
				continue
			}
			if string(a.Category) == q {
				apps = append(apps, a)
			}
		}
		sort.Slice(apps, func(i, j int) bool { return apps[i].Domains > apps[j].Domains })
		if len(apps) > 200 {
			apps = apps[:200]
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"categories": list,
		"apps":       apps,
		"version":    st.Version,
	})
}

// lookupAppForHost 是给 DPI 识别路径用的内部辅助：识别出的主机名 -> 应用。
func lookupAppForHost(host string) (appid.App, bool) {
	return appid.Identify(host)
}
