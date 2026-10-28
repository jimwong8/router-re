# log flood recovery + rotate/cleanup test report

> generated: 2026-10-28 23:18:42  BASE=http://127.0.0.1:9090

**summary**: 5 items / PASS 5 / FAIL 0 / SKIP 0

has logrotate endpoint: yes (/api/v1/logs/rotate)
6x20 concurrent: all pass
hang/disk-full: no
report path: /home/jimwong/projects/router-re/docs/log-flood-test.md

| # | check | result |
|---|---|---|
| 1 | 1. GET /api/v1/logs baseline | PASS |
| 2 | 2. GET /api/v1/logs/rotate policy | PASS |
| 3 | 3. POST /api/v1/logs/rotate rotate + disk check | PASS |
| 4 | 4. concurrent log reads 6x20 | PASS |
| 5 | 5. logrotate endpoint existence | PASS |

## 1. 1. GET /api/v1/logs baseline - PASS

- detail: HTTP 200 entries=50

```
{"count": 50, "entries": [{"ts": "2026-10-28 23:18:26", "unit": "user@1000.service", "priority": "info", "message": "Client does not support MCP Roots, using allowed directories set from server args: [ '/home/jimwong' ]"}, {"ts": "2026-10-2...
```

## 2. 2. GET /api/v1/logs/rotate policy - PASS

- detail: HTTP 200 config={'path': '/home/jimwong/router-re-logflood.log', 'max_size_mb': 1, 'keep_files': 3, 'compress': False, 'interval': 'none'}

```
{"config": {"path": "/home/jimwong/router-re-logflood.log", "max_size_mb": 1, "keep_files": 3, "compress": false, "interval": "none"}, "logrotate": "/home/jimwong/router-re-logflood.log {\n\tsize 1M\n\trotate 3\n\tnocompress\n\tmissingok\n\...
```

## 3. 3. POST /api/v1/logs/rotate rotate + disk check - PASS

- detail: HTTP 200 applied=True du=/dev/sda1       469G  339G  107G   77% /

```
{"applied": true, "logrotate": "/home/jimwong/router-re-logflood.log {\n\tsize 1M\n\trotate 3\n\tnocompress\n\tmissingok\n\tnotifempty\n\tcopytruncate\n}\n", "path": "/etc/logrotate.d/router-re-generated"}
```

## 4. 4. concurrent log reads 6x20 - PASS

- detail: requests=120 errors=0 max_latency=0.04s total=0.63s

```
{"sample_latencies": [0.026, 0.035, 0.036, 0.038, 0.036]}
```

## 5. 5. logrotate endpoint existence - PASS

- detail: POST /api/v1/logs/rotate exists

```
{"exists": true}
```

