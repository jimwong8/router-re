# iKuai 远程管理中心（爱快云）研究

**对象**：本机实测设备 `Xiaomi_DNSA`，iKuai **4.0.308 免费版**，GWID `8c8ece9be485473f881c2cec1f243727`，
固件样本 iKuai8_x64_4.0.314_beta（rootfs 已解密，`work/mnt/`）。
**日期**：2026-10-31　**方法**：设备 Web 只读实测 + 固件二进制动静态分析 + 云端点真实 TLS/HTTP 探测。

---

## 0. 结论速览

| 问题 | 结论 | 证据 |
|---|---|---|
| 远程管理是什么形态 | 设备**主动向云平台建一条常驻连接**，云平台把浏览器请求**原样塞进这条连接**打到设备本地 Web | `rest_ctrl_req.req` 是 **BYTES**（原始 HTTP 请求字节）；`ik_rc_client -b/-p` 指定本地 backend；日志 `entry,host_or_path:%s,server_port:%d,sid:%u` |
| 云端点 | `as-v4.ikuai8.com:9444` → `123.56.171.79`（北京阿里云），服务端 **openresty/1.19.3.1** | 实测 DNS + TCP + HTTP 响应头 |
| 传输层 | TLS1.2 `ECDHE-RSA-AES256-GCM-SHA384`，服务端自签证书 `CN=*.ikuai8.com`（2019-08-22 → 2029-08-19，issuer=subject） | `openssl s_client` 实测 |
| 是否强制客户端证书 | **是，但不在 TLS 层**：不带证书握手能完成，HTTP 请求被 400 拒绝 `No required SSL certificate was sent` | 实测响应正文（nginx 语义） |
| 应用层 | HTTP/1.1 长连接，`202 Accepted` 表示"接受并保持"，媒体类型 `application/vnd.ikuai8.remote-access.v1+json` | 二进制字符串 + 上一轮反汇编 |
| 消息编码 | protobuf-c（6 种消息）→ **AES-256-CBC 再加密**（密钥由 `rclink_set_key` 的 key 经 EVP_BytesToKey/SHA1/count=5 派生） | descriptor 全表（本文 §3）+ `rc_aes_new` 反汇编 |
| 身份 | `GWID`(32 hex) + `token` + 客户端证书。**证书在固件里**（加密资源 `map8`/`map9`，开机由 `libwpa_verify.so` 解出）；真正运行时下发的是**证书私钥口令**与 **AES 会话密钥** | `maps_dec/map8.dec`(client.crt, OU=ikclient) / `map9.dec`(client.key, DES-EDE3-CBC)；`des3_conclusion.md` 结论：`key_passwd` 来自 AS 服务器 JSON |
| 复用性 | 一条 TCP 连接上多路复用多个 HTTP 会话（sid） | 日志 `remote ctrl request, sid:%u` / `Receive sid %lu session close msg` |

一句话：**爱快云 = 带 mTLS 的常驻反向 HTTP 隧道 + protobuf 控制面 + 本地 Web 代理**。
它没有内网穿透打洞、没有 P2P、没有设备间组网——远程"管理"实质是"云端替你访问设备自己的 Web/API"。

---

## 1. 设备侧实测（只读）

登录 `10.100.1.1` 看 **设备设置 → 云服务绑定**：

| 字段 | 实测值 |
|---|---|
| 节点服务器 | 中国 |
| 路由 ID | `8c8ece9be485473f881c2cec1f243727`（= 概览页 GWID，32 hex） |
| 绑定码 | `a23c…581b`（32 hex，**已脱敏**）。页面同时提供「一键换绑」，但**没有**明确的「已绑定/解绑」指示，所以绑定状态只能从备注名已有值推测为曾绑定 |
| 备注名 | 台湾城 |
| 页面自述 | "绑定后可实现**多路由设备集中管理、远程运维**及解锁订阅服务" |
| 服务码 | 可获取（联系官方技术支持用） |

补充：设备版本 4.0.308、运行 23 天、在线终端 114。GWID 由 `/etc/setup/rc.console: get_device_gwid()` 从 release 文件取
`GWID=` 并转小写、校验 32 hex，取不到时用全 0 兜底。

---

## 2. 云侧实测（从本机 ens18 直连，绕过本机 Tailscale 的 table 52 默认路由）

```
DNS   as-v4.ikuai8.com → 123.56.171.79        （as1.ikuai8.com→101.200.55.4，as2→123.56.221.14/39.96.76.173）
TCP   123.56.171.79:9444  OPEN  0.0s
TLS   TLSv1.2  ECDHE-RSA-AES256-GCM-SHA384   （无客户端证书，握手成功）
证书   subject = C=CN, ST=beijing, O=ikuai, OU=ik, CN=*.ikuai8.com
       issuer  = 同 subject（自签），有效期 2019-08-22 → 2029-08-19
HTTP   POST /  (Content-Type: application/vnd.ikuai8.remote-access.v1+json, 无客户端证书)
       → HTTP/1.1 400 Bad Request
         Server: openresty/1.19.3.1
         <title>400 No required SSL certificate was sent</title>
```

**三个可直接落地的结论**：

1. **mTLS 的强制点在应用层**。TLS 层 `ssl_verify_client optional`（否则握手就断），
   openresty 在 HTTP 层检查 `$ssl_client_verify`，没有证书直接 400。要复刻只需一行
   `if ($ssl_client_verify != SUCCESS) { return 400 "No required SSL certificate was sent"; }`。
2. **自签 10 年 CA**，所以固件必须自带 `ca-certificates.d/ikuai` 做 pinning——公信 CA 用不上。
3. 老版本兜底 `as1/as2.ikuai8.com:9443`，4.0 起走 `as-v4…:9444`（端口从 9443 迁到 9444）。

> 旁证：本机 `ip route get 123.56.171.79` 命中 **table 52 的 `default dev tailscale0`**，
> 直连会超时；用 `SO_BINDTODEVICE=ens18` 才通。这再次印证了 table 52 默认路由劫持问题（见 router-re 既有记录）。

---

## 3. 客户端与消息结构（本轮补全）

### 3.1 进程与依赖

- `/usr/ikuai/script/rc: start_remote_services()` 在开机时直接拉起 `ik_rc_client`（无参数，配置走 `/etc/remote2/`）。
- `ik_rc_client` → `librc_core.so`（`rclink_*` 全在这里实现），依赖 `libprotobuf-c`、`libcurl`、`libssl/libcrypto`、`libjansson`、glib。
- CLI 选项（`strings` 实证）：`-c 配置路径`、`-C 配置文件`、`-b backend 主机`、`-p backend 端口`、`-l 日志级别`、`-L 日志路径`、`-k 锁文件`。
- 进程模型：`start up %d workers`（多 worker 处理并发会话），`Get lock failed or another ik_rc_client is running` 防双开。

### 3.2 六个 protobuf 消息（descriptor 全表）

用 `work/bootchain/ikrc_descriptors.py`（本轮新写）从 `.so` 的 `*__descriptor` 符号直接解出：

| 消息 | 字段 | 说明 |
|---|---|---|
| `auth_req` (48B) | `gwid`(1,string) `token`(2,string) `keepalive_intvl`(3,uint32) | 设备 → 云认证 |
| `auth_resp` (48B) | `gwid` `token` `code`(3,uint32) | 认证结果码 |
| `rest_ctrl_req` (40B) | **`req`(1, BYTES)** | 云端下发的**原始 HTTP 请求字节** |
| `rest_ctrl_resp` (40B) | **`resp`(1, BYTES)** | 设备回的**原始 HTTP 响应字节** |
| `disconn_req` (40B) | `gwid`(1) `token`(2, optional) | 主动断开 |
| `session_end` (32B) | `gwid`(1) | 会话结束通知 |

**本轮纠正上一版**：descriptor 头里 `sizeof_message` / `n_fields` 是 **u64**（偏移 0x28 / 0x30），
`fields` 指针在 **0x38**，field descriptor 步长 `0x48`。上一版按 u32 读会得到 `n_fields=0`。

### 3.3 分层

```
┌──────────────────────────────────────────────┐
│ 浏览器（爱快云 Web / 小程序）                  │
└───────────────┬──────────────────────────────┘
                │  云平台 → 设备：一条常驻连接
┌───────────────▼──────────────────────────────┐
│ TLS 1.2 (自签 CN=*.ikuai8.com)                │  ← 传输层：服务端证书；客户端证书在 HTTP 层校验
├──────────────────────────────────────────────┤
│ HTTP/1.1 长连接，202 Accepted                 │  ← Content-Type: application/vnd.ikuai8.remote-access.v1+json
├──────────────────────────────────────────────┤
│ protobuf-c 消息（auth / rest_ctrl / …）        │
├──────────────────────────────────────────────┤
│ AES-256-CBC（EVP_BytesToKey/SHA1/count=5）    │  ← 应用层再加一层（密钥=rclink_set_key 的 key）
└───────────────┬──────────────────────────────┘
                │ sid 多路复用
┌───────────────▼──────────────────────────────┐
│ ik_rc_client：把 rest_ctrl_req.req 当 HTTP 请求 │
│ 转发到本地 backend（默认 127.0.0.1:80，openresty）│
│ 再把响应原样封进 rest_ctrl_resp.resp            │
└───────────────┬──────────────────────────────┘
                │
┌───────────────▼──────────────────────────────┐
│ iKuai 本地 Web / API（同一套页面，云端没有另开接口）│
└──────────────────────────────────────────────┘
```

关键实现细节（字符串实证）：
`connect back end failed,sid(%u)`、`connect is closed by back end,sid(%u)`、`create http proxy failed for sid(%u) of link(%p)`、
`entry,host_or_path:%s,server_port:%d,sid:%u`、`End send request(%d:%d) by sock %d for sid:%u`、`Receive sid %lu session close msg`。
即：**每个 sid = 一条到本地 Web 的 HTTP 会话**，同一条隧道上并发多个，彼此独立关闭。

### 3.4 时序

```
设备                                       云 (as-v4:9444)
 │ 1. mTLS 建连（带 client.crt）             │
 │ 2. POST auth_req{gwid,token,keepalive}    │
 │ 3.                                   ◀────│ auth_resp{code=0}
 │ 4. 保持连接；按 keepalive_intvl 保活       │  (5–300s)
 │ 5.                                   ◀────│ rest_ctrl_req{req=<HTTP 请求>}   sid=1
 │ 6. 连本地 127.0.0.1:80 转发该请求          │
 │ 7. ─────────────────────────────────────▶│ rest_ctrl_resp{resp=<HTTP 响应>} sid=1
 │ 8.                                   ◀────│ session_end / disconn_req
 │ 9. token 失效 → 退避重试；失联 → 回调降级   │
```

---

## 3.5 更正（交叉核对既有逆向资料后）

本节之前的版本有两处需要更正，均以 `work/bootchain/des3_conclusion.md`、`pmd_analysis.md` 与 `maps_dec/` 实测为准：

1. ~~证书不在固件里~~ → **客户端证书与私钥都在固件里**：
   `map8.dec` = `client.crt`（`C=CN,ST=beijing,O=ikuai,OU=ikclient,CN=*.ikuai8.com`，2019-08-22→2029-08-19，由自签 CA `OU=ik` 签发），
   `map9.dec` = `client.key`（2048 位 RSA，`DES-EDE3-CBC` 加密）。它们由 `libwpa_verify.so` 用**硬编码口令 `bcfae016-ea75-4569-b9e6`** 从
   `/usr/ikuai/enc/map*.enc` 解出后落到 `/etc/remote2/ca-certificates.d/ikuai/`。
2. **运行时下发的不是证书，而是两把密钥**：`client.key` 的 DES3 口令 `key_passwd`（喂 libcurl `CURLOPT_KEYPASSWD` 0x272a）与
   AES 会话密钥（`rclink_set_key`），都由 AS 服务器下发的 JSON 提供，镜像里没有硬编码、全镜像暴力扫描未命中。

安全含义：**云凭证可以离线从固件里还原**（CA 口令硬编码），安全性实际只剩"运行时 key_passwd + token 可撤销"这一层。

---

## 4. 它到底提供了什么能力（和实现对应）

| 云平台能力 | 实现方式 | 说明 |
|---|---|---|
| 远程 Web 管理 | `rest_ctrl_req` 代理 HTTP | 就是"云端浏览器 ↔ 设备 Web"，不需要设备有公网 IP |
| 多设备集中管理 | 云侧按 GWID 索引设备列表 | 设备主动连入，云侧维护在线状态（保活间隔） |
| 远程重启/升级 | 云端下发控制请求打到本地管理接口 | 与 Web 管理同一套 API |
| 告警/订阅 | 云侧 + 设备侧 `IKCTRL_EVENT_*` 事件 | `WAN_IP` 变化、`CONFIG_CHANGED` 会主动上报 |
| 服务码/技术支持 | 设备生成服务码，官方凭码接入 | 独立于云绑定的旁路通道 |

**它没有的**：内网穿透给第三方、P2P 组网、设备间互通、边缘计算。这些要用 EasyTier / Tailscale / WireGuard。

---

## 4.5 补充：云端不是一个端点，而是三个分工端点（本轮实测）

| 端点 | IP | 端口 | 协议 | 角色 |
|---|---|---|---|---|
| `as-v4.ikuai8.com` | 123.56.171.79 | 9444 | **HTTPS/mTLS**（openresty），TLS1.2 | 接入服务器：认证 + 远程管理隧道（本文主体） |
| `pkgmgr-v4.ikuai8.com` | 123.56.230.35 | 1864 | **裸 TCP 自定义协议**（AES + JSON，无 TLS、无 banner） | `pmd` 包/资源管理：下发 `map*.enc`、固件/应用更新 |
| `dis-v4.ikuai8.com` | 59.110.171.18 | 1854 | **裸 TCP 自定义协议** | `cre` 分发/内容通道（脚本里 `cre_client`，仅 `OEMNAME=zh` 启动） |

实测方法：`SO_BINDTODEVICE=ens18` 直连（绕开本机 tailscale table 52）。1864/1854 **TCP 能建连但 TLS ClientHello 立刻被断
（`UNEXPECTED_EOF_WHILE_READING`），发 HTTP 也直接 EOF** —— 说明这两个是二进制私有协议，不是 HTTP 服务。
`pmd` 二进制里有 `aes-128-cbc` / `aes-256-cbc` / `bad json array` / `begin to download %s from cdn` / MD5 校验；
`cre` 里有 `Failed to get gwid at 2.0 router` / `Failed to get token file %s` —— 同样是 GWID+token+JSON+AES 那一套。

**结论**：iKuai 的"云管理"是**控制面（9444 隧道）+ 资源面（1864/1854 私有协议）**分离的。
自建方案不要把它们混在一个通道里：资源下发有独立的分发/校验（MD5、签名）需求。

### 4.5.1 第二条"可下发指令"的链路：pmd 的资源投放

`/usr/ikuai/enc/map1..11.enc` + `/usr/ikuai/sig/map1..11.sig`，开机由 `libwpa_verify.so: verify_files()` 解密投放：
`map*.enc` 是 OpenSSL `enc` 格式（`Salted__`+salt+AES-256-CBC，口令硬编码），`map*.sig` = `SHA256(明文)[32] ‖ RSA-2048 签名(256) ‖ 00 01 00 00`。
其中 `map1`→`upgrade.sh`、`map2`→`rc.console`、`map4`→`/etc/passwd`、`map7`→`remote2/ikuai.conf`、`map8/9`→云管理证书、`map10`→`ik_rc_client`、`map11`→`download.lua`。

也就是说 **pmd + map 资源链**是独立于 9444 隧道的第二条"能替换设备上文件"的通道，第三方 debloat 脚本 `01-remove-backdoor.sh`
清理的就是这一整套：`pmd`(→`pmd.ikuai.com`)、`ik_rc_client`、`cre`、`submit.lua`、`remote2`、`/etc/init.d/cloud1002`。
注意：后三者在本 4.0.314 rootfs 里**不存在**（`usr/sbin/remote2`、`usr/bin/submit.lua`、`etc/init.d/cloud1002` 均 MISSING），
它们是老版本组件或运行时生成，**不要照抄到新方案里当现状**。

---

## 4.6 实测：1854/1864 私有帧格式（已完全解开）

做法：**独立 netns（只有 lo）+ chroot（固件自带 uClibc）**跑厂家的 `cre` / `pmd`，`-s 127.0.0.1:<port>` 指向本地假服务器，
再加 gdb 断在 `EVP_EncryptInit_ex` 上取 key/iv——一次运行里**同时拿到密文帧和密钥**，两边一对就闭环了。
不出网、不接触真实云端。脚本 `capture_{cre,pmd}.sh`、`pmd_sync.gdb`、`ikrc_decode_frame.py`，证据在 `samples/`。

### 4.6.1 帧结构（两条通道同构）

```
0        16       20                       20+N            20+N+4
+--------+--------+-----------------------+---------------+
| 头 16B | 常量4B | AES-256-CBC 密文 N B  | CRC32(密文) 4B|
+--------+--------+-----------------------+---------------+
                     总长 = N + 24
```

头 16 字节：

| 偏移 | 字段 | pmd(1864) | cre(1854) |
|---|---|---|---|
| 0 | 产品/模块号 | `01` | `02` |
| 1 | 版本 | `01` | `02` |
| 2 | 命令 | `01` | `01` |
| 3 | 保留 | `00` | `00` |
| 4..7 | 消息序号（BE32） | `00000001` | `00000000` |
| 8..11 | 标志位 | `ffffffff` | `00ffffff` |
| 12..15 | **密文长度（BE32）** | `000001f0`(496) | `000000a0`(160) |
| 16..19 | **消息常量** | `c7fb4bc1`（login） | `80d811f7` |
| 末尾 4B | **CRC32(密文) 大端** | `89ed26b5` | `9e8dc222` |

实测：`crc_ok = True`、PKCS#7 填充合法、`trailing = 0`（解码器 `ikrc_decode_frame.py` 会逐项打印）。

### 4.6.2 加密：AES-256-CBC + 固定 ASCII 密钥

两条通道各用一套 key/iv，都是从 `EVP_EncryptInit_ex` 的参数里直接读出来的，**跨运行完全不变**：

| 通道 | KEY(32B) | IV(16B) |
|---|---|---|
| pmd(1864) | `qgzdcnijopiptqtsrrpitrbczjQooRev` | `)w2Duts:I91mx?]q` |
| cre(1854) | `kAvzrx<y$&ptMb4tzeawad1vl2wNjak9` | `xl)4Yi9Av6]P6_ys` |

四段全是可打印 ASCII 串（不是 MD5 派生的二进制摘要），说明这是厂家自己生成的固定密钥串，
即 **key 硬编码在二进制/资源里、IV 固定、同一消息每次加密结果完全相同**。
（`EVP_BytesToKey(EVP_md5, count=1)` 那条分支是"key 未就绪时"的兜底路径，实际走的是预置 key 分支。）

这带来两个直接后果：**同一条 login 报文可被离线解密、可在网络中比对识别**。

### 4.6.3 完全解密后的明文（pmd → 1864）

```json
{"method": "login", "gwid": "",
 "aux_info": {"device_type": "router", "mac": "null", "arch": "x86_64", "model_type": "X86",
              "model_name": "", "sysbit": "x64", "mtd_type": "", "boot_guide": "", "boot_hdd": "",
              "etc_log": 18148512, "rootfs": 18148512, "tmp": 18148512,
              "memfree": 2101704, "memtotal": 65419740},
 "data": {"basic_info": {"enterprise": "", "oem_name": "", "firmware": "IK-RouterOS",
                         "version": "4.0.314", "gwid": ""},
          "packages_list": [{"name": "pmd", "version": "120"}]}}
```

与 `pmd` 自己 debug 日志里 dump 的 JSON 逐字符一致 —— 解密链路可自证。

### 4.6.3.1 cre → 1854 的明文（不是 JSON，是 `key:value` 行文本）

```
method:GetRules
gwid:
arch:x86_64
mac:null
router_ver:4.0.314
rule_ver:null
uptime: 20:35:00 up 23 days, 20:10,  load average: 6.83, 6.31, 6.41
```

即设备每轮问"要不要更新规则"，顺带把 **架构、MAC、固件版本、规则版本、系统 uptime/负载** 一起报上去。
`uptime` 那行直接把 shell `uptime` 的输出抄进去了（含运行时长与负载），信息量比预想大。

两份不同的捕获（相隔数分钟）都能用同一把 key 解开 —— 再次印证 key/iv 是固定的。

### 4.6.4 结论

- 资源面两条通道用**同一套帧格式**：16B 头 + 4B 消息常量 + N 密文 + 4B CRC32(密文)，载荷是 **JSON**。
- 与 9444 隧道（HTTP/1.1 + protobuf）是**两套完全不同的协议**，印证了"控制面/资源面分离"。
- 加密强度实际为"混淆级"：固定 ASCII key + 固定 IV + 无随机数，抓包即可离线还原**两条通道的全部内容**
  （设备型号/固件版本/磁盘内存/已装包清单 + 架构/MAC/规则版本/uptime/负载）。
- `ikrc_decode_frame.py` 已内置两套 profile，会自动按 PKCS#7 合法性选择，一条命令出明文。
- 之前几轮"解不开"的原因很简单：**密文从 offset 20 开始**（16B 头 + 4B 常量），我按 16/24 切分，自然全错。
  这条教训写在这里：先确认帧边界，再谈密钥。

## 4.7 隧道侧的帧头：`netev_set_length_field` 给出 8 字节头

`librc_core.so` 导出 `netev_set_length_field(ev, u32, u16, u16)`，反汇编（0x137b3）非常短：

```asm
0x137c9: call malloc(8)          ; 分配 8 字节头
0x137ce: mov [rax],    r12d      ; +0..3  32 位
0x137d1: mov [rax+4],  bp        ; +4..5  16 位
0x137d5: mov [rax+6],  bx        ; +6..7  16 位
0x137d9: mov [r13+0x10], rax     ; 挂到 peer 对象 +0x10
```

`netev_send_msg`（0x1380f）随后把这段头拼在数据前面写出（日志 `peer %s send msg len %d`）。
所以库内 peer 帧格式 = **8 字节头（u32 + u16 + u16）+ 载荷**，最可能是 `sid / 类型 / 长度`（顺序待定）。
直接 `call` 调用点没搜到（该库对内部函数走 GOT/PLT 间接调用），所以字段命名仍属**推断**。
另有 `netev_set_conn_cb / set_recv_cb / set_length_field` 说明这是通用流式框架，**不一定**是 9444 那条 HTTP 长连接本身
（更可能是给本地 proxy / 会话通道用的），这点没有抓包就无法坐实。

**注意别和 §4.6 混淆**：`cre`/`pmd` **不链接** `librc_core.so`（它们只依赖 libcrypto/libjansson/libcurl/libz），
所以 1854/1864 的「16B 头 + 8B 固定 IV + N B 密文」是它们各自实现的格式，与这里的 8B peer 头是两套东西。

---

## 5. 安全审计要点（给自建方案做参照）

1. **双层加密，但信任根集中**：mTLS（自签 CA）+ 应用层 AES，密钥都来自同一份 `/etc/remote2/` 配置与云端下发；
   一旦拿到设备 rootfs 与 `key_passwd`，可离线解 `client.key`（DES-EDE3-CBC）。
2. **自签证书 10 年不轮换**，靠固件 pinning；证书泄露窗口极长。
3. **HTTP 层才校验客户端证书**，TLS 层 `optional`：如果哪天前面换成不校验的普通反代（或误配），mTLS 直接失效。
4. `token` 是长期凭证，客户端每次 `auth_req` 明文（在 AES 里）携带，服务端用白名单 `token_list_find_*` 比对。
5. 设备侧**没有出站白名单**：隧道一通，云端可下发任意 HTTP 请求到本地 80 端口——等于把设备管理面完全暴露给云侧，
   只能靠"云侧 API 鉴权"把关。自建时必须补一条：**设备侧按路径/方法做本地 ACL**。

---

## 6. 给 router-re 自建"远程管理中心"的可落地设计

现成件已经够，不需要另造协议：

1. **反向隧道**：复用 `agent-sync` 那套 **WSS 反向连接**（设备主动连中心，带设备 ID + token），
   中心侧按键值路由到对应设备；隧道内跑 **HTTP 帧**（`method path headers body`，与爱快的 `rest_ctrl_req` 同构即可）。
   这比自研 protobuf 更省事，WebSocket 天然支持多路复用（一个连接上多 stream id）。
2. **身份三层**（比爱快更严）：
   - 设备证书（mTLS，中心签发，**1 年轮换**，支持吊销列表）；
   - 设备 token（一次性绑定码 → 换取长期 token，可撤销）；
   - 中心侧账号 + 设备授权矩阵（谁能管哪台设备）。
3. **本地 ACL（爱快缺的那块）**：设备侧对隧道进来的请求做**白名单**（只允许 `/api/v1/**` 且方法受限），
   禁止任意路径打到本地管理面；写操作要求二次确认/审计。
4. **审计与可观测**：每条隧道请求落 `device_id / user / method / path / status / 耗时`，
   对齐 router-re 已有的审计表；中心侧展示"最近远程操作"。
5. **断网兜底**：设备侧缓存"最近一次成功的中心地址 + 证书"，中心不可达时按指数退避重连，
   并允许本地 Web 直接管理（不能因为云挂了把设备锁死）。
6. **不做的事**：不做 P2P、不做设备间互通（那属于 EasyTier/Tailscale 的职责），避免把两种拓扑混在一个模块里。

### 分阶段

| 阶段 | 内容 | 验收 |
|---|---|---|
| M1 | 设备注册（绑定码）+ WSS 反向隧道 + 远程打开设备 Web（HTTP 帧代理）+ 本地路径白名单 | 中心侧浏览器能操作设备页面；越权路径被拒；断线自动重连 |
| M2 | 批量配置下发/版本升级（复用隧道，任务化 + 幂等 + 回滚） | 两台设备同时下发，失败可回滚，审计可查 |
| M3 | 在线状态/心跳/告警（沿用保活间隔语义，中心侧判离线） | 拔网线 30s 内中心显示离线并告警 |
| M4 | 灰度/分组/权限（账号 → 设备组 → 操作） | 只读账号看不到写按钮，审计记录到人 |

现有 `mgmtd/internal/cloud/` 里那套（内存 devices + heartbeat + config push，无隧道、无鉴权、无持久化）
只能算接口占位，M1 需要重写成"隧道 + 会话"模型。

---

## 6.5 补充：隧道里的请求怎么鉴权（结合 Web 认证逆向）

`crypto/web_api_auth.md` 已还原本地 Web 的鉴权：OpenResty + `webman.lua`，会话键是 **cookie `sess_key`**，
`session_mgr.get_session()` 查 SQLite；`VerifyReqExcl` 只放行 `^/static/`、`^/Action/login$`、`^/favicon.ico`、`^/check_temp_webport$`，
其余 `/Action/*` 一律 `VerifySesskey`，失效时 POST 返回 `errcode_session_expired`、GET 302 到 `/login`。

因此云端"打开设备页面"必然来自两种实现之一（**未抓包证实，标注为推断**）：
1. 云平台在浏览器侧已有用户会话，隧道里转发的是**带 `sess_key` cookie 的原始请求**（与 `rest_ctrl_req.req` 是原始 HTTP 字节完全吻合）；
2. 或设备侧对来自本机 backend 的请求做特殊放行（但从 `VerifyExclUri` 看没有 localhost 例外）。

这条对自建有直接指导：**代理层照抄"原始 HTTP 字节转发"最省事，但必须显式决定鉴权归属**——
要么中心侧持有一次性令牌并在设备侧换成会话，要么像爱快一样透传 cookie（那就要防 cookie 被中心留存）。

---

## 7. 未证实 / 下一步

1. **9444 隧道没有真实抓包**：协议结论来自静态（字符串/反汇编/descriptor）+ 云端点探测；
   `sid` 究竟放在 protobuf 之外的消息头还是 `req` 里，尚未在流量里看到（需要能镜像设备出口的环境）。
   *（1854/1864 的帧格式已用隔离环境动态捕获，见 §4.6 —— 那两条不需要抓设备出口。）*
1.1 **1854/1864 的加密 key 来源未证实**：已知明文（pmd 的 login JSON）与密文，但用硬编码口令 `bcfae016…` 做 EVP_BytesToKey 派生解密未命中；
   头部 8 字节跨次运行不变，更像固定 IV 而非盐。下一步可反汇编 `cre`/`pmd` 里调用 AES 前的 key 构造点。
2. **绑定与证书签发流程**未实测（需要向爱快云提交绑定码，会真实绑定设备，涉及账户，不做）。
3. `rest_ctrl_req.req` 的 HTTP 报文是否带 Cookie/鉴权，静态无法确定（大概率沿用浏览器 Cookie，中心侧做身份映射）。
4. 设备侧"服务码"通道（`cre`/`cre_client`）尚未分析，它与云管理是两条独立链路；`cre` 读 "token 文件" 的具体路径也还没定位（二进制字符串只有 `Failed to get token file %s`）。
5. **心跳消息形态未定**：`librc_core.so` 导出了 `build_heart_beat_req/resp`（0xf702 / 0xf751），但 `nm -D` 里**没有对应 descriptor**
   —— 保活可能是 HTTP 层行为（`keepalive_intvl` + 202 长连接），也可能复用 `rest_ctrl`；需要抓包或反汇编调用点确认。
6. **物理侧后门（影响威胁模型）**：安装态 grub 的第 3 个菜单项 `iKuai Console Password Clean`（`timeout=1`）可在 1 秒内把
   Web/控制台口令复位成默认 `admin`（见 `bootchain/analysis.md` §4）。远程管理中心做得再严，物理接触者仍可绕过——自建方案若面向不可信现场，需要显式区分这一层。

## 附：本轮新增可复用件

| 文件 | 说明 |
|---|---|
| `work/bootchain/ikrc_descriptors.py` | 从 `*__descriptor` 符号 + program header 解析 protobuf-c 消息表（已修正 u64 布局） |
| `work/bootchain/ikrc_find_func.py` | 按日志字符串反查函数体（无节表二进制可用） |
| `work/bootchain/capture_cre.sh` / `capture_cre2.sh` / `capture_pmd.sh` | 隔离 netns+chroot 抓 1854/1864 首帧（不出网） |
| `work/bootchain/pmd_sync.gdb` / `pmd_dump.gdb` | gdb 断点脚本：取 AES key/iv，dump 明文与密文缓冲 |
| `work/bootchain/ikrc_decode_frame.py` | **帧解码器**：解析头/CRC/密文并解密成 JSON（pmd 通道已验证） |
| `work/bootchain/try_decrypt_pmd.py` | 已知明文下试解 1864 帧体（EVP_BytesToKey 候选口令） |
| `work/bootchain/samples/` | cre/pmd 首帧 hex + 运行日志（原始证据） |
| 本文 | 远程管理中心完整链路（设备实测 + 云侧实测 + 协议全表 + 自建设计） |

---

## 附录 A：动态分析方法（gdb + chroot + netns，可复用）

这套组合拳是解开 1854/1864 的关键，iKuai 的守护进程都是 ET_EXEC + uClibc，做法如下：

1. **隔离**：`ip netns add X` → 只把 `lo` 起来；`chroot` 到固件 rootfs 副本（uClibc 齐全）。
   `-s 127.0.0.1:<port>` 指向 netns 内的假服务器，厂家进程连不到公网。
2. **让进程能跑**：rootfs 是 ro 挂载时先 `cp -a` 到可写目录；补齐 `/var/run`、`/etc/log/packages`、`/tmp/iktmp`；
   `cre`/`pmd` 有单实例锁（`/var/run/*.pid`），每轮先删。
3. **gdb 附加技巧**（uClibc 动态链接器不被 gdb 识别，`EVP_*` 的 pending 断点永不生效）：
   - `file /tmp/mnt_rw/usr/sbin/pmd`（让 gdb 认识可执行文件与地址）
   - `set sysroot /tmp/mnt_rw` + `set solib-search-path /tmp/mnt_rw/usr/lib:/tmp/mnt_rw/lib`
   - `set exec-wrapper chroot /tmp/mnt_rw`
   - **在 chroot 内建同路径符号链接**（`/tmp/mnt_rw/tmp/mnt_rw/usr/sbin/pmd -> /usr/sbin/pmd`），否则报 `No executable file specified`
   - 断点直接用绝对地址（ET_EXEC 不随机化），例如 `break *0x406642`
4. **取密钥**：断在加密初始化调用点，`x/32xb $rcx`（key）、`x/16xb $r8`（iv）。
5. **取明文/密文**：`dump binary memory /tmp/pt.bin <in> <in+len>`、`dump binary memory /tmp/ct.bin $r14 $r14+len`。

坑：
- `pkill -f "pmd -d ..."` 会匹配到执行它的 shell 自身，把自己杀掉 —— 用 `pgrep -x pmd`。
- `cre`/`pmd` 捕获 SIGTERM 不退出，收尾必须 `kill -9`。
- 先在帧里定位密文起点（**找 ct 与帧的公共子串**），再谈密钥，否则会白试几百个组合（本次教训）。
