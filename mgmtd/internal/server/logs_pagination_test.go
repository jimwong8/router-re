package server

import "testing"

// 分页语义单测。
//
// 为什么要单独测这个纯函数：真实 journal 一直在刷（高流量时同一秒几十条重复
// 消息），用 HTTP 连打两次 limit/offset 根本比不出窗口有没有位移——实测两次
// 请求首条一模一样，看着像 offset 没生效，其实只是日志在刷屏。把语义钉在纯
// 函数上，边界一次说清。
//
// 顺便钉死一个真 bug：total 曾按 len(entries) 直接返回，于是「5000 条 journal
// 取 limit=10」会返回 total=10、has_more=false，前端以为总共就 10 条。
func TestPaginateEntries(t *testing.T) {
	mk := func(n int) []int {
		all := make([]int, n)
		for i := range all {
			all[i] = i
		}
		return all
	}

	cases := []struct {
		name        string
		all         []int
		limit       int
		offset      int
		queryLimit  int
		wantPage    []int
		wantTotal   int
		wantHasMore bool
	}{
		{
			name: "窗口被填满：说不清总数就说 -1，并标 has_more",
			all:  mk(20), limit: 10, offset: 0, queryLimit: 10,
			wantPage: mk(20)[0:10], wantTotal: -1, wantHasMore: true,
		},
		{
			name: "offset 位移：取第二页（较老的 10 条）",
			all:  mk(20), limit: 10, offset: 10, queryLimit: 20,
			wantPage: mk(20)[10:20], wantTotal: -1, wantHasMore: true,
		},
		{
			name: "取到末尾：total 是真总数，has_more 为假",
			all:  mk(7), limit: 10, offset: 0, queryLimit: 10,
			wantPage: mk(7), wantTotal: 7, wantHasMore: false,
		},
		{
			name: "空日志：返回空切片而不是 nil",
			all:  nil, limit: 10, offset: 0, queryLimit: 10,
			wantPage: []int{}, wantTotal: 0, wantHasMore: false,
		},
		{
			name: "offset 超过总数：空页但不 panic",
			all:  mk(5), limit: 10, offset: 99, queryLimit: 109,
			wantPage: []int{}, wantTotal: 5, wantHasMore: false,
		},
		{
			name: "最后一页不满：截到末尾",
			all:  mk(15), limit: 10, offset: 10, queryLimit: 20,
			wantPage: mk(15)[10:15], wantTotal: 15, wantHasMore: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page, total, hasMore := paginateEntries(c.all, c.limit, c.offset, c.queryLimit)
			if len(page) != len(c.wantPage) {
				t.Fatalf("page 长度 %d，期望 %d（page=%v）", len(page), len(c.wantPage), page)
			}
			for i := range page {
				if page[i] != c.wantPage[i] {
					t.Fatalf("page[%d]=%d，期望 %d（page=%v）", i, page[i], c.wantPage[i], page)
				}
			}
			if total != c.wantTotal {
				t.Fatalf("total=%d，期望 %d", total, c.wantTotal)
			}
			if hasMore != c.wantHasMore {
				t.Fatalf("has_more=%v，期望 %v", hasMore, c.wantHasMore)
			}
			if page == nil {
				t.Fatal("page 不能是 nil（前端会拿到 null 而不是空数组）")
			}
		})
	}
}
