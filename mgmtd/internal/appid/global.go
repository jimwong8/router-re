package appid

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// 全局库：让识别路径（每包/每流）不必层层传参。
//
// 用 atomic.Pointer 整体替换：热更新时先构建新库，再一次性换指针，
// 读者永远看到完整的库，不会看到"加载到一半"的中间态。

var current atomic.Pointer[Library]

// Set 替换全局库。
func Set(l *Library) { current.Store(l) }

// Get 返回当前全局库（可能为 nil，调用方要判空）。
func Get() *Library { return current.Load() }

// Identify 用全局库识别 host。库未加载时返回未命中。
func Identify(host string) (App, bool) {
	l := current.Load()
	if l == nil {
		return App{}, false
	}
	return l.Lookup(host)
}

// Path 是特征库快照的默认位置。
func Path() string {
	if v := os.Getenv("ROUTER_RE_APPID"); v != "" {
		return v
	}
	return "/var/lib/router-re/appid.json"
}

// LoadFrom 从快照文件装载并设为全局库。
//
// 文件不存在不算错误——首次部署本来就没有库，此时所有流量都识别为
// "未识别"，界面上的 stats 会给出"库为空"的原因，而不是一个空的表格。
func LoadFrom(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			Set(New())
			return 0, nil
		}
		return 0, err
	}
	l, err := LoadSnapshot(b)
	if err != nil {
		// 损坏的库不该让管理面起不来：退回空库并让调用方记日志
		Set(New())
		return 0, err
	}
	Set(l)
	return l.Stats().Apps, nil
}

// SaveTo 把全局库原子写入 path。
func SaveTo(path string) error {
	l := current.Load()
	if l == nil {
		return nil
	}
	snap, err := l.Snapshot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, snap, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// BuildAndInstall 从 domain-list 数据构建新库并设为全局。
//
// 这是"升级特征库"的入口：支持把多个文件的内容直接喂进来（来自上传、
// 拉取的压缩包或本地目录），构建完成后写快照并握手替换。
func BuildAndInstall(groups map[string][]byte, meta map[string]App, version, source, savePath string) (Stats, Rejected, error) {
	l := New()
	rej, err := l.ImportDomainList(groups, meta)
	if err != nil {
		return Stats{}, rej, err
	}
	l.Version = version
	if l.Version == "" {
		l.Version = time.Now().Format("20060102")
	}
	l.UpdatedAt = time.Now()
	l.Source = source

	Set(l)
	if savePath != "" {
		if err := SaveTo(savePath); err != nil {
			return l.Stats(), rej, err
		}
	}
	return l.Stats(), rej, nil
}
