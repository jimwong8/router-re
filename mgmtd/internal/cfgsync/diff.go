package cfgsync

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// Change 描述一个配置字段的变化。
type Change struct {
	Section string `json:"section"`
	Key     string `json:"key"` // 扁平化路径，如 "lan.ipaddr"
	Old     string `json:"old"`
	New     string `json:"new"`
	Kind    string `json:"kind"` // added / removed / changed
}

// Diff 比较两个 bundle，返回变更列表（按 Section+Key 排序，输出稳定）。
//
// 对每个 section 的 JSON 做扁平化（Flatten 递归展平成 a.b.c 形式的 key）再比较，
// 这样嵌套字段的差异能定位到具体路径，而不是"整个 section 变了"。
func Diff(old, new Bundle) []Change {
	var changes []Change
	sections := make(map[string]bool)
	for s := range old.Sections {
		sections[s] = true
	}
	for s := range new.Sections {
		sections[s] = true
	}
	// 排序保证遍历稳定。
	secNames := make([]string, 0, len(sections))
	for s := range sections {
		secNames = append(secNames, s)
	}
	sort.Strings(secNames)

	for _, sec := range secNames {
		oldRaw, oldHas := old.Sections[sec]
		newRaw, newHas := new.Sections[sec]

		oldFlat, oldErr := Flatten(oldRaw, "")
		newFlat, newErr := Flatten(newRaw, "")

		switch {
		case !oldHas && newHas:
			// 整 section 新增：逐字段 added
			for k := range newFlat {
				changes = append(changes, Change{Section: sec, Key: k, New: newFlat[k], Kind: "added"})
			}
		case oldHas && !newHas:
			// 整 section 移除：逐字段 removed
			for k := range oldFlat {
				changes = append(changes, Change{Section: sec, Key: k, Old: oldFlat[k], Kind: "removed"})
			}
		default:
			// 两侧都有：逐字段比较（old 或 new 解析失败时按该侧为空处理）
			if oldErr != nil {
				oldFlat = map[string]string{}
			}
			if newErr != nil {
				newFlat = map[string]string{}
			}
			allKeys := make(map[string]bool)
			for k := range oldFlat {
				allKeys[k] = true
			}
			for k := range newFlat {
				allKeys[k] = true
			}
			keys := make([]string, 0, len(allKeys))
			for k := range allKeys {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				ov, ohas := oldFlat[k]
				nv, nhas := newFlat[k]
				switch {
				case !ohas && nhas:
					changes = append(changes, Change{Section: sec, Key: k, New: nv, Kind: "added"})
				case ohas && !nhas:
					changes = append(changes, Change{Section: sec, Key: k, Old: ov, Kind: "removed"})
				case ov != nv:
					changes = append(changes, Change{Section: sec, Key: k, Old: ov, New: nv, Kind: "changed"})
				}
			}
		}
	}
	// 按 Section+Key 排序保证输出稳定（两次调用结果相同）。
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Section != changes[j].Section {
			return changes[i].Section < changes[j].Section
		}
		return changes[i].Key < changes[j].Key
	})
	return changes
}

// Flatten 把 JSON 递归展平成 a.b.c 形式的扁平 key -> 字符串值。
// 数组用 a[0]、a[1] 下标；对象递归。返回 map[string]string。
// 非法 JSON 返回 error。prefix 是当前路径前缀（根调用传 ""）。
func Flatten(raw json.RawMessage, prefix string) (map[string]string, error) {
	out := make(map[string]string)
	if len(raw) == 0 {
		return out, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON: %s", string(raw))
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	flattenVal(out, prefix, v)
	return out, nil
}

func flattenVal(out map[string]string, prefix string, v interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, sub := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenVal(out, key, sub)
		}
	case []interface{}:
		for i, sub := range val {
			key := prefix + "[" + strconv.Itoa(i) + "]"
			flattenVal(out, key, sub)
		}
	case nil:
		out[prefix] = ""
	default:
		out[prefix] = fmt.Sprintf("%v", val)
	}
}
