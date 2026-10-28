# 错误码审计报告 (104xx)

> 扫描: /home/jimwong/projects/router-re/mgmtd  (mgmtd Go 源码)
> 生成: 2026-10-28 22:58:36

## 1. 概览

- 实际使用 code 数量: **42**
- 范围: **10400 ~ 10485**
- 冲突 (同一 code 多个不同 message): **2**
- 跳号 (10400~10485 范围内缺失): **44**
- 在 docs/e2e-audit-fixes.md 中未记录的 code: **5**
- 文档记录但源码未使用的 code: **0**

## 2. 冲突详情 (同一 code, 多个 message)

- `10454`:
  - "cannot build probe args"
  - "unknown kind"
- `10480`:
  - "invalid body"
  - "即时开启失败"

## 3. 跳号 (10400 ~ 10485)

10406, 10407, 10408, 10409, 10410, 10411, 10412, 10413, 10414, 10415, 10416, 10417, 10418, 10419, 10426, 10427, 10428, 10429, 10443, 10444, 10445, 10446, 10447, 10448, 10449, 10455, 10457, 10458, 10459, 10465, 10466, 10467, 10468, 10469, 10470, 10471, 10472, 10473, 10474, 10475, 10476, 10477, 10478, 10479

## 3b. 单点跳号 (潜在 bug: N-1 与 N+1 都存在但 N 缺失)

10455

## 4. 多文件重复定义 (信息性)

(无)

## 5. 源码中有但文档未记录

10405, 10482, 10483, 10484, 10485

## 6. 文档记录但源码未使用

(无)

## 7. 全部 code 清单 (code -> messages, files)

| code | messages | files |
|---|---|---|
| 10400 | "refusing to stop/restart a protected unit without confirm" | internal/server/p15.go |
| 10401 | "confirm required" | internal/server/p15.go |
| 10402 | "signal not allowed" | internal/server/p15.go |
| 10403 | "rules must not be empty" | internal/server/p15.go |
| 10404 | "rule " | internal/server/p15.go |
| 10405 | "save failed" | internal/server/p15.go |
| 10420 | "invalid JSON body" | internal/server/p16.go |
| 10421 | "invalid wireguard client config" | internal/server/p16.go |
| 10422 | "invalid JSON body" | internal/server/p16.go |
| 10423 | "record " | internal/server/p16.go |
| 10424 | "save failed" | internal/server/p16.go |
| 10425 | "records must not be empty" | internal/server/p16.go |
| 10430 | "invalid JSON body" | internal/server/p17.go |
| 10431 | "count must be 1-500" | internal/server/p17.go |
| 10432 | "generate failed" | internal/server/p17.go |
| 10433 | "generate password failed" | internal/server/p17.go |
| 10434 | "generated voucher failed validation" | internal/server/p17.go |
| 10435 | "save failed" | internal/server/p17.go |
| 10436 | "invalid JSON body" | internal/server/p17.go |
| 10437 | "核销失败" | internal/server/p17.go |
| 10438 | "save failed" | internal/server/p17.go |
| 10439 | "核销失败" | internal/server/p17.go |
| 10440 | "code required" | internal/server/p17.go |
| 10441 | "not found" | internal/server/p17.go |
| 10442 | "save failed" | internal/server/p17.go |
| 10450 | "targets must not be empty" | internal/server/p18.go |
| 10451 | "target " | internal/server/p18.go |
| 10452 | "invalid JSON body" | internal/server/p18.go |
| 10453 | "target not found" | internal/server/p18.go |
| 10454 | "cannot build probe args" / "unknown kind" | internal/server/p18.go |
| 10456 | "save failed" | internal/server/p18.go |
| 10460 | "invalid body" | internal/server/p18b.go |
| 10461 | "invalid template" | internal/server/p18b.go |
| 10462 | "marshal failed" | internal/server/p18b.go |
| 10463 | "save failed" | internal/server/p18b.go |
| 10464 | "invalid template" | internal/server/p18b.go |
| 10480 | "invalid body" / "即时开启失败" | internal/server/p24.go |
| 10481 | "持久化失败" | internal/server/p24.go |
| 10482 | "save failed" | internal/server/p18.go |
| 10483 | "save failed" | internal/server/p16.go |
| 10484 | "links 不能为空" | internal/server/server.go |
| 10485 | "链路缺少接口名" | internal/server/server.go |

