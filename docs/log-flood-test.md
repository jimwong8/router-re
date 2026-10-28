# log flood recovery + rotate/cleanup test report

> generated: 2026-10-28 23:17:33  BASE=http://127.0.0.1:9090

**summary**: 5 items / PASS 4 / FAIL 1 / SKIP 0

has logrotate endpoint: yes (/api/v1/logs/rotate)
6x20 concurrent: all pass
hang/disk-full: no
report path: /home/jimwong/projects/router-re/docs/log-flood-test.md

| # | check | result |
|---|---|---|
| 1 | 1. GET /api/v1/logs baseline | PASS |
| 2 | 2. GET /api/v1/logs/rotate policy | PASS |
| 3 | 3. POST /api/v1/logs/rotate rotate + disk check | FAIL |
| 4 | 4. concurrent log reads 6x20 | PASS |
| 5 | 5. logrotate endpoint existence | PASS |

## 1. 1. GET /api/v1/logs baseline - PASS

- detail: HTTP 200 entries=50

```
{"count": 50, "entries": [{"ts": "2026-10-28 23:17:09", "unit": "user@1000.service", "priority": "info", "message": "2026-10-28 23:17:09,308 WARNING hermes_plugins.platforms__telegram.adapter: [Telegram] Telegram network error (attempt 5/10...
```

## 2. 2. GET /api/v1/logs/rotate policy - PASS

- detail: HTTP 200 config={'path': '', 'max_size_mb': 0, 'keep_files': 0, 'compress': False, 'interval': ''}

```
{"config": {"path": "", "max_size_mb": 0, "keep_files": 0, "compress": false, "interval": ""}, "logrotate": " {\n\n\trotate 0\n\tnocompress\n\tmissingok\n\tnotifempty\n\tcopytruncate\n}\n"}
```

## 3. 3. POST /api/v1/logs/rotate rotate + disk check - FAIL

- detail: HTTP 500 applied=None du=du error: Command '['du', '-sh', '/tmp']' returned non-zero exit status 1.

```
{"code": "10215", "detail": "logrotate -d /etc/logrotate.d/router-re-generated: /usr/sbin/logrotate -d /etc/logrotate.d/router-re-generated: warning: logrotate in debug mode does nothing except printing debug messages!  Consider using verbo...
```

## 4. 4. concurrent log reads 6x20 - PASS

- detail: requests=120 errors=0 max_latency=0.04s total=0.62s

```
{"sample_latencies": [0.026, 0.026, 0.025, 0.03, 0.032]}
```

## 5. 5. logrotate endpoint existence - PASS

- detail: POST /api/v1/logs/rotate exists

```
{"exists": true}
```

