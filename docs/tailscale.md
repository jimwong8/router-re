# Tailscale 接入（router-re）

Tailscale 与 OpenVPN / EasyTier 的实现方式不同：**不托管进程**。tailscaled 由系统 systemd 管理，
router-re 只通过它的 unix socket 下发 `tailscale up / down` 并读取真实状态。

## 位置

- 后端包：`mgmtd/internal/tailscale/`
  - `config.go`：Config / Validate / BuildUpArgs / MaskArgs / socket 与二进制路径解析
  - `status.go`：`status --json` 解析、`Apply`（up/down + 回读状态）
- 接口：`mgmtd/internal/server/tailscale.go`，路由注册在 `server.go` 的 EasyTier 区块之后
- 前端：`web/src/views/VpnServers.vue` 的「Tailscale」标签

## 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/vpn/tailscale/config` | 配置 + 真实状态 + 能力（binary_exists / socket / socket_exists / system_socket / up_args_preview） |
| POST | `/api/v1/vpn/tailscale/config` | 保存并立即执行 up 或 down；auth key 留空 = 沿用已保存的 |
| POST | `/api/v1/vpn/tailscale/preview` | 只回 `up` 参数（authkey 打码），不动守护进程 |
| GET | `/api/v1/vpn/tailscale/status` | 运行态 + 节点列表（在线优先排序） |
| POST | `/api/v1/vpn/tailscale/down` | 断开本节点 |

错误码：12020 JSON 解码、12021 配置校验、12022 保存失败、12023 down 失败、12024/12025 预览解码/校验。

## 判定"生效"的唯一标准是 BackendState

`tailscale up` 退出码 0 不代表接入了网络：key 无效/过期时它会停在 `NeedsLogin`。
所以 `Apply` 执行完必须回读状态，只有 `BackendState == "Running"` 才算成功，其余情况把 CLI 原话带回接口，例如：

    节点需要授权（未返回授权链接，多半是 auth key 无效或已过期）：backend error: invalid key: API key does not exist

## 安全设计

- **socket 可配置**：`MGM_TAILSCALE_SOCKET`（默认 `/var/run/tailscale/tailscaled.sock`）。
  接隔离实例做测试时必须覆盖，否则动的是本机自己的 Tailscale。
- **系统 socket 警告**：`IsDefaultSocket()` 为真时接口返回 `system_socket=true`，
  页面顶部黄条提示"点断开连接会切断本机 Tailscale，包括正在使用的远程管理通道"，前端点断开还有二次确认。
- **auth key 不回显**：GET 只返回 `key_set`；预览里打码成 `tskey-********`；保存时留空表示不修改。
- **参数注入防护**：auth key / 主机名 / 控制面地址都拒绝换行与控制字符（它们会作为进程参数出现）。

## 实测证据

`python3 scripts/e2e_tailscale.py` → 15 PASS / 0 FAIL（报告 `docs/e2e-tailscale.md`）：

- 隔离实例（userspace 网络 + 独立 socket，:9096）：能力探测、参数预览与打码、假 key 被真实拒绝并如实回报 `applied=false`、
  非法主机名 422/12021、down 生效且状态仍可读。
- 系统实例（:9097，只读）：`system_socket=true`、`BackendState=Running`、
  **peer_count=75 / online=47~48**、`Self` 地址 `100.111.28.64` + IPv6 —— 真实 75 个节点的解析与展示跑通。
- 界面：浏览器打开页面，标题含 Tailscale、系统 socket 黄条、状态 tag `Running`、描述区（网络 `jimwong8.github` / 本机地址 / 在线 48/75）、
  75 行节点表格；截图经视觉模型核对列对齐、无溢出。

## 过程中修掉的两个缺陷

1. 加标签后 PPTP/L2TP 的公共表单（服务端地址、地址池、拨号账号）会一起渲染 ——
   公共块的条件从 `proto !== 'easytier'` 改成同时排除 `tailscale`。
2. 页头「服务状态」在 Tailscale 标签下恒为 `inactive` —— 现在随 `BackendState` 同步（Running → active）。

## 已知限制

- 需要有效 auth key 才能真正加入网络；测试里用假 key 只能验证"失败路径真实可信"，不能验证真实入网（那需要账号凭据）。
- 只支持 IPv4 通告子网（`--advertise-routes`），IPv6 子网未做校验分支。
- 只对接本机 tailscaled；对"远端设备"的管理仍走 Tailscale 管理台。
