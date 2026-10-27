// Package voucher 实现上网卡密（优惠券）的生成、校验与核销。
package voucher

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// Voucher 是一张上网优惠券/卡密。
type Voucher struct {
	Code        string `json:"code"`
	Password    string `json:"password"`
	Minutes     int    `json:"minutes"`
	TrafficMB   int64  `json:"traffic_mb"`
	DeviceLimit int    `json:"device_limit"`
	CreatedAt   int64  `json:"created_at"`
	ExpiresAt   int64  `json:"expires_at"`
	UsedAt      int64  `json:"used_at"`
	Used        bool   `json:"used"`
	Note        string `json:"note"`
}

// errBadVoucher 是统一的核销失败错误，避免枚举。
var errBadVoucher = fmt.Errorf("卡号或卡密错误")

// Validate 校验卡密。小写 code 自动转大写。
func Validate(v interface{}) error {
	var ptr *Voucher
	var val Voucher
	switch t := v.(type) {
	case *Voucher:
		ptr = t
		val = *t
	case Voucher:
		val = t
	default:
		return fmt.Errorf("Validate expects Voucher or *Voucher, got %T", v)
	}
	code := strings.ToUpper(strings.TrimSpace(val.Code))
	val.Code = code
	if len(code) < 6 || len(code) > 32 {
		return fmt.Errorf("code length must be 6-32, got %d", len(code))
	}
	for _, c := range code {
		if !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
			return fmt.Errorf("code must contain only [A-Z0-9], got %q", val.Code)
		}
	}
	if len(val.Password) < 6 || len(val.Password) > 32 {
		return fmt.Errorf("password length must be 6-32, got %d", len(val.Password))
	}
	if val.Minutes != 0 && (val.Minutes < 1 || val.Minutes > 525600) {
		return fmt.Errorf("minutes must be 0 or 1-525600, got %d", val.Minutes)
	}
	if val.TrafficMB < 0 {
		return fmt.Errorf("traffic_mb must be >= 0")
	}
	if val.DeviceLimit < 1 || val.DeviceLimit > 10 {
		return fmt.Errorf("device_limit must be 1-10, got %d", val.DeviceLimit)
	}
	if val.ExpiresAt != 0 && val.ExpiresAt <= val.CreatedAt {
		return fmt.Errorf("expires_at must be 0 or > created_at")
	}
	// 归一化写回调用方（如果传的是指针）
	if ptr != nil {
		ptr.Code = code
	}
	return nil
}

// charset 排除易混字符 0/O、1/I/l。
const voucherCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateCode 生成随机卡号，使用 crypto/rand。
func GenerateCode(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("length must be > 0")
	}
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(voucherCharset))))
		if err != nil {
			return "", err
		}
		b[i] = voucherCharset[idx.Int64()]
	}
	return string(b), nil
}

// GenerateBatch 批量生成，保证批内不重复。
func GenerateBatch(n, codeLen int) ([]string, error) {
	if n <= 0 || codeLen <= 0 {
		return nil, fmt.Errorf("n and codeLen must be > 0")
	}
	seen := map[string]bool{}
	out := make([]string, 0, n)
	attempts := 0
	maxAttempts := n * 10
	for len(out) < n && attempts < maxAttempts {
		attempts++
		code, err := GenerateCode(codeLen)
		if err != nil {
			return nil, err
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	if len(out) < n {
		return nil, fmt.Errorf("could only generate %d unique codes, wanted %d", len(out), n)
	}
	return out, nil
}

// Redeem 核销一张卡。返回 (可上网时长分钟, 可上网流量MB, error)。
func Redeem(v *Voucher, password string, now int64) (int, int64, error) {
	if v == nil {
		return 0, 0, errBadVoucher
	}
	// 1. 卡是否存在：由调用方保证指针非 nil；这里只校验密码
	// 2. 卡密正确性：统一错误，不区分卡号/密码错误，防枚举
	if v.Password != password {
		return 0, 0, errBadVoucher
	}
	// 3. 是否已过期
	if v.ExpiresAt != 0 && now > v.ExpiresAt {
		return 0, 0, fmt.Errorf("卡已过期")
	}
	// 4. 是否已用完
	if v.Used && v.Minutes > 0 && RemainingMinutes(*v, now) <= 0 {
		return 0, 0, fmt.Errorf("卡已用完")
	}
	// 核销
	if v.UsedAt == 0 {
		v.UsedAt = now
	}
	v.Used = true
	return v.Minutes, v.TrafficMB, nil
}

// RemainingMinutes 计算剩余时长。
func RemainingMinutes(v Voucher, now int64) int {
	if !v.Used {
		return v.Minutes
	}
	if v.Minutes == 0 {
		return 0 // 不限量
	}
	elapsed := int((now - v.UsedAt) / 60)
	rem := v.Minutes - elapsed
	if rem < 0 {
		return 0
	}
	return rem
}

// Mask 返回打码副本（卡密只留前 2 后 2）。
func Mask(v Voucher) Voucher {
	out := v
	if len(v.Password) <= 4 {
		out.Password = strings.Repeat("*", len(v.Password))
	} else {
		out.Password = v.Password[:2] + strings.Repeat("*", len(v.Password)-4) + v.Password[len(v.Password)-2:]
	}
	return out
}
