package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/netgroup"
)

// 对象管理：IP 分组 / MAC 分组 / 终端分组。
//
// iKuai 把这三种分组做成独立对象（menu.json 里的「对象管理」一组），
// ACL、分流、限速都引用同一个分组 ID，改一处全生效。之前的实现只能在
// 每条 ACL 里内联写地址，同一个网段在 5 条规则里出现 5 次，改一次要改 5 处。
//
// 存储用单个 JSON 数组（键 objects/groups），规模是几十到几百个分组，
// 没必要为此引入一张表。

func (s *Server) loadGroups() []netgroup.Group {
	// 空集合返回 []，不返回 null：前端 v-for 遇到 null 会报错，
	// 仓库里 empty_arrays_test.go 也是按这条规范检查的。
	out := make([]netgroup.Group, 0)
	raw, _ := s.store.Get("objects", "groups")
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func (s *Server) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	groups := s.loadGroups()
	writeJSON(w, 200, map[string]interface{}{
		"groups": groups,
		"kinds":  []string{netgroup.KindIP, netgroup.KindMAC, netgroup.KindTerminal},
	})
}

func (s *Server) handleSetGroups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Groups []netgroup.Group `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, errBody(11100, "json decode fail", err.Error()))
		return
	}
	if len(req.Groups) > 512 {
		writeJSON(w, 422, errBody(11101, "too many groups", "分组最多 512 个"))
		return
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	cleaned := make([]netgroup.Group, 0, len(req.Groups))
	for i, g := range req.Groups {
		if err := netgroup.ValidateGroup(g); err != nil {
			writeJSON(w, 422, errBody(11102, "group invalid",
				"第 "+strconv.Itoa(i+1)+" 个分组非法："+err.Error()))
			return
		}
		// ID 由 (kind,name) 稳定推导：与列表顺序无关，中文名也不会撞车
		g.ID = netgroup.GroupID(g.Kind, g.Name)
		key := strings.ToLower(g.Kind + "/" + g.Name)
		if names[key] {
			writeJSON(w, 422, errBody(11103, "duplicate group name",
				"分组名重复："+g.Name+"（同类型下不能重名）"))
			return
		}
		names[key] = true
		// 走到这里还撞 ID 说明两个分组的还是同一个 (kind,name)，
		// 那是上面 names 检查漏了；保守起见仍加序号避免覆盖。
		if ids[g.ID] {
			g.ID = g.ID + "_" + strconv.Itoa(i+1)
		}
		ids[g.ID] = true
		g.Items = netgroup.NormalizeItems(g.Kind, g.Items)
		cleaned = append(cleaned, g)
	}
	b, err := json.Marshal(cleaned)
	if err != nil {
		writeJSON(w, 500, errBody(11104, "marshal fail", err.Error()))
		return
	}
	if err := s.store.Set("objects", "groups", string(b)); err != nil {
		writeJSON(w, 500, errBody(11105, "store fail", err.Error()))
		return
	}
	writeJSON(w, 200, map[string]interface{}{"groups": cleaned, "saved": true})
}

// handleRenderGroupSet 把一个分组渲染成 nft 集合文本，前端用来预览/复制。
func (s *Server) handleRenderGroupSet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	for _, g := range s.loadGroups() {
		if g.ID == id {
			set, err := netgroup.RenderNftSet(g)
			if err != nil {
				writeJSON(w, 422, errBody(11106, "render fail", err.Error()))
				return
			}
			writeJSON(w, 200, map[string]interface{}{"id": id, "nft": set})
			return
		}
	}
	writeJSON(w, 404, errBody(11107, "group not found", "分组不存在："+id))
}
