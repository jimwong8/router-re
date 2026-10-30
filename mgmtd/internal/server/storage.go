package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
	"github.com/jimwong/router-re/mgmtd/internal/storage"
	"github.com/jimwong/router-re/mgmtd/internal/sysctl"
)

// 磁盘管理 / 文件管理 / 内核参数。
//
// 路径沿用 iKuai：system/disks、system/files、system/kernel-params。
// 破坏性操作（格式化、改分区表）一律两段式：先 dry_run 拿计划，
// 再带 confirmed=true 执行——中间那一步就是给人看一眼的机会。

func fileMgr() *storage.FileManager {
	m := storage.NewFileManager(storage.DefaultFileRoot)
	// 根目录不存在时建出来：存储没挂任何盘时文件管理页也应该是空列表，
	// 而不是一个 500。
	_ = os.MkdirAll(m.Root, 0o755)
	return m
}

// ---------- 磁盘 ----------

func (s *Server) handleGetDisks(w http.ResponseWriter, r *http.Request) {
	disks, err := storage.ListDisks()
	if err != nil {
		writeJSON(w, 500, errBody(11200, "list disks failed", err.Error()))
		return
	}
	if disks == nil {
		disks = []storage.Disk{}
	}
	writeJSON(w, 200, map[string]interface{}{
		"disks":        disks,
		"mount_root":   storage.MountRoot,
		"file_root":    storage.DefaultFileRoot,
		"max_parts":    storage.MaxUserPartitions,
		"min_part_gib": storage.MinPartitionBytes >> 30,
	})
}

func (s *Server) handleMountDisk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device   string `json:"device"`
		ReadOnly bool   `json:"read_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11201, "json decode fail", err.Error()))
		return
	}
	mp, err := storage.Mount(req.Device, req.ReadOnly)
	if err != nil {
		writeJSON(w, 422, errBody(11202, "mount failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"mounted": true, "mount_point": mp})
}

func (s *Server) handleUmountDisk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MountPoint string `json:"mount_point"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11203, "json decode fail", err.Error()))
		return
	}
	if err := storage.Unmount(req.MountPoint); err != nil {
		writeJSON(w, 422, errBody(11204, "umount failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"unmounted": true})
}

func (s *Server) handleFormatDisk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device    string `json:"device"`
		FSType    string `json:"fstype"`
		Label     string `json:"label"`
		DryRun    bool   `json:"dry_run"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11205, "json decode fail", err.Error()))
		return
	}
	plan, err := storage.PlanFormat(req.Device, req.FSType, req.Label)
	if err != nil {
		writeJSON(w, 422, errBody(11206, "format plan rejected", err.Error()))
		return
	}
	if req.DryRun || !req.Confirmed {
		writeJSON(w, 200, map[string]interface{}{"plan": plan, "executed": false,
			"note": "这是计划，未执行。确认后带 confirmed=true 再提交一次。"})
		return
	}
	plan.Confirmed = true
	out, err := storage.Execute(plan, execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(11207, "format failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"plan": plan, "executed": true, "output": out})
}

func (s *Server) handlePartitionDisk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device    string `json:"device"`
		StartMiB  int    `json:"start_mib"`
		EndMiB    int    `json:"end_mib"`
		DryRun    bool   `json:"dry_run"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11208, "json decode fail", err.Error()))
		return
	}
	plan, err := storage.PlanPartition(req.Device, req.StartMiB, req.EndMiB)
	if err != nil {
		writeJSON(w, 422, errBody(11209, "partition plan rejected", err.Error()))
		return
	}
	if req.DryRun || !req.Confirmed {
		writeJSON(w, 200, map[string]interface{}{"plan": plan, "executed": false,
			"note": "这是计划，未执行。确认后带 confirmed=true 再提交一次。"})
		return
	}
	plan.Confirmed = true
	out, err := storage.Execute(plan, execx.DefaultRunner{})
	if err != nil {
		writeJSON(w, 500, errBody(11210, "partition failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"plan": plan, "executed": true, "output": out})
}

// ---------- 文件管理 ----------

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}
	m := fileMgr()
	entries, err := m.List(path)
	if err != nil {
		writeJSON(w, 422, errBody(11220, "list files failed", err.Error()))
		return
	}
	if entries == nil {
		entries = []storage.Entry{}
	}
	writeJSON(w, 200, map[string]interface{}{
		"path":    path,
		"root":    m.Root,
		"entries": entries,
	})
}

func (s *Server) handleFileOp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Op      string `json:"op"`   // mkdir / newfile / rename / move / remove
		Path    string `json:"path"` // 所在目录（remove 时是目标本身）
		Name    string `json:"name"`
		NewName string `json:"new_name"`
		DstDir  string `json:"dst_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11221, "json decode fail", err.Error()))
		return
	}
	m := fileMgr()
	var err error
	switch req.Op {
	case "mkdir":
		err = m.Mkdir(req.Path, req.Name)
	case "newfile":
		err = m.CreateFile(req.Path, req.Name)
	case "rename":
		err = m.Rename(req.Path, req.Name, req.NewName)
	case "move":
		err = m.Move(req.Path, req.Name, req.DstDir)
	case "remove":
		target := req.Path
		if req.Name != "" {
			target = strings.TrimSuffix(req.Path, "/") + "/" + req.Name
		}
		err = m.Remove(target)
	default:
		writeJSON(w, 422, errBody(11222, "unknown op", "不支持的操作："+req.Op))
		return
	}
	if err != nil {
		writeJSON(w, 422, errBody(11223, "file op failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"ok": true, "op": req.Op})
}

// ---------- 内核参数 ----------

func (s *Server) handleGetKernelParams(w http.ResponseWriter, r *http.Request) {
	saved := sysctl.DefaultKernelParams()
	if raw, err := os.ReadFile(sysctl.KernelParamsPath()); err == nil {
		saved = sysctl.ParseSysctlConf(string(raw))
	}
	if raw, _ := s.store.Get("system", "kernel_params"); raw != "" {
		var p sysctl.KernelParams
		if json.Unmarshal([]byte(raw), &p) == nil {
			saved = p
		}
	}
	current, missing := sysctl.ReadCurrent()
	writeJSON(w, 200, map[string]interface{}{
		"config":  saved,
		"current": current,
		"missing": missing,
		"path":    sysctl.KernelParamsPath(),
		"specs":   sysctl.KernelSpecs,
	})
}

func (s *Server) handleSetKernelParams(w http.ResponseWriter, r *http.Request) {
	var req struct {
		sysctl.KernelParams
		Apply bool `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11240, "json decode fail", err.Error()))
		return
	}
	if err := sysctl.ValidateKernelParams(req.KernelParams); err != nil {
		writeJSON(w, 422, errBody(11241, "kernel params invalid", err.Error()))
		return
	}
	conf := sysctl.RenderKernelConf(req.KernelParams)
	if err := writeFileSafe(sysctl.KernelParamsPath(), conf); err != nil {
		writeJSON(w, 500, errBody(11242, "write sysctl conf failed", err.Error()))
		return
	}
	b, _ := json.Marshal(req.KernelParams)
	_ = s.store.Set("system", "kernel_params", string(b))

	resp := map[string]interface{}{"config": req.KernelParams, "conf": conf, "applied": false}
	if req.Apply {
		runner := execx.DefaultRunner{}
		out, err := runner.Run("sysctl", "-p", sysctl.KernelParamsPath())
		okKeys, failedKeys := sysctl.ParseApplyOutput(out)
		resp["output"] = strings.TrimSpace(out)
		resp["failed_keys"] = failedKeys
		resp["applied"] = len(okKeys) > 0
		if len(okKeys) > 0 {
			// 有键生效就保留配置文件：删掉会让已生效的值在重启后全部丢失
			current, missing := sysctl.ReadCurrent()
			resp["current"] = current
			resp["missing"] = missing
		}
		if len(okKeys) == 0 && err != nil {
			writeJSON(w, 500, errBody(11243, "sysctl apply failed", strings.TrimSpace(out)))
			return
		}
	}
	writeJSON(w, 200, resp)
}
