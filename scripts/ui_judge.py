#!/usr/bin/env python3
"""ui_judge.py — UI 验收的「渲染行数 vs 接口条数」判定，三份验收脚本共用。

## 为什么要抽出来

`ui_acceptance.py` / `ui_acceptance_ext.py` / `ui_acceptance_terminals.py` 各抄了
一份判据。分页上线后，我改了其中一份、忘了另外两份，于是同一件事在三个脚本里
给出不同结论：/services 接口 525 条、当页 50 行，一个脚本判 OK、两个判 FAIL。

## 判据本身

要抓的真症状是「接口有数据、页面却空着」：

- 接口 0 条          → 只要满足 min_rows 即可（空列表是合法状态）
- 接口 N 条、无分页   → 页面行数 >= N * 0.8（el-table 合计行会有 ±1 出入）
- 接口 N 条、有分页   → 只要求当页非空。前端自己按 limit 取数，页面本就只渲染
                        当页那几十行，拿总数去卡必然假 FAIL

分页的识别交给调用方（它已经拿到页面 DOM），这里只做判定。
"""
from __future__ import annotations

PAGER_SELECTOR = ".el-pagination, .el-pager, button.el-pagination"

# 在页面里数可见 tab-pane 的表格行，并顺手报告有没有分页控件。
# 返回 {rows, has_pager}，一次 evaluate 拿全，省一次往返。
PROBE_JS = """() => {
  const panes = Array.from(document.querySelectorAll('.el-tab-pane'));
  let scope = document;
  if (panes.length) {
    const vis = panes.find(p => getComputedStyle(p).display !== 'none');
    if (vis) scope = vis;
  }
  return {
    rows: scope.querySelectorAll('table tbody tr').length,
    has_pager: !!scope.querySelector('.el-pagination, .el-pager, button.el-pagination'),
  };
}"""


def judge(rows: int, expect: int, has_pager: bool, min_rows: int = 0) -> tuple[bool, str]:
    """返回 (是否通过, 附加标注)。"""
    if expect and expect > 0:
        if has_pager and rows > 0:
            return True, " [分页]"
        return (rows > 0 and rows >= expect * 0.8), ""
    return rows >= min_rows, ""
