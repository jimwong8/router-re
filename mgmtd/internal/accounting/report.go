package accounting

import "sort"

// Report 是单个账号的配额使用报表。
type Report struct {
	Username string `json:"username"`
	UsedMB   int    `json:"used_mb"`
	QuotaMB  int    `json:"quota_mb"`
	Percent  int    `json:"percent"`
	Exceeded bool   `json:"exceeded"`
	LastSeen int64  `json:"last_seen"`
}

// BuildReport 把 counter 快照 + 账号配额合成报表，按使用百分比降序排序。
// used 为 username -> UsedMB；quotas 为 username -> 配额 MB（<=0 表示不限）；
// lastSeen 为 username -> 最近活跃时间戳。
//
// 百分比：quota<=0（不限）时 Percent=0（无上限可比），Exceeded=false；
// 否则 Percent = used*100/quota（向下取整，可能 >100），used>=quota 时 Exceeded。
// 注意：不存在的 username 不会出现在报表里（只对既有快照生成）。
func BuildReport(used map[string]int, quotas map[string]int, lastSeen map[string]int64) []Report {
	reports := make([]Report, 0, len(used))
	for u, usedMB := range used {
		quotaMB := quotas[u]
		exceeded := false
		percent := 0
		if quotaMB > 0 {
			if usedMB >= quotaMB {
				exceeded = true
			}
			percent = usedMB * 100 / quotaMB
		}
		reports = append(reports, Report{
			Username: u,
			UsedMB:   usedMB,
			QuotaMB:  quotaMB,
			Percent:  percent,
			Exceeded: exceeded,
			LastSeen: lastSeen[u],
		})
	}
	// 按使用百分比降序；同百分比按用户名升序保证稳定。
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].Percent != reports[j].Percent {
			return reports[i].Percent > reports[j].Percent
		}
		return reports[i].Username < reports[j].Username
	})
	return reports
}
