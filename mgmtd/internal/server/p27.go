package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/firmware"
)

// ---------- 固件二次确认 ----------

// handleFirmwareConfirm 只做验证，不刷机、不重启。
//
// 流程：
//  1. InspectImage 拿真实镜像（sha256/magic/size/board）
//  2. body.sha256 必须与实测一致，否则 422（码 10500）
//  3. 本机 board 来源：body.board 优先；拿不到则尝试 /etc/board.json、
//     /proc/device-tree/model、/tmp/sysinfo/board_name；再拿不到则 board 检查跳过，
//     响应里 board_checked:false
//  4. 全过返回 verified:true + image + plan；plan 失败不挡 verified，只带 plan_error
//  5. 绝不调用 firmware.Apply，绝不重启设备
func (s *Server) handleFirmwareConfirm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Board  string `json:"board"`
		Force  bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 422, errBody(10503, "invalid body", err.Error()))
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		writeJSON(w, 422, errBody(10504, "missing path", "path points to the firmware image on the server"))
		return
	}

	img, err := firmware.InspectImage(body.Path)
	if err != nil {
		writeJSON(w, 422, errBody(10501, "读取镜像失败", err.Error()))
		return
	}

	// sha256 必须与实测一致 —— 防止传错文件/被篡改
	if strings.TrimSpace(body.SHA256) != "" && !strings.EqualFold(strings.TrimSpace(body.SHA256), img.SHA256) {
		writeJSON(w, 422, errBody(10500, "sha256 mismatch",
			fmt.Sprintf("body sha256=%s but inspected image sha256=%s", body.SHA256, img.SHA256)))
		return
	}

	// 本机 board 来源
	localBoard := strings.TrimSpace(body.Board)
	boardChecked := false
	if localBoard == "" {
		if detected, ok := detectLocalBoard(); ok {
			localBoard = detected
			boardChecked = true
		}
	} else {
		boardChecked = true
		// body.board 视为镜像声明的板型，写回 img.Board 以便 ValidateImage 比对
		img.Board = localBoard
	}

	verified := true
	if boardChecked {
		if err := firmware.ValidateImage(img, localBoard); err != nil {
			verified = false
			writeJSON(w, 422, errBody(10502, "镜像不可用于本机", err.Error()))
			return
		}
	}

	// 生成计划；失败不挡 verified，只带 plan_error
	hasBackup := false
	plan, perr := firmware.BuildPlan(img, localBoard, hasBackup)
	resp := map[string]interface{}{
		"verified":      verified,
		"board_checked": boardChecked,
		"image": map[string]interface{}{
			"sha256": img.SHA256,
			"board":  img.Board,
			"magic":  img.Magic,
			"size":   img.Size,
		},
	}
	if perr != nil {
		resp["plan_error"] = perr.Error()
		resp["plan"] = nil
	} else {
		resp["plan"] = plan
	}
	writeJSON(w, 200, resp)
}

// detectLocalBoard 尝试从常见位置读取本机板型。拿不到返回 false。
func detectLocalBoard() (string, bool) {
	// /etc/board.json
	if data, err := os.ReadFile("/etc/board.json"); err == nil {
		var m map[string]interface{}
		if json.Unmarshal(data, &m) == nil {
			for _, key := range []string{"model", "board_name", "board", "id", "name"} {
				if v, ok := m[key]; ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s), true
					}
				}
			}
		}
	}
	// /proc/device-tree/model 或 compatible
	for _, p := range []string{"/proc/device-tree/model", "/proc/device-tree/compatible"} {
		if data, err := os.ReadFile(p); err == nil {
			s := strings.TrimSpace(strings.TrimRight(string(data), "\x00"))
			if s != "" {
				return s, true
			}
		}
	}
	// /tmp/sysinfo
	for _, p := range []string{"/tmp/sysinfo/board_name", "/tmp/sysinfo/model"} {
		if data, err := os.ReadFile(p); err == nil {
			s := strings.TrimSpace(strings.TrimRight(string(data), "\x00"))
			if s != "" {
				return s, true
			}
		}
	}
	return "", false
}
