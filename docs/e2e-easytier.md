# EasyTier 组网 E2E 验证

**Result: 12 PASS / 0 FAIL**

- 目标实例: `http://127.0.0.1:9095`（真实 easytier-core 二进制）

| 用例 | 结果 | 实测 |
|---|---|---|
| 登录 | PASS | 200 + token |
| GET config | PASS | 200 binary_exists=True version=easytier-core 2.6.4-8428a89d tun=True |
| POST config 启用 | PASS | 200 applied=True err=None |
| 进程运行 | PASS | running=True pid=2504128 |
| 配置渲染 | PASS | 含 network_name 段，无 peer 时未生成 [[peer]] |
| GET status | PASS | 200 running=True peers=1 |
| 节点自检 | PASS | local={'ipv4': '10.200.44.1/24', 'hostname': 'jimwong-standardpc', 'cost': 'Local', 'latency_ms': 0, 'latency_ok': False, 'loss': '-', 'rx': '-', 'tx': '-', 'tunnel': '-', 'nat': 'Unknown', 'version': '2.6.4-8428a89d', 'local': True} |
| POST preview | PASS | 200 密钥打码=True |
| 拒绝配置注入 | PASS | 422 code=12011 detail=配置项含换行或控制字符 |
| 留空沿用密钥 | PASS | 200 渲染中仍含原密钥 |
| POST stop | PASS | 200 stopped=True running=False |
| 停止后查询 | PASS | 200 peers=[] running=False |
