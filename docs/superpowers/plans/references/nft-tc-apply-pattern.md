## nftables 防火墙模块配方（router-re/mgmtd 内部验证）

适用：Go module `github.com/jimwong/router-re/mgmtd` 里 `internal/firewall/` 包，用 nftables 做真实规则渲染 + 下发验证（非假实现）。

### 数据模型

```go
type Rule struct {
    Chain  string // "input"/"forward"/自定义
    Src    string // "0.0.0.0/0" 或 CIDR
    Dst    string
    Proto  string // "tcp"/"udp"
    DPort  string // "80" 或空
    Action string // "accept"/"drop"
}
```

### 渲染铁则

规则语句（`tcp dport 80 accept`）必须写在 `chain <name> { ... }` 块内，不能写在 hook 声明（`chain input { type filter hook input priority 0; policy accept; }`）之后的顶层。渲染器按 Chain 分组，每个非空组生成一个 `chain` 块：

```
table inet router {
  chain input { type filter hook input priority 0; policy accept; }
  chain forward { type filter hook forward priority 0; policy accept; }
  chain forward {
    tcp dport 80 accept
  }
}
```

### Validate / Apply 实现模式

- `Validate`: `exec.Command("/usr/sbin/nft", "-c", "-f", "-")`，Stdin 喂 `RenderNftables(...)`。
- `Apply`: 先 `Validate`（失败直接返回，不写 store）；再 `runNft(["-f", "-"], ruleset)` 应用。
- `runNft` 先试直接 `bin`；错误含 `Operation not permitted`/`permission denied` 时自动 `sudo -n <bin>` 重试一次；仍失败返回带 `permission denied` 标记的错误（供调用方区分语法错误与权限问题）。
- 测试：`exec.LookPath("/usr/sbin/nft")` 无则 `t.Skip`；有 nft 但无 root 则 `Validate` 失败返回 `permission denied` → 测试跳过（`t.Skipf`）而非 `t.Fatalf`。

### tc 流量控制（QoS）配方

`RateLimit` + `RenderTcCommands`：
- egress → `tc qdisc replace dev <if> root tbf rate <N>mbit burst <B>kb latency 50ms`
- ingress → 先 `qdisc add ... handle ffff: ingress` 再 `filter add ... police rate ... drop`
- `nft` 备选规则：`limit rate <N> mbytes/second`

### 真实验证坑

- `nft -c` 仍需要 root（netlink cache 初始化），非 root 报 `Operation not permitted`。
- `nft -f` 在已有 table 上追加规则 → 规则重复累加，`Apply` 前必须 `delete table inet <table>` 并忽略首次不存在的错误。
- `nft` 二进制在 `/usr/sbin/`（非 PATH 默认），用 `exec.LookPath` 探测多个路径，不直接写 `"nft"`。
- `go test` 的单文件 vet 对 `*_test.go` 报 `undefined: RateLimit`（同包兄弟文件符号）是噪音，包级 `go test ./...` 才是真信号。
