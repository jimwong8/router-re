# 测试覆盖率报告

> 生成时间：由真实命令输出汇总，未估算。

## 概览

| 指标 | 数值 |
|------|------|
| 总包数 | 75 |
| 有测试的包 | 74 |
| 无测试的包 | 1 |
| 测试函数总数（internal/） | 786 |
| 通过率 | 74/75 = 98.7% |

命令：
- `go list ./... | wc -l` → 75
- `go test ./... 2>&1 | grep -c '^ok'` → 74
- `go test ./... 2>&1 | grep 'no test files' | wc -l` → 1
- `grep -rc '^func Test' mgmtd/internal --include=*_test.go | awk -F: '{s+=$2} END {print s}'` → 786

## 无测试的包

- `github.com/jimwong/router-re/mgmtd/cmd/mgmtd`

（所有 internal/* 包均已有测试文件。）

## 建议优先补测试的包

从无测试包中按名称筛选安全相关（auth / voucher / nac / vpn / ddos / portal）：

- 无匹配项。当前唯一无测试包 `cmd/mgmtd` 为命令入口，不命中安全关键字。

说明：虽然 `cmd/mgmtd` 本身无测试，但其依赖的内部包（auth 相关逻辑、ddos、nac、vpn、portal 等）均有测试覆盖。若后续需要提升入口层信心，可为 `cmd/mgmtd` 增加启动/路由冒烟测试。