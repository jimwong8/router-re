package server

import (
	"encoding/json"
	"net/http"

	"github.com/jimwong/router-re/mgmtd/internal/kvm"
)

// 虚拟机（KVM）管理。
//
// 接口第一件事返回 capability：这个环境的 CPU/设备/libvirt 到底支不支持。
// 不支持就明确说缺什么——路由器上给用户一堆点不动的按钮，比告诉他
// "这台机器没有 /dev/kvm" 更糟。

func (s *Server) handleListVMs(w http.ResponseWriter, r *http.Request) {
	cap := kvm.Detect()
	resp := map[string]interface{}{
		"capability": cap,
		"vms":        []kvm.VM{},
	}
	if !cap.Available {
		// 不可用时不尝试 virsh，避免把"命令不存在"当成"没有虚机"
		resp["note"] = cap.Reason
		writeJSON(w, 200, resp)
		return
	}
	vms, err := kvm.NewManager().List()
	if err != nil {
		resp["note"] = err.Error()
		writeJSON(w, 200, resp)
		return
	}
	if vms == nil {
		vms = []kvm.VM{}
	}
	resp["vms"] = vms
	writeJSON(w, 200, resp)
}

func (s *Server) handleCreateVM(w http.ResponseWriter, r *http.Request) {
	cap := kvm.Detect()
	if !cap.Available {
		writeJSON(w, 422, errBody(11600, "kvm unavailable", cap.Reason))
		return
	}
	var req struct {
		kvm.CreateSpec
		Start bool `json:"start"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11601, "json decode fail", err.Error()))
		return
	}
	if err := kvm.ValidateCreate(req.CreateSpec); err != nil {
		writeJSON(w, 422, errBody(11602, "vm spec rejected", err.Error()))
		return
	}
	if err := kvm.NewManager().Create(req.CreateSpec, req.Start); err != nil {
		writeJSON(w, 500, errBody(11603, "create vm failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"created": true, "name": req.Name, "started": req.Start,
		"xml": kvm.RenderXML(req.CreateSpec),
	})
}

func (s *Server) handleVMAction(w http.ResponseWriter, r *http.Request) {
	cap := kvm.Detect()
	if !cap.Available {
		writeJSON(w, 422, errBody(11604, "kvm unavailable", cap.Reason))
		return
	}
	name := r.PathValue("name")
	var body struct {
		Action string `json:"action"` // start / shutdown / destroy / autostart-on / autostart-off
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(11605, "json decode fail", err.Error()))
		return
	}
	m := kvm.NewManager()
	var err error
	switch body.Action {
	case "start":
		err = m.Start(name)
	case "shutdown":
		err = m.Shutdown(name)
	case "destroy":
		// 强制断电会丢数据，这是明确的破坏性操作，前端要二次确认
		err = m.Destroy(name)
	case "autostart-on":
		err = m.SetAutostart(name, true)
	case "autostart-off":
		err = m.SetAutostart(name, false)
	default:
		writeJSON(w, 422, errBody(11606, "unknown action", "只支持 start/shutdown/destroy/autostart-on/autostart-off"))
		return
	}
	if err != nil {
		writeJSON(w, 422, errBody(11607, "vm action failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"name": name, "action": body.Action, "ok": true})
}
