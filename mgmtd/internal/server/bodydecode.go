package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// 整表替换型接口的请求体解析。
//
// 背景：诸如 /api/v1/firewall/rules、/api/v1/qos/limits、/api/v1/nat/rules
// 这类接口的语义是"用请求体里的数组整体替换现有配置"。若直接
// json.Decode 到 struct{ Rules []T `json:"rules"` }，字段名写错或漏传时
// 切片保持 nil，随后 Apply(nil) 会把线上配置整体清空并返回 200 ——
// 一次拼写错误就能静默抹掉防火墙规则，对路由器是灾难性行为。
//
// decodeReplaceList 要求请求体是 JSON 对象且必须包含 field 键：
//   - 字段缺失 → errMissingListField（调用方回 422，提示显式传 [] 才是清空）
//   - 显式空数组 [] → 正常解析为 nil/空切片，允许清空
const maxReplaceListBody = 4 << 20

var errMissingListField = errors.New("missing list field")

func decodeReplaceList[T any](r *http.Request, field string, dst *[]T) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxReplaceListBody))
	if err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return err
	}
	v, ok := probe[field]
	if !ok {
		return fmt.Errorf("%w: %q", errMissingListField, field)
	}
	// null 不是数组：显式清空必须写 []，避免 null 被静默当成"清空全部"
	if trimmed := bytes.TrimSpace(v); len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("%w: %q (null is not an array, use [] to clear)", errMissingListField, field)
	}
	return json.Unmarshal(v, dst)
}

// writeReplaceListError 把 decodeReplaceList 的错误统一映射成响应：
// 缺字段 422/10099，其它解析错误 400/10007。返回 false 表示已写出错误响应。
func (s *Server) writeReplaceListError(w http.ResponseWriter, r *http.Request, action, field string, err error) bool {
	if errors.Is(err, errMissingListField) {
		s.logAudit(r, action, "", "fail", err.Error())
		writeJSON(w, 422, errBody(10099, "missing list field",
			fmt.Sprintf("body must contain %q array; send [] explicitly to clear all rules", field)))
		return true
	}
	s.logAudit(r, action, "", "fail", "json decode: "+err.Error())
	writeJSON(w, 400, errBody(10007, "json decode fail", err.Error()))
	return true
}
