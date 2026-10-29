# p122-fuzz-rerun

- fuzz: total=168 safe_4xx=149 accepted_2xx=12 dangerous=3 status500=0 status4xx=149 stack_leak=0 xss=0 conn_fail=3
- error_code_audit: codes=52 range=10400-10497 conflicts=3 missing=46 not_in_doc=17
- 结论: 无 500/panic/stack/xss；3 个 conn_fail 为 huge_1mb 超大 payload 连接中断（非 500）；错误码存在 3 个冲突需后续清理。
