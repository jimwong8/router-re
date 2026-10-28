# p92-threat-batch: 威胁情报端点测试

生成时间: 2026-10-29 01:28:36

目标: mgmtd @ http://127.0.0.1:9090（root 运行，admin 登录取 Bearer）。全部真实 HTTP 调用。

## 汇总: 10 PASS / 0 FAIL / 0 SKIP（共 10 步）

覆盖端点: GET threat/stats、POST threat/block、DELETE threat/block、POST threat/import、
POST threat/fetch（本机 127.0.0.1 feed，零外联；任务书写的 GET 实为 POST，按 server.go L316 真实路由测）。
fetch 无需 SKIP：外部源可换成临时本机 feed；另验 SSRF 防护（file:// 被 422 拒绝）。
测试 IP 全部为 RFC 5737 TEST-NET-1 (192.0.2.0/24) 保留地址。

## 分步结果

### GET threat/stats 基线

- **状态**: PASS
- **详情**: HTTP 200, baseline={'active': 0, 'expired': 0, 'permanent': 0, 'total': 0} (期望 200 且含 total/active/expired/permanent)

### POST threat/block

- **状态**: PASS
- **详情**: HTTP 200, added=True, entry.ip='192.0.2.90', source='manual' (期望 200/added=true/source=manual)

### GET threat/stats (block 后)

- **状态**: PASS
- **详情**: HTTP 200, stats={'active': 1, 'expired': 0, 'permanent': 0, 'total': 1}, Δtotal=1 Δactive=1 (期望各 +1)

### POST threat/import

- **状态**: PASS
- **详情**: HTTP 200, added=2 skipped=1 parse_note='解析 1 行失败被跳过（共 1 个原因）: line 4: 既不是合法 IP 也不是合法 CIDR' (期望 added=2 skipped=1, 坏行不静默)

### POST threat/import 空文本

- **状态**: PASS
- **详情**: HTTP 422, code=10166 (期望 422/10166)

### GET threat/stats (import 后)

- **状态**: PASS
- **详情**: HTTP 200, Δtotal=3 (期望累计 +3: block+import)

### POST threat/fetch (本机127.0.0.1 feed)

- **状态**: PASS
- **详情**: HTTP 200, added=2 source='http://127.0.0.1:46515/p92feed' (期望 200/added=2, 未外联)

### POST threat/fetch SSRF 防护

- **状态**: PASS
- **详情**: HTTP 422, code=10167 detail='拒绝非 http/https 的情报源 scheme "file"（防 SSRF，仅允许 http/https）' (期望 422/10167 拒绝 file://)

### GET threat/stats (fetch 后)

- **状态**: PASS
- **详情**: HTTP 200, Δtotal=5 (期望累计 +5: 1 block + 2 import + 2 fetch)

### 清理 (DELETE 全部测试条目)

- **状态**: PASS
- **详情**: 5 条测试条目全部 removed=true, 最终 stats={'active': 0, 'expired': 0, 'permanent': 0, 'total': 0} 与基线 {'active': 0, 'expired': 0, 'permanent': 0, 'total': 0} 一致

## 证据片段

**GET threat/stats 基线**

```
{"active":0,"expired":0,"permanent":0,"total":0}

```
**POST threat/block**

```
{"added":true,"entry":{"ip":"192.0.2.90","reason":"p92test","source":"manual","created_at":1793208515,"expires_at":1793208815,"hits":0}}

```
**block 后 GET threat/stats**

```
{"active":1,"expired":0,"permanent":0,"total":1}

```
**POST threat/import**

```
{"added":2,"parse_note":"解析 1 行失败被跳过（共 1 个原因）: line 4: 既不是合法 IP 也不是合法 CIDR","skipped":1}

```
**import 空文本被拒**

```
{"code":"10166","detail":"导入内容为空；空文本无法产生任何情报条目","message":"text must not be empty"}

```
**POST threat/fetch (本机 feed)**

```
{"added":2,"skipped":0,"source":"http://127.0.0.1:46515/p92feed"}

```
**fetch SSRF 防护 (file:// 被拒)**

```
{"code":"10167","detail":"拒绝非 http/https 的情报源 scheme \"file\"（防 SSRF，仅允许 http/https）","message":"fetch failed"}

```
**DELETE 192.0.2.90**

```
{"ip":"192.0.2.90","removed":true}

```
**DELETE 192.0.2.91**

```
{"ip":"192.0.2.91","removed":true}

```
**DELETE 192.0.2.92/30**

```
{"ip":"192.0.2.92/30","removed":true}

```
**DELETE 192.0.2.93**

```
{"ip":"192.0.2.93","removed":true}

```
**DELETE 192.0.2.94**

```
{"ip":"192.0.2.94","removed":true}

```
**最终 GET threat/stats**

```
{"active":0,"expired":0,"permanent":0,"total":0}

```