package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Ring 的持久化。
//
// 为什么必须落盘：Ring 是进程内存里的环形缓冲，进程一重启（升级、崩溃、
// 手工重启）历史就全清零。于是"360 分钟"视图在重启后永远只有几分钟的数据，
// 用户看到的是一条只有两三个点的线——看起来像图表坏了，其实是没数据。
//
// 这里做成"启动恢复 + 定期快照"：文件很小（720 个样本约 100 KB），
// 每 5 分钟写一次的代价可以忽略。

// Snapshot 返回按时间顺序排列的全部样本（拷贝，调用方可安全持有）。
func (r *Ring) Snapshot() []Sample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Sample, 0, len(r.buf))
	if len(r.buf) < r.size {
		return append(out, r.buf...)
	}
	// 满了之后 buf 是环形的，pos 指向最旧的一条
	out = append(out, r.buf[r.pos:]...)
	out = append(out, r.buf[:r.pos]...)
	return out
}

// Load 用已有样本初始化。会按 TS 排序并只保留最后 size 条。
func (r *Ring) Load(samples []Sample) {
	if len(samples) == 0 {
		return
	}
	s := make([]Sample, len(samples))
	copy(s, samples)
	sort.SliceStable(s, func(i, j int) bool { return s[i].TS < s[j].TS })

	// 去掉明显重复的时间戳：落盘期间可能有重叠
	dedup := s[:0]
	var last int64 = -1
	for _, x := range s {
		if x.TS == last {
			continue
		}
		dedup = append(dedup, x)
		last = x.TS
	}
	s = dedup

	if len(s) > r.size {
		s = s[len(s)-r.size:]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf[:0], s...)
	r.pos = 0
}

// SaveFile 把当前样本原子写入 path（先写临时文件再 rename，
// 避免进程在写一半时被杀导致文件损坏）。
func (r *Ring) SaveFile(path string) error {
	snap := r.Snapshot()
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// LoadFile 从 path 恢复历史。文件不存在时返回空 ring 且不报错——
// 首次启动本来就没有文件，这不是异常。
func LoadFile(path string, size int) (*Ring, int, error) {
	r := NewRing(size)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, 0, nil
		}
		return r, 0, err
	}
	var samples []Sample
	if err := json.Unmarshal(b, &samples); err != nil {
		// 文件损坏不该让服务起不来：丢弃历史继续跑
		return r, 0, fmt.Errorf("历史文件损坏，已忽略: %w", err)
	}
	r.Load(samples)
	return r, len(r.Snapshot()), nil
}
