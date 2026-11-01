# EasyTier 去中心化组网（router-re）

EasyTier 是一个去中心化的 mesh VPN：节点之间自动打洞组网，不需要中心服务器。
本功能把它接进 router-re 的「虚拟专网」页，作为与 PPTP / L2TP / OpenVPN 并列的一个标签。

## 位置

- 后端包：`mgmtd/internal/easytier/`（config.go 渲染与校验、process.go 进程生命周期、parse.go 节点表解析）
- 后端接口：`mgmtd/internal/server/easytier.go`，路由注册在 `server.go` 的 OpenVPN 区块之后
- 前端：`web/src/views/VpnServers.vue` 的「EasyTier 组网」标签

## 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/vpn/easytier/config` | 配置 + 运行态 + 能力（binary_exists / tun / privileged / version）。密钥不回显，返回 `secret_set` |
| POST | `/api/v1/vpn/easytier/config` | 保存并立即生效；密钥留空 = 沿用已保存的 |
| POST | `/api/v1/vpn/easytier/preview` | 只渲染 TOML，不落盘不启动；密钥打码成 `********` |
| GET | `/api/v1/vpn/easytier/status` | 运行态 + 对等节点列表（未运行时 `peers` 是 `[]` 不是 `null`） |
| POST | `/api/v1/vpn/easytier/stop` | 停止实例，保留配置 |

错误码：12010 json 解析失败、12011 配置校验失败、12012 保存失败、12013 停止失败、12014/12015 预览的解码/校验失败。

## 配置与运行

- 配置文件默认 `/etc/easytier/config.toml`（`MGM_EASYTIER_CONFIG` 可覆盖），0600、tmp+rename 原子写。
- 二进制默认 `easytier-core` / `easytier-cli`（`MGM_EASYTIER_BINARY` / `MGM_EASYTIER_CLI` 可覆盖）。
- 进程日志默认 `/var/log/router-re/easytier.log`（`MGM_EASYTIER_LOG`），启动失败的原因从日志尾部回传给接口。
- 参数：`easytier-core -c <config> -r <rpc_portal>`。

### 两个必须知道的坑

1. **2.6.4 的配置文件不认 `rpc_portal`**。写进 TOML 会被静默丢弃（日志里回显的解析结果没有这一项），
   节点仍监听默认 15888；此时 `easytier-cli -p <配置里的端口>` 连不上，状态页永远 0 个节点。
   必须用命令行 `-r` 显式传（命令行覆盖配置文件）。这条是靠 E2E 里「进程在跑但 peers=0」发现的。
2. **建 TUN 需要 CAP_NET_ADMIN**。非 root 运行时 `/dev/net/tun` 可读但 ioctl 会 EPERM，
   所以接口返回 `privileged=false`、`tun=false`，前端提示"只能作为中继"，而不是让用户点了启用才看到报错。

## 实测证据

- 单测：`go test ./internal/easytier/` —— 渲染（含转义/空 IPv4 不渲染）、校验（换行注入、IPv6 混入、端口越界、密钥过短等 10 个反例）、
  真实 `easytier-cli` 输出解析（2 行、列错位、ANSI 颜色码、非表格输出）。
- 双节点真隧道：两个 netns 各跑一个 easytier-core，`peer` 表显示 p2p/tcp、延迟 ~0.9ms，
  双向 ping 3/3 通、0% 丢包（同 netns 会因本地投递失败，必须分 netns）。
- 接口 E2E：`python3 scripts/e2e_easytier.py`，12 PASS / 0 FAIL，报告 `docs/e2e-easytier.md`。
- 界面：真实浏览器打开「虚拟专网 → EasyTier 组网」，标题/字段/运行状态 tag/节点表格均渲染，
  且不再残留 PPTP 的「服务端地址 / 地址池 / 拨号账号」。

## 使用要点

- 所有节点必须 `network_name` + `network_secret` 一致，否则不同网络。
- 没有公网入口的节点，填一个公共中继（如 `tcp://public.easytier.cn:11010`）作为探路入口。
- 想把某个局域网导出给其他节点：填「代理网段」，如 `192.168.1.0/24` 或 `10.0.0.0/24->192.168.50.0/24`。
- RPC 管理地址保持 `127.0.0.1:xxx`；它等于节点管理权，暴露到公网是事故。
