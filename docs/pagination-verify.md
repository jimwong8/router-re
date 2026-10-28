# 分页语义运行时验证报告

> 生成: 2026-10-28 20:43:48  BASE=http://127.0.0.1:9090

**汇总**: 8 项 / PASS 8 / FAIL 0

| 检查项 | 结果 | 实测 |
|---|---|---|
| services 无参数全量且 count==total | PASS | count=525 total=525 has_more=False |
| services limit=10 只回 10 条且 total 是真总数 | PASS | count=10 total=525 |
| services 最后一页 has_more=false 且条数正确 | PASS | offset=520 count=5 期望 5 has_more=False |
| services limit 超上限被夹住但不报错 | PASS | count=525 期望 525 |
| logs 窗口填满时 has_more=true 且 total=-1（不谎报总数） | PASS | count=10 total=-1 has_more=True |
| logs 回带 limit/offset 便于前端翻页 | PASS | limit=10 offset=0 |
| logs 取到末尾时 total 是真值、has_more=false | PASS | count=0 total=0 has_more=False |
| logs 无 token 仍 401 | PASS | HTTP 401 |

## 说明

服务列表能拿到全量，所以 `total` 始终是真值；日志走 `journalctl -n`，
取到的只是尾部窗口，窗口被填满时无法确定总数，因此返回 `total=-1` +
`has_more=true`——宁可说"还有更多但不知道多少"，也不谎报一个假总数。
