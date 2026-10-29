# Smart QoS 三状态码验证报告

**日期**: 2026-10-30

## 验证结果

### 1. GET /htb/smart-plan?iface=eth0&root_mbit=100 → 200
返回每终端均分配置（13 个终端，各 ~7.69 Mbps）

### 2. POST /htb/smart-apply (无 confirm) → 422
```json
{"code":"11018","detail":"confirm=true required","message":"confirm required"}
```

### 3. POST /htb/smart-apply (confirm=true) → 200
```json
{"applied":false,"detail":"command failed: tc qdisc add dev eth0 root handle 1: htb default 10 → ... Cannot find device \"eth0\""}
```
(applied=false 因本机无 eth0 接口，非逻辑错误)

## 结论
三状态码验证通过: 200 / 422 / 200
