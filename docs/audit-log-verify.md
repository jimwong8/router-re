# Audit Log Verify

- Base: http://127.0.0.1:9090
- Audit file: `/var/log/router-re/audit.jsonl`
- Date: 2026-10-28 21:13:50

## 1. 文件定位
- 路径: `/var/log/router-re/audit.jsonl`
- 存在: True
- 初始行数: 51

## 2. 触发 auditWrap 写端点
- 端点: POST /api/v1/ddns/config
- action: `ddns.config.set`
- HTTP 状态: 200
- 触发前行数: 51
- 触发后行数: 52
- 新增行数: 1
- action 字段正确: True
- 含 actor: True
- 含 timestamp: True

### 新增日志行（脱敏）
```
{"ts":1793193229,"actor":"admin","action":"ddns.config.set","target":"/api/v1/ddns/config","result":"ok","detail":"HTTP 200","remote_ip":"127.0.0.1"}
```

## 3. 读端点不审计
- 端点: GET /api/v1/system/info
- HTTP 状态: 200
- 读前行数: 52
- 读后行数: 52
- 行数未变: True

## 4. 结论
- 审计日志落盘: PASS
- action 字段正确: PASS
- 读操作不审计: PASS
- 总体: PASS
