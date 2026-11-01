# Tailscale 接入 E2E 验证

**Result: 15 PASS / 0 FAIL**

- 隔离实例 `http://127.0.0.1:9096`：独立 tailscaled（userspace 网络、独立 socket），可安全 up/down
- 系统实例 `http://127.0.0.1:9097`：接系统 tailscaled，**只读**，验证真实节点解析

| 用例 | 结果 | 实测 |
|---|---|---|
| 登录（隔离实例） | PASS | token=有 |
| GET config | PASS | 200 binary=True socket_exists=True system_socket=False |
| 识别为隔离 socket | PASS | system_socket=False |
| 初始状态可读 | PASS | reachable=True backend_state=NeedsLogin |
| POST preview | PASS | 200 up --authkey=tskey-******** --hostname=router-e2e --advertise-routes=192.168.77.0/24 --advertise-exit-node=false --accept-routes=true --accept-dns=true --ssh=false |
| 预览里密钥打码 | PASS | up --authkey=tskey-******** --hostname=router-e2e --advertise-routes=192.168.77.0/24 --advertise-exit-node=false --accept-routes=true --accept-dns=true --ssh=false |
| 假 key 被真实拒绝 | PASS | 200 applied=False error=节点需要授权（未返回授权链接，多半是 auth key 无效或已过期）：backend error: invalid key: API key does not exist |
| 失败后状态如实 | PASS | backend_state=NeedsLogin |
| 拒绝非法主机名 | PASS | 422 code=12021 detail=节点名只能包含字母数字和连字符，且不能以连字符开头/结尾：-bad_host |
| POST down | PASS | 200 down=True state=NeedsLogin |
| 停止后仍可读状态 | PASS | 200 state=NeedsLogin |
| 系统节点识别 | PASS | system_socket=True socket=/var/run/tailscale/tailscaled.sock |
| 系统节点在线 | PASS | backend_state=Running version=1.94.2-t0a29cf18b-g3f044c9f6 |
| 真实节点解析 | PASS | peer_count=75 online=48 首行=ares3178379047 |
| 本机地址解析 | PASS | self=jimwong-standardpc ips=['100.111.28.64', 'fd7a:115c:a1e0::ea35:1c40'] |
