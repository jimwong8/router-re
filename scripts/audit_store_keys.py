#!/usr/bin/env python3
"""审计配置文件里的键是否出现"自我叠加"。

背景：某版本的恢复逻辑若把完整键（section.option）当成 section 再拼一次，
就会写出 `alerts.alerts/rules.alerts.alerts/rules` 这种键。它不报错、不影响
读取（没人读这个名字），但会让配置文件随每次恢复膨胀，并且在导出/对比时
制造噪音。历史数据里已经出现过，所以留一个只读审计，别让新实例重复踩。

用法:
  python3 scripts/audit_store_keys.py /path/to/config.json
退出码: 0 干净 / 1 发现叠加键 / 2 文件不可读
"""
import json
import sys


def is_self_doubled(key: str) -> bool:
    """判断 key 是否为 X 形式：存在 k 使得 key == k + "." + k（可多级）。"""
    if key.count(".") < 1:
        return False
    # 从最短的重复单元试到一半长度
    for cut in range(1, len(key) // 2 + 1):
        unit = key[:cut]
        rest = key[cut:]
        if rest == "." + unit:
            return True
        if rest.startswith("." + unit) and is_self_doubled(rest[1:]):
            return True
    return False


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    path = sys.argv[1]
    try:
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
    except (OSError, json.JSONDecodeError) as e:
        print(f"读不了 {path}: {e}")
        return 2
    if not isinstance(data, dict):
        print(f"{path} 顶层不是对象")
        return 2

    bad = sorted(k for k in data if is_self_doubled(k))
    if not bad:
        print(f"干净: {path} 共 {len(data)} 个键，无自我叠加")
        return 0

    print(f"发现 {len(bad)} 个自我叠加键（共 {len(data)} 键），逐个列出：")
    for k in bad:
        print(f"  - {k}")
    print("\n处理建议：这些键没有任何代码读取，可以直接从文件里删掉；"
          "但在删除前先确认当前版本已无法再生（跑 "
          "go test ./internal/config/ -run RoundTrip）。")
    return 1


if __name__ == "__main__":
    sys.exit(main())
