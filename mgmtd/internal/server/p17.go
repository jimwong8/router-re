package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/voucher"
)

// ---------- 卡密认证 ----------

func (s *Server) handleListVouchers(w http.ResponseWriter, r *http.Request) {
	list := s.loadVouchers()
	now := time.Now().Unix()
	type view struct {
		voucher.Voucher
		RemainingMin int  `json:"remaining_min"`
		Expired      bool `json:"expired"`
	}
	out := make([]view, 0, len(list))
	used, unused := 0, 0
	for _, v := range list {
		m := voucher.Mask(v) // 密码不出后端
		out = append(out, view{
			Voucher:      m,
			RemainingMin: voucher.RemainingMinutes(v, now),
			Expired:      v.ExpiresAt > 0 && now > v.ExpiresAt,
		})
		if v.Used {
			used++
		} else {
			unused++
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"count":    len(out),
		"used":     used,
		"unused":   unused,
		"vouchers": out,
	})
}

// handleGenerateVouchers 批量生成卡密。
// 私钥式的凭据在这里诞生，所以：不落日志、返回值是唯一一次能看到明文的机会。
func (s *Server) handleGenerateVouchers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Count       int    `json:"count"`
		CodeLen     int    `json:"code_len"`
		Minutes     int    `json:"minutes"`
		TrafficMB   int64  `json:"traffic_mb"`
		DeviceLimit int    `json:"device_limit"`
		ExpireDays  int    `json:"expire_days"`
		Note        string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(10430, "invalid JSON body", err.Error()))
		return
	}
	if body.Count < 1 || body.Count > 500 {
		writeJSON(w, 422, errBody(10431, "count must be 1-500",
			"一次最多生成 500 张，避免返回体过大和误操作"))
		return
	}
	if body.CodeLen == 0 {
		body.CodeLen = 10
	}
	if body.DeviceLimit == 0 {
		body.DeviceLimit = 1
	}
	codes, err := voucher.GenerateBatch(body.Count, body.CodeLen)
	if err != nil {
		writeJSON(w, 500, errBody(10432, "generate failed", err.Error()))
		return
	}
	now := time.Now().Unix()
	var expires int64
	if body.ExpireDays > 0 {
		expires = now + int64(body.ExpireDays)*86400
	}
	created := make([]voucher.Voucher, 0, len(codes))
	for _, c := range codes {
		// 卡密用同一批随机码派生出独立随机密码
		pw, err := voucher.GenerateCode(body.CodeLen)
		if err != nil {
			writeJSON(w, 500, errBody(10433, "generate password failed", err.Error()))
			return
		}
		v := voucher.Voucher{
			Code: c, Password: pw,
			Minutes: body.Minutes, TrafficMB: body.TrafficMB,
			DeviceLimit: body.DeviceLimit,
			CreatedAt:   now, ExpiresAt: expires, Note: body.Note,
		}
		if err := voucher.Validate(v); err != nil {
			// 生成出来的东西过不了自己的校验，说明参数有问题，必须报错而不是跳过
			writeJSON(w, 422, errBody(10434, "generated voucher failed validation", err.Error()))
			return
		}
		created = append(created, v)
	}
	all := append(s.loadVouchers(), created...)
	if err := s.saveVouchers(all); err != nil {
		writeJSON(w, 500, errBody(10435, "save failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"generated": len(created),
		// 明文只在这里出现一次，之后列表接口只有 Mask 后的版本
		"vouchers": created,
		"note":     "卡密明文只返回这一次，请立即保存；之后查询只能看到打码版本",
	})
}

// handleRedeemVoucher 核销。
func (s *Server) handleRedeemVoucher(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, errBody(10436, "invalid JSON body", err.Error()))
		return
	}
	list := s.loadVouchers()
	now := time.Now().Unix()
	for i := range list {
		if list[i].Code != body.Code && list[i].Code != upper(body.Code) {
			continue
		}
		v := list[i]
		mins, mb, err := voucher.Redeem(&v, body.Password, now)
		if err != nil {
			// Redeem 对"卡不存在"和"密码错"返回同一句话，这里不额外区分
			writeJSON(w, 422, errBody(10437, "核销失败", err.Error()))
			return
		}
		list[i] = v
		if err := s.saveVouchers(list); err != nil {
			writeJSON(w, 500, errBody(10438, "save failed", err.Error()))
			return
		}
		writeJSON(w, 200, map[string]interface{}{
			"ok":            true,
			"minutes":       mins,
			"traffic_mb":    mb,
			"remaining_min": voucher.RemainingMinutes(v, now),
			"device_limit":  v.DeviceLimit,
		})
		return
	}
	// 找不到卡：与密码错误同样的错误，避免枚举卡号
	writeJSON(w, 422, errBody(10439, "核销失败", "卡号或卡密错误"))
}

func (s *Server) handleDeleteVoucher(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, 422, errBody(10440, "code required", "传 ?code=XXXX"))
		return
	}
	list := s.loadVouchers()
	out := make([]voucher.Voucher, 0, len(list))
	removed := 0
	for _, v := range list {
		if v.Code == code || v.Code == upper(code) {
			removed++
			continue
		}
		out = append(out, v)
	}
	if removed == 0 {
		writeJSON(w, 404, errBody(10441, "not found", "没有卡号 "+code))
		return
	}
	if err := s.saveVouchers(out); err != nil {
		writeJSON(w, 500, errBody(10442, "save failed", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"removed": removed, "remaining": len(out)})
}

func (s *Server) loadVouchers() []voucher.Voucher {
	raw, err := s.store.Get("voucher", "list")
	if err != nil || raw == "" {
		return []voucher.Voucher{}
	}
	var list []voucher.Voucher
	if json.Unmarshal([]byte(raw), &list) != nil {
		return []voucher.Voucher{}
	}
	return list
}

func (s *Server) saveVouchers(list []voucher.Voucher) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.store.Set("voucher", "list", string(b))
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

var _ = strconv.Itoa
