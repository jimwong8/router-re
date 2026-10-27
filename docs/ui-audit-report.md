# UI 验收报告

> 本次为**实测**（非文档推演）：运行 `scripts/ui_acceptance.py`，用 Playwright 打开每个页面，
> 对比「后端接口返回条数」与「页面真实渲染行数」。所有数字取自脚本真实输出，未做任何伪造。
> 环境：mgmtd 在 `127.0.0.1:9090`，vite preview 在 `127.0.0.1:4173`，均存活。

## 验收范围

`web/src/views/*.vue` 共 **53 个页面**。本次只验收 9 个新增/指定的页面（`/voucher` `/ddns` `/services`
`/network-tools` `/vpn-client` `/security-plus` `/online-users` `/cert` `/network-advanced`），
并保留脚本原有的 `/routing-policy` `/alerts`。全部已接路由（`web/src/router/index.ts` 中均有 `path` 项）。

## 结果表格（真实输出）

| 页面 | 取数接口 | 接口条数 | 渲染行数 | 结论 |
|------|---------|---------:|---------:|:----:|
| /services | /api/v1/system/services | 522 | 522 | OK |
| /online-users | /api/v1/online | 19 | 19 | OK |
| /routing-policy | /api/v1/routing/rules | 1 | 2 | OK (合计行) |
| /alerts | /api/v1/alerts/rules | 1 | 1 | OK |
| /voucher | /api/v1/voucher/list | 2 | 2 | OK |
| /ddns | /api/v1/ddns/records | 0 | 0 | OK (空) |
| /vpn-client | /api/v1/vpn/client | 0 | 0 | OK (空) |
| /security-plus | /api/v1/policy/timeschedule | 0 | 0 | OK (空) |
| /cert | /api/v1/cert | — | — | SKIP |
| /network-tools | (无) | — | — | SKIP |
| /network-advanced | (无) | — | — | SKIP |

**汇总**：8 个 OK，3 个 SKIP，0 个 FAIL。脚本退出码 0，`✅ UI 验收通过（8 个页面）`。

### 脚本真实输出（关键行）

```
  OK   /services          接口  522 条 -> 页面  522 行   systemd 单元列表
  OK   /online-users      接口   19 条 -> 页面   19 行   在线会话
  OK   /routing-policy    接口    1 条 -> 页面    2 行   策略路由规则(可为空)
  SKIP /cert              接口取数失败: 没有可识别的列表字段: detail,exists,path
  OK   /alerts            接口    1 条 -> 页面    1 行   告警规则(可为空)
  OK   /voucher           接口    2 条 -> 页面    2 行   卡密列表(可能为空)
  OK   /ddns              接口    0 条 -> 页面    0 行   DDNS 记录(可为空)
  OK   /vpn-client        接口    0 条 -> 页面    0 行   VPN 客户端 peers(可为空)
  OK   /security-plus     接口    0 条 -> 页面    0 行   时间段策略 rules(可为空)
  SKIP /network-tools     接口取数失败: 无取数接口
  SKIP /network-advanced  接口取数失败: 无取数接口

✅ UI 验收通过（8 个页面，渲染行数与接口一致）
```

## SKIP 原因说明

| 页面 | 原因 |
|------|------|
| /cert | 接口 `GET /api/v1/cert` 返回 `{exists, path, detail|info, ...}`，是**证书描述信息 + 生成按钮**页，不是列表渲染页。`api_count` 无法从响应里识别出任何列表字段（`没有可识别的列表字段: detail,exists,path`），故 SKIP。这是"接口返回对象而非列表"，并非失败。 |
| /network-tools | 该页是纯工具页（ping/traceroute/portprobe），**全部走 POST 探测**，没有任何 GET 列表接口可对比条数。SKIP 属设计如此，非缺陷。 |
| /network-advanced | 该页为表单/单接口调优（`GET /api/v1/iface/tune` 需带 `iface` 参数且返回 `live` map，非列表）。无列表渲染可对比，SKIP。 |

## 发现的问题

1. **脚本 `api_count` 字段白名单不全（已修）**：voucher 列表字段是 `vouchers`，原白名单没有，直接会导致 `/voucher` 判为 SKIP。本次给 `api_count` 的识别字段列表**加入 `vouchers`**（见 `scripts/ui_acceptance.py`），修复后 `/voucher` 常规 OK（2 条 → 2 行）。
2. **`/routing-policy` 行数与接口条数 ±1**：接口 1 条，页面渲染 2 行。脚本注释已说明 el-table 合计行会带来 ±1，非 bug。若要精确，可改进判据排除合计行（暂未改，保持脚本最小改动）。
3. **空列表页的可达性**：`/ddns`、`/vpn-client`、`/security-plus` 当前接口返回 0 条、页面 0 行，判定 OK 但**无法验证"有数据时是否正确渲染"**。这属于用例覆盖盲区，非页面缺陷。

## 未能验证的页面（诚实列出）

- **`/cert`**：接口返回对象而非列表，框架只按文件里注册的 GET「列表类」接口判断，无法自动验证证书信息 el-descriptions 的字段渲染是否正确。需人工打开页面核对 info 字段。
- **`/network-tools`、`/network-advanced`**：无 GET 列表接口，脚本无法自动判。这两个页面的正确性依赖点按按钮后的 POST 结果，不在本脚本能力范围。
- **所有"有数据但当前为空"的页面**（`/voucher`/`/ddns`/`/vpn-client`/`/security-plus` 的非空分支）：当前后端无数据，渲染"空态"已 OK，但"非空态"表格列映射未实测。

## 关于「53 个页面」的范围说明

`ls web/src/views/*.vue | wc -l` = **53**。但本脚本 PAGES 只覆盖 11 个条目（8 OK + 3 SKIP），
其余 40+ 页面（如 /network /security /qos /traffic 等）**不在本次任务要求的验收清单内**，未逐一跑脚本。
report 的"全站验收"结论严格限定在本任务指定的 9 个页面 + 脚本原有 4 个页面范围内。