# Router-RE — 现代化路由系统

Router-RE 是一个**现代化软路由系统**，架构理念模仿 [iKuai](https://www.ikuai8.com)（爱快）等企业级路由平台：将传统路由器 OS 的配置管理、认证、防火墙等模块拆分为独立服务，以 Go 实现，具备清晰的模块边界和可测试性。

## 特性

- **配置管理** (`internal/config`)：UCI 风格的配置抽象接口，支持快照/回滚事务语义
- **认证** (`internal/auth`)：JWT 会话令牌 + 口令哈希
- **防火墙** (`internal/firewall`)：策略模型 → nftables 规则渲染
- **NAT** (`internal/nat`)：masquerade / 端口映射 → nftables
- **QoS** (`internal/qos`)：限速 → tc 命令
- **多 WAN** (`internal/wan`)：加权轮询 + 策略路由
- **动态路由** (`internal/route`)：FRR (BGP/OSPF) 配置生成
- **VPN** (`internal/vpn`)：WireGuard 密钥/配置
- **系统信息** (`internal/system`)：真实 /proc 读取
- **云管理 / 无线 / DPI** (`internal/cloud`, `internal/wireless`, `internal/dpi`)
- **REST API** (`internal/server`)：HTTP 服务端（`/api/v1/*` 端点）

## 架构

```
mgmtd/
├── cmd/mgmtd/           # 入口
├── internal/
│   ├── auth/            # JWT 认证 + 口令哈希
│   ├── config/          # 配置存储抽象 (Store) + 快照/回滚
│   ├── firewall/        # 防火墙策略 → nftables
│   └── server/          # REST HTTP 服务
├── Makefile
└── go.mod
```

## 构建

```bash
make build        # 产出 bin/mgmtd（CGO_ENABLED=0 静态编译）
```

## 运行

需要先设置会话密钥环境变量（32+ 字节）：

```bash
MGM_SESSION_SECRET=your-32-byte-secret-here ./bin/mgmtd
```

默认监听 `:8080`。

## 测试

```bash
make test         # go test ./...
```

## 清理

```bash
make clean        # rm -rf bin
```

## OpenWrt 打包

```bash
cd openwrt && ./build_ipk.sh
# 产出 openwrt/router-re_0.1.0-r1_x86_64.ipk
```

## Docker 冒烟

```bash
cd mgmtd && make build && docker build -t mgmtd:test . && \
  docker run --rm -p 8080:8080 -e MGM_SESSION_SECRET=0123456789abcdef0123456789abcdef mgmtd:test
```
