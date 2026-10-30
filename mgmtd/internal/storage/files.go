package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 文件管理。
//
// 与 iKuai 的 system/files 对应，但路径校验方式不同：
//
// iKuai 的做法是 `cd /etc/disk_user$path` 之后检查 $PWD 是否以
// /etc/disk_user/ 开头。PWD 是 shell 的逻辑路径，跟着符号链接进去时
// 仍然显示 /etc/disk_user/...，所以只要目录里有一个指向 /etc 的软链，
// 就能读写系统文件。
//
// 这里改用 EvalSymlinks 解析到真实路径再判断前缀，并且对"新建目标"
// 校验其父目录的真实路径——两条路径都逃不出去。
const DefaultFileRoot = "/etc/disk_user"

// FileManager 把文件操作限制在一个根目录内。
type FileManager struct {
	Root string
}

// NewFileManager 返回限定在 root 内的管理器。
func NewFileManager(root string) *FileManager {
	if root == "" {
		root = DefaultFileRoot
	}
	return &FileManager{Root: root}
}

// Entry 是目录里的一个条目。
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // 相对于根目录的路径，以 / 开头
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"mod_time"`
}

// ErrForbidden 表示路径逃出了根目录。
var ErrForbidden = errors.New("路径越界：只能访问存储目录内部")

// ErrRootAccess 表示试图把根目录本身当成操作对象。
var ErrRootAccess = errors.New("不能直接操作存储根目录")

// resolve 把外部传入的相对路径解析成根目录内的真实绝对路径。
//
// 三种逃逸都要挡住：../ 穿越、绝对路径、符号链接指向外部。
func (m *FileManager) resolve(rel string, mustExist bool) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("路径包含非法字符")
	}
	// 逐段检查 ..：Clean 会把 ".." 归一成 "/"（结果安全但语义模糊），
	// 明确拒绝才能让越界尝试留下可见的错误，而不是静默当成根目录。
	for _, seg := range strings.Split(strings.TrimPrefix(rel, "/"), "/") {
		if seg == ".." {
			return "", ErrForbidden
		}
	}
	// 统一成以 / 开头的相对形式；绝对路径 "//etc/passwd" 也要被当成相对
	clean := filepath.Clean("/" + strings.TrimPrefix(rel, "/"))
	rootReal, err := filepath.EvalSymlinks(m.Root)
	if err != nil {
		// 根目录不存在时不放行，避免"还没初始化就写入任意位置"
		return "", fmt.Errorf("存储根目录不可用: %w", err)
	}
	full := filepath.Join(rootReal, clean)
	if full == rootReal {
		return full, nil // 调用方决定是否允许（读列表允许，删除不允许）
	}

	checkInside := func(p string) error {
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			return err
		}
		if real != rootReal && !strings.HasPrefix(real, rootReal+string(os.PathSeparator)) {
			return ErrForbidden
		}
		return nil
	}

	if _, err := os.Lstat(full); err == nil {
		if err := checkInside(full); err != nil {
			if errors.Is(err, ErrForbidden) {
				return "", ErrForbidden
			}
			return "", err
		}
		return full, nil
	} else if mustExist {
		return "", fmt.Errorf("路径不存在: %s", clean)
	}

	// 目标不存在（新建）：父目录必须真实存在于根内
	parent := filepath.Dir(full)
	if err := checkInside(parent); err != nil {
		if errors.Is(err, ErrForbidden) {
			return "", ErrForbidden
		}
		return "", fmt.Errorf("父目录不可用: %w", err)
	}
	return full, nil
}

// relPath 把绝对路径还原成对外的相对路径（以 / 开头）。
func (m *FileManager) relPath(abs string) string {
	rootReal, err := filepath.EvalSymlinks(m.Root)
	if err != nil {
		return "/"
	}
	rel, err := filepath.Rel(rootReal, abs)
	if err != nil || rel == "." {
		return "/"
	}
	return "/" + filepath.ToSlash(rel)
}

// List 返回目录内容，目录在前、同类型按名字排序。
func (m *FileManager) List(rel string) ([]Entry, error) {
	full, err := m.resolve(rel, true)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		info, err := it.Info()
		if err != nil {
			continue // 竞态下文件可能已被删除，跳过而不是整体失败
		}
		fullPath := filepath.Join(full, it.Name())
		// 符号链接本身不算目录，避免前端点击后跳到根外
		out = append(out, Entry{
			Name:    it.Name(),
			Path:    m.relPath(fullPath),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// validateName 校验单层文件名。与 iKuai 的 __check_valid_name1/2 一致：
// 不允许 / 和控制字符，额外拒绝以 . 结尾这种在部分文件系统上有歧义的名字。
func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("名字非法")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("名字不能包含路径分隔符")
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("名字包含非法字符")
	}
	for _, r := range name {
		if r < 0x20 {
			return fmt.Errorf("名字包含控制字符")
		}
	}
	if len(name) > 255 {
		return fmt.Errorf("名字过长（最多 255 字节）")
	}
	return nil
}

// Mkdir 在 rel 目录下建子目录。
func (m *FileManager) Mkdir(rel, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	// 父目录必须存在且在根内
	if _, err := m.resolve(rel, true); err != nil {
		return err
	}
	target, err := m.resolve(filepath.Join(rel, name), false)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("同名目录或文件已存在")
	}
	return os.Mkdir(target, 0o755)
}

// CreateFile 建空文件。
func (m *FileManager) CreateFile(rel, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	target, err := m.resolve(filepath.Join(rel, name), false)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("同名目录或文件已存在")
		}
		return err
	}
	return f.Close()
}

// Rename 在同一个目录内改名。
func (m *FileManager) Rename(rel, oldName, newName string) error {
	if err := validateName(oldName); err != nil {
		return err
	}
	if err := validateName(newName); err != nil {
		return err
	}
	src, err := m.resolve(filepath.Join(rel, oldName), true)
	if err != nil {
		return err
	}
	if src == "" {
		return ErrRootAccess
	}
	dst, err := m.resolve(filepath.Join(rel, newName), false)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("目标名字已存在")
	}
	return os.Rename(src, dst)
}

// Move 把条目从 srcDir 移到 dstDir（两端都在根内）。
func (m *FileManager) Move(srcDir, name, dstDir string) error {
	if err := validateName(name); err != nil {
		return err
	}
	src, err := m.resolve(filepath.Join(srcDir, name), true)
	if err != nil {
		return err
	}
	dstDirAbs, err := m.resolve(dstDir, true)
	if err != nil {
		return err
	}
	// 不允许把目录移进自己（会把目录树弄坏）
	if info, err := os.Stat(src); err == nil && info.IsDir() {
		if strings.HasPrefix(dstDirAbs+"/", src+"/") {
			return fmt.Errorf("不能把目录移动到它自身内部")
		}
	}
	dst := filepath.Join(dstDirAbs, name)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("目标目录已有同名条目")
	}
	return os.Rename(src, dst)
}

// Remove 删除文件或目录（目录递归）。根目录本身禁止删除。
func (m *FileManager) Remove(rel string) error {
	full, err := m.resolve(rel, true)
	if err != nil {
		return err
	}
	rootReal, err := filepath.EvalSymlinks(m.Root)
	if err != nil {
		return err
	}
	if full == rootReal {
		return ErrRootAccess
	}
	return os.RemoveAll(full)
}

// Stat 返回单个条目的信息。
func (m *FileManager) Stat(rel string) (Entry, error) {
	full, err := m.resolve(rel, true)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name:    info.Name(),
		Path:    m.relPath(full),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Unix(),
	}, nil
}
