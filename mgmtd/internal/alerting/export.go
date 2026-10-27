package alerting

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RenderJSONL 把日志条目导出成 JSONL（每行一个 JSON 对象）。
// lines 的每行作为一个条目；行内的转义由 json.Marshal 处理。
func RenderJSONL(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		obj := map[string]string{"message": line}
		raw, err := json.Marshal(obj)
		if err != nil {
			continue
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

// RenderCSV 把 entries 导出成 RFC4180 CSV。
//
// CSV 转义规则（最容易错的地方，务必遵守）：
//   - 字段含逗号、双引号或换行时，整个字段用双引号包裹；
//   - 字段内的双引号要翻倍（""）转义；
//   - 不外层的字段直接原样输出。
//
// 每行一行；空 entries 返回空串。
func RenderCSV(entries [][]string) string {
	var b strings.Builder
	for rowIdx, row := range entries {
		if rowIdx > 0 {
			b.WriteByte('\n')
		}
		for i, field := range row {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(csvEscape(field))
		}
	}
	return b.String()
}

// csvEscape 按 RFC4180 转义单个字段：仅当含逗号/双引号/换行时才包裹。
func csvEscape(field string) string {
	if strings.ContainsAny(field, ",\"\n\r") {
		var sb strings.Builder
		sb.WriteByte('"')
		for _, r := range field {
			if r == '"' {
				sb.WriteString(`""`)
			} else {
				sb.WriteRune(r)
			}
		}
		sb.WriteByte('"')
		return sb.String()
	}
	return field
}

// ParseTimeRange 解析相对时间 "1h" / "30m" / "24h" / "7d"，相对 now 回推。
// 返回 (since, until) 毫秒时间戳。不支持的格式返回明确错误。
func ParseTimeRange(spec string, now int64) (int64, int64, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, fmt.Errorf("empty time range spec")
	}
	if len(spec) < 2 {
		return 0, 0, fmt.Errorf("invalid time range %q (want e.g. 1h/30m/24h/7d)", spec)
	}
	numStr := spec[:len(spec)-1]
	unit := spec[len(spec)-1]
	var n int64
	for _, c := range numStr {
		if c < '0' || c > '9' {
			return 0, 0, fmt.Errorf("invalid time range %q (want e.g. 1h/30m/24h/7d)", spec)
		}
		n = n*10 + int64(c-'0')
	}
	if n <= 0 {
		return 0, 0, fmt.Errorf("invalid time range %q: amount must be > 0", spec)
	}
	var d time.Duration
	switch unit {
	case 'h':
		d = time.Duration(n) * time.Hour
	case 'm':
		d = time.Duration(n) * time.Minute
	case 's':
		d = time.Duration(n) * time.Second
	case 'd':
		d = time.Duration(n) * 24 * time.Hour
	default:
		return 0, 0, fmt.Errorf("invalid time unit %q in %q (supported h/m/s/d)", unit, spec)
	}
	since := now - d.Milliseconds()
	return since, now, nil
}
