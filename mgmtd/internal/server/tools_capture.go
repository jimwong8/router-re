package server

// 抓包工具（tcpdump）端点。独立文件：remote.go 曾被返工任务整体覆盖过多次，
// 把这里连同别的 handler 一起吞掉（路由还在、函数没了，编译直接断）。

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CaptureJob 是一次抓包任务的运行时状态。
type CaptureJob struct {
	mu           sync.Mutex
	ID           string    `json:"id"`
	Interface    string    `json:"interface"`
	Filter       string    `json:"filter"`
	MaxSeconds   int       `json:"max_seconds"`
	MaxBytes     int64     `json:"max_bytes"`
	OutputPath   string    `json:"output_path"`
	StartTime    time.Time `json:"start_time"`
	Status       string    `json:"status"`
	ErrorMsg     string    `json:"error,omitempty"`
	BytesWritten int64     `json:"bytes_written"`

	cancel context.CancelFunc
}

var (
	captureJobsMu   sync.Mutex
	captureJobs = map[string]*CaptureJob{}
)

// 硬上限：即使调用方传更大的值也不接受，避免一次抓包把磁盘写满。
const (
	captureMaxSecondsLimit = 600
	captureMaxBytesLimit   = 512 << 20
)

func newCaptureID() string {
	return fmt.Sprintf("%s-%06d", time.Now().Format("20060102-150405"), rand.Intn(1000000))
}

// handleCaptureStart 启动抓包。POST /api/v1/tools/capture/start
func (s *Server) handleCaptureStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Interface string `json:"interface"`
		Filter    string `json:"filter"`
		Seconds   int    `json:"seconds"`
		MaxBytes  int64  `json:"max_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(10491, "invalid body", err.Error()))
		return
	}
	if strings.TrimSpace(req.Interface) == "" {
		writeJSON(w, 422, errBody(10492, "missing interface", "interface is required"))
		return
	}

	sec := req.Seconds
	if sec <= 0 || sec > captureMaxSecondsLimit {
		sec = 60
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 || maxBytes > captureMaxBytesLimit {
		maxBytes = 32 << 20
	}

	id := newCaptureID()
	out := filepath.Join(os.TempDir(), "capture-"+id+".pcap")
	ctx, cancel := context.WithCancel(context.Background())

	job := &CaptureJob{
		ID: id, Interface: req.Interface, Filter: req.Filter,
		MaxSeconds: sec, MaxBytes: maxBytes, OutputPath: out,
		StartTime: time.Now(), Status: "running", cancel: cancel,
	}
	captureJobsMu.Lock()
	captureJobs[id] = job
	captureJobsMu.Unlock()

	go func() {
		args := []string{"-i", req.Interface, "-w", out, "-B", "4096", "-U", "-s", "0"}
		if req.Filter != "" {
			args = append(args, req.Filter)
		}
		// 必须 CommandContext：execx.Runner.Run 是阻塞调用、不认 ctx，
		// stop 会返回 200 但 tcpdump 继续写盘。
		cmd := exec.CommandContext(ctx, "tcpdump", args...)
		err := cmd.Run()

		job.mu.Lock()
		defer job.mu.Unlock()
		switch {
		case ctx.Err() != nil:
			job.Status = "stopped"
		case err != nil:
			job.Status = "error"
			job.ErrorMsg = err.Error()
		default:
			job.Status = "completed"
		}
		if fi, statErr := os.Stat(out); statErr == nil {
			job.BytesWritten = fi.Size()
		}
	}()

	// 看门狗：超时或超过体积上限主动停，不依赖调用方记得来 stop。
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		deadline := time.Now().Add(time.Duration(sec) * time.Second)
		for range tick.C {
			job.mu.Lock()
			running := job.Status == "running"
			job.mu.Unlock()
			if !running {
				return
			}
			over := false
			if fi, err := os.Stat(out); err == nil && fi.Size() >= maxBytes {
				over = true
			}
			if time.Now().After(deadline) || over {
				cancel()
				return
			}
		}
	}()

	writeJSON(w, 200, map[string]interface{}{
		"id": id, "status": "running",
		"interface": req.Interface, "max_seconds": sec, "max_bytes": maxBytes,
	})
}

// handleCaptureStatus 查单个任务（?id=）或列表。GET /api/v1/tools/capture/status
func (s *Server) handleCaptureStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		captureJobsMu.Lock()
		list := make([]map[string]interface{}, 0, len(captureJobs))
		for _, j := range captureJobs {
			list = append(list, captureSnapshot(j))
		}
		captureJobsMu.Unlock()
		writeJSON(w, 200, map[string]interface{}{"captures": list, "count": len(list)})
		return
	}

	captureJobsMu.Lock()
	job, ok := captureJobs[id]
	captureJobsMu.Unlock()
	if !ok {
		writeJSON(w, 404, errBody(10493, "capture not found", "no such capture id"))
		return
	}
	writeJSON(w, 200, captureSnapshot(job))
}

func captureSnapshot(j *CaptureJob) map[string]interface{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	return map[string]interface{}{
		"id": j.ID, "status": j.Status, "interface": j.Interface,
		"filter": j.Filter, "output_path": j.OutputPath,
		"bytes_written": j.BytesWritten, "max_seconds": j.MaxSeconds,
		"max_bytes": j.MaxBytes, "start_time": j.StartTime, "error": j.ErrorMsg,
	}
}

// handleCaptureDownload 下载 pcap。GET /api/v1/tools/capture/{id}/download
func (s *Server) handleCaptureDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := "download"
	if id == "" {
		id, action = splitCapturePath(r.URL.Path)
	}
	if id == "" || action != "download" {
		writeJSON(w, 400, errBody(10494, "bad path", "expected /{id}/download"))
		return
	}
	captureJobsMu.Lock()
	job, ok := captureJobs[id]
	captureJobsMu.Unlock()
	if !ok {
		writeJSON(w, 404, errBody(10493, "capture not found", "no such capture id"))
		return
	}
	job.mu.Lock()
	path, status := job.OutputPath, job.Status
	job.mu.Unlock()
	if status == "running" {
		writeJSON(w, 409, errBody(10496, "still running", "stop the capture before downloading"))
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeJSON(w, 404, errBody(10495, "file missing", path))
		return
	}
	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+".pcap"))
	http.ServeFile(w, r, path)
}

// handleCaptureStop 停止抓包。POST /api/v1/tools/capture/{id}/stop
func (s *Server) handleCaptureStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := "stop"
	if id == "" {
		id, action = splitCapturePath(r.URL.Path)
	}
	if id == "" || action != "stop" {
		writeJSON(w, 400, errBody(10494, "bad path", "expected /{id}/stop"))
		return
	}
	captureJobsMu.Lock()
	job, ok := captureJobs[id]
	captureJobsMu.Unlock()
	if !ok {
		writeJSON(w, 404, errBody(10493, "capture not found", "no such capture id"))
		return
	}
	job.mu.Lock()
	if job.Status != "running" {
		st := job.Status
		job.mu.Unlock()
		writeJSON(w, 409, errBody(10500, "not running", "capture is "+st))
		return
	}
	// cancel 通过 context 传入 exec.CommandContext(run ctx) → 停止时终止进程。
	if job.cancel != nil {
		job.cancel()
	}
	job.Status = "stopped"
	job.mu.Unlock()
	writeJSON(w, 200, map[string]interface{}{"id": id, "status": "stopped", "message": "capture stopped"})
}

// splitCapturePath 从 /api/v1/tools/capture/{id}/{action} 取出 id 与动作。
func splitCapturePath(p string) (string, string) {
	const prefix = "/api/v1/tools/capture/"
	if !strings.HasPrefix(p, prefix) {
		return "", ""
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(p, prefix), "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}
