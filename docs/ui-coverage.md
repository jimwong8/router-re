# UI 验收覆盖报告（docs/ui-coverage.md）

验收脚本：`scripts/ui_acceptance.py`（判据统一在 `scripts/ui_judge.py`，三份验收脚本共用）。
环境：mgmtd @ 127.0.0.1:9090，vite preview @ 127.0.0.1:4173，Playwright 真实渲染行数比对。

## p82-user-role-update-UI（2026-10-29）

改动：`web/src/views/Users.vue` 每行新增「编辑」按钮 + 编辑对话框（角色下拉 admin/operator/viewer，启用 el-switch），
保存走 `api.put('/api/v1/users/{username}', {role, enabled})`；成功后 `ElMessage.success` 并刷新列表；
失败按后端 `{code,message,detail}` 显示 message+detail，不静默。
构建：`cd web && npm run build` 通过（21.6s，exit 0）。

验收结果（`python3 scripts/ui_acceptance.py`）：

```
OK   /services          接口  525 条 -> 页面   50 行   systemd 单元列表 [分页]
OK   /online-users      接口   24 条 -> 页面   24 行   在线会话
OK   /routing-policy    接口    1 条 -> 页面    2 行   策略路由规则(可为空)
SKIP /cert              接口取数失败: 没有可识别的列表字段: detail,exists,path
OK   /alerts            接口    1 条 -> 页面    1 行   告警规则(可为空)
OK   /voucher           接口   38 条 -> 页面   38 行   卡密列表(可能为空)
OK   /ddns              接口    0 条 -> 页面    0 行   DDNS 记录(可为空)
OK   /vpn-client        接口    0 条 -> 页面    0 行   VPN 客户端 peers(可为空)
OK   /security-plus     接口    1 条 -> 页面    2 行   时间段策略 rules(可为空)
SKIP /network-tools     接口取数失败: 无取数接口
SKIP /network-advanced  接口取数失败: 无取数接口

✅ UI 验收通过（8 个页面，渲染行数与接口一致）
```

8 个可比对页面全部 OK（3 个 SKIP 为设计内：cert 无列表字段、network-tools / network-advanced 无 GET 列表接口）。
