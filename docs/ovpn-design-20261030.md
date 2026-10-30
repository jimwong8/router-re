# OpenVPN 服务端：接线设计（等 pki/status 两包落地后执行）

目标：路由器管理面能开一个可用的 OpenVPN 服务端（UDP 1194 起），签发客户端证书、
导出 `.ovpn`、看在线客户端，并在页面上启停。**不自己实现 OpenVPN 协议**——
用系统 openvpn 进程，mgmtd 负责证书、配置渲染、服务控制、状态读取。

## 组件分工

    internal/ovpn/pki.go     证书体系（工人 ovpn-pki-01 产出）
    internal/ovpn/status.go  management 文本解析（工人 ovpn-status-01 产出）
    internal/ovpn/server.go  本设计：配置渲染 + 进程/服务控制 + 状态聚合（指挥位）
    internal/server/ovpn.go  HTTP handler + 路由注册（指挥位）
    web/src/views/VpnServer.vue  页面（指挥位）

## 运行参数（先定死，避免接线时再改）

    PKI 目录        /var/lib/router-re/ovpn/pki/        CA、server.crt/key、clients/<CN>.crt|key
    ���务端配置      /var/lib/router-re/ovpn/server.conf  由模板渲染，不手改
    持久化状态      /var/lib/router-re/ovpn/state.json   enabled/port/proto/subnet/dns/重定向开关
    management      unix:/run/router-re-openvpn.sock    mgmtd 只读它拿状态
    日志            /var/log/router-re/openvpn.log

默认参数（可在 state.json 覆盖）：

    proto   udp
    port    1194
    dev     tun0
    subnet  10.8.0.0/24
    dns     由 DHCP 下发，默认取「客户端所在段」的网关，空则不下发
    redirect-gateway   默认关闭（开了会把客户端全部流量引到路由器，需明确提示）

## 配置渲染要点

`server.conf` 用模板渲染，每个会被用户改的值都要做**白名单校验**：port 1-65535、
proto ∈ {udp,tcp}、subnet 必须是合法 CIDR 且 /16../30、DNS 必须是 IP 字面量。
**拼字符串进配置文件等于命令注入**——CN 里出现换行就能往 conf 里塞指令，
所以 client CN 入库前必须匹配 `^[A-Za-z0-9_.-]{1,64}$`。

模板里必须显式写：

    management /run/router-re-openvpn.sock unix
    management-client-user root
    status-version 3
    keepalive 10 60
    persist-key / persist-tun
    user nobody / group nogroup（rendered 但在容器里没有权限时跳过，见下）

## 服务控制

不复用系统 `openvpn@.service`，由 mgmtd 直接管进程（`exec.Command` + `SysProcAttr`），
PID 落 `state.json`。理由：路由器的启停要能被 mgmtd 完全观测，systemd 单元的状态
只能靠猜，而且渲染出的 conf 一变就得多一次 daemon-reload。

启动后必须等两件事再算成功：① `ss -lun | grep :1194` 有监听；② management socket
能 `status` 出回包。任一超时 5s 就判定启动失败并把 `openvpn.log` 尾部 20 行带回去——
只回 `started` 而进程其实秒退，是这类功能最常见的假成功。

## HTTP 接口

    GET  /api/v1/vpn/openvpn/status     {enabled,running,port,proto,subnet,up_since,
                                         clients:[{cn,real_addr,virtual_addr,bytes_in,bytes_out,connected_since}],
                                         ca_ready,pki_dir}
    POST /api/v1/vpn/openvpn/config     改端口/协议/网段/DNS/redirect；改了要提示需重启
    POST /api/v1/vpn/openvpn/enable     {enabled:true|false} 启停（异步 3s 内回状态）
    GET  /api/v1/vpn/openvpn/clients    已签发证书列表（CN/序列号/到期）
    POST /api/v1/vpn/openvpn/clients    {common_name} 签发新客户端，返回一次性下载凭据
    GET  /api/v1/vpn/openvpn/clients/{cn}/profile   生成 .ovpn（inline cert+key+CA）
    DELETE /api/v1/vpn/openvpn/clients/{cn}         吊销：写 CRL 并断开该客户端

错误码沿用项目规则从 10480 起接续，用前先 grep 现有最大号。

## 验收（缺一不可）

1. `openvpn --version` 存在；不存在时接口明确回「openvpn 未安装」而不是 500。
2. 签发 client → 拿到 .ovpn → 在本机用 `openvpn --config x.ovpn --daemon` 真连上，
   `ip addr` 出现 tun 且能 ping 通网关 10.8.0.1。
3. `/status` 里能列出该客户端，字节数在涨。
4. 断开后 `>CLIENT_LIST` 消失，状态回到 0 客户端。
5. 关掉服务后端口释放，重新 enable 能起来（幂等）。
6. 把 conf 里 port 改成被占用端口 → enable 失败，且返回的日志尾部能看出 bind 失败，
   而不是"成功"。

## 已知环境约束

- 目标机 10.100.1.132：openvpn 已装（`/usr/sbin/openvpn`），但 **`/dev/net/tun` 打不开
  （EPERM，容器 device cgroup 未放行）**，`ip tuntap add` 同样失败。所以 132 上只能验
  「渲染 + 启动尝试 + 明确报错」，**不能**声称隧道可用。
- 真实验收换到 **10.100.1.18**（本机，有 tun，tailscale 依赖它）：服务端监听 1194，
  客户端连 `127.0.0.1:1194`，两端各起一个 tun（tun0/tun1），`ping 10.8.0.1` 走内核回环
  即可闭环——不需要第二台机器。
- 132 上已有 9090 的 mgmtd 在跑，重启必须用 `pkill -TERM -x mgmtd-132` + setsid 拉起，
  不能 `pkill -f`（会杀掉调用它的 shell）。

## 状态（2026-10-30）

    pki.go / status.go   两个工人在写，接到即评审
    接线（server.go/ovpn.go/VpnServer.vue）  等两包绿了再做，避免接口猜测

