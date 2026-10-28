# nftables 生成差异测试报告

> 生成: 2026-10-28 23:19:25

**汇总**: 4 步 | PASS 4 / FAIL 0

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | 1 GET 基线 nft 预览 | PASS | HTTP 200 rules=1 nft_len=274 |
| 2 | 2 POST 追加 marker 规则 | PASS | HTTP 200 marker_present=True |
| 3 | 3 diff 可见新规则且旧规则保留 | PASS | marker_present=True old_retained=True |
| 4 | 4 删除 marker 恢复基线 | PASS | HTTP 200 restored_exact=True |

## 1. 1 GET 基线 nft 预览

- 结果: **PASS** — HTTP 200 rules=1 nft_len=274

```
table inet router {
  chain input {
    type filter hook input priority 0; policy accept;
    tcp dport 22 accept
  }
  chain forward {
    type filter hook forward priority 0; policy accept;
  }
  chain output {
    type filter hook output priority 0; policy accept;
  }
}

```

## 2. 2 POST 追加 marker 规则

- 结果: **PASS** — HTTP 200 marker_present=True

```
table inet router {
  chain input {
    type filter hook input priority 0; policy accept;
    tcp dport 22 accept
  }
  chain forward {
    type filter hook forward priority 0; policy accept;
    ip saddr 10.99.99.99 tcp dport 9999 accept
  }
  chain output {
    type filter hook output priority ...
```

## 3. 3 diff 可见新规则且旧规则保留

- 结果: **PASS** — marker_present=True old_retained=True

```
--- baseline
+++ with_marker
@@ -5,6 +5,7 @@
   }
   chain forward {
     type filter hook forward priority 0; policy accept;
+    ip saddr 10.99.99.99 tcp dport 9999 accept
   }
   chain output {
     type filter hook output priority 0; policy accept;
```

## 4. 4 删除 marker 恢复基线

- 结果: **PASS** — HTTP 200 restored_exact=True

```
table inet router {
  chain input {
    type filter hook input priority 0; policy accept;
    tcp dport 22 accept
  }
  chain forward {
    type filter hook forward priority 0; policy accept;
  }
  chain output {
    type filter hook output priority 0; policy accept;
  }
}

```
