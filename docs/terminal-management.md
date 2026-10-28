# 终端管理 (Terminal Management)

## /api/v1/terminals — 终端列表聚合

**方法**：`GET /api/v1/terminals`
**性质**：只读聚合查询，不修改任何系统状态。

### 数据源

| 来源 | 来源文件 / 调用 | 说明 |
|---|---|---|
| ARP | `/proc/net/arp` | 解析 `IP HW_type Flags HW_address Mask Device` 字段 |
| DHCP | `dhcp.NewApplier().Leases()` | 读取 `/var/lib/misc/dnsmasq.leases` |
| conntrack | `online.ParseConntrackSessions(string)` 来源 `/proc/net/nf_conntrack` | 文件不存在时静默返回空 |

- 三个数据源按 `ip` 去重合并。
- 同一 IP 在多个数据源中均出现时，`source` 拼接为逗号列表（如 `"arp,dhcp"`）。
- `hostname` 优先来自 DHCP 租约；ARP/conntrack 通常没有主机名。

### 响应

```json
{
  "count": 2,
  "terminals": [
    {
      "ip": "10.100.1.50",
      "mac": "b8:2a:72:xx:xx:xx",
      "hostname": "phone-jim",
      "iface": "br-lan",
      "source": "arp,dhcp",
      "online": true
    },
    {
      "ip": "10.100.1.88",
      "mac": "",
      "hostname": "",
      "iface": "",
      "source": "conntrack",
      "online": true
    }
  ]
}
```

### 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `count` | int | 去重后终端总数 |
| `terminals` | array | 终端对象列表 |
| `- ip` | string | IPv4/IPv6 地址 |
| `- mac` | string | 物理地址，已小写 |
| `- hostname` | string | 主机名，可能为空 |
| `- iface` | string | 所属接口，可能为空 |
| `- source` | string | 数据来源：`arp` / `dhcp` / `conntrack`，可组合 |
| `- online` | bool | 在线状态，聚合后恒为 `true` |
| `note` | string | 仅 `/proc/net/arp` 不存在或读取失败时出现，说明原因 |

### 异常返回

当 `/proc/net/arp` 不存在或无法读取时，返回 `count=0` 且 `terminals=[]`，同时在 `note` 中写明原因（例如 `/proc/net/arp 不存在，可能非 Linux 或内核未开启 ARP 表`），HTTP 仍为 `200`。
