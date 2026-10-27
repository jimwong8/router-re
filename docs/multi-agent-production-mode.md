# 一拖多：多工人并行开发的生产模式

> 结论先行：这套系统能跑，**但不是因为"多 agent 更聪明"，而是因为"多一个上下文窗口 + 队列替代注意力"**。
> 真正的瓶颈有三个，全都不是模型能力：**上游请求节奏、任务粒度、验收独立性**。
> 本文的每个阈值都来自本机实测或论文数据，不是拍脑袋。

---

## 一、研究结论（外部证据）

### 1.1 多 agent 的失败率被系统性低估

UC Berkeley MAST 论文（arXiv:2503.13657，NeurIPS 2025 D&B）标注了 **1,642 条执行轨迹**、7 个框架，发现失败率 **41%–86.7%**：

| 框架 | 失败率 |
|---|---|
| ChatDev | 41.4% |
| MetaGPT | 56.4% |
| Magentic-One | 78.6% |
| OpenManus | 86.7% |

**14 种失败模式，3 大类**（标注者一致性 κ=0.88）：

| 类别 | 占比 | 代表模式 | 对我们的含义 |
|---|---|---|---|
| FC1 系统设计 | **41.8%** | FM-1.1 违反任务规格、FM-1.3 步骤重复、FM-1.5 不知道何时终止 | 队列+验收判据能挡掉大部分 |
| FC2 智能体错位 | 36.9% | FM-2.4 信息隐瞒、FM-2.6 推理-行动不一致 | 工人之间不该互相通信，只经队列 |
| FC3 验证失败 | 21.3% | FM-3.1 过早终止、FM-3.3 错误验证 | 必须独立验证，不能自证 |

关键含义：**失败主要来自架构设计，不是模型不行**。所以"换个更强的模型"解不了，改架构才行。

实测印证：ChatDev 只做一处流程调整（CEO 有最终决定权）就 **+9.4%** 成功率。

### 1.2 编排的收益其实是"多花 token"

Anthropic 生产系统（Claude Research）用 orchestrator-worker，比单 agent **+90.2%**。但他们自己说得很清楚：

> token 用量单独解释了 **80%** 的性能方差。
> 多 agent 系统用 **15×** 于聊天的 token，单 agent 是 4×。

而且明确警告：

> **大多数编码任务真正可并行的部分比研究少得多，LLM agent 也还不擅长实时协调与委派。**

这解释了为什么我们的收益主要来自"一个工人一个上下文窗口"，而不是"工人之间协作"。

### 1.3 动态并发的实证：有界任务上是负收益

arXiv:2610.10263（354 任务 / 2,124 次执行 / >$20K）：

- 动态并发在**长周期任务**上收益最大
- **有界任务（如 SWE-bench 这类小补丁）收益很小甚至为负**
- token 消耗是串行的 **1.41–3.31 倍**
- 并发失败模式：4 大类 / 13 子类 / 28 具体模式
- 真有收益的三个条件：**互补的并行工作、替代方案探索、独立验证**

对我们：**别给"改一个文件"的任务开并发**，那是纯亏。并发只用在真正独立的多文件上。

### 1.4 瓶颈是人，不是算力

多篇实践一致（Simon Willison、Polymorphism、Agentastic）：

> 并发上限由 **review** 决定，不是由算力决定。最多 5 个 agent / 人，且不超过你能检查的量。
> 两条人工队列：**派活前写清楚工作**、**完成后读结果**。

Agentastic 的七阶段（我们只做到了 3 个）：

1. 任务图（不是提示列表）：独立 / 依赖 / 竞争 / 共享状态
2. 每个可编辑任务一个**物理边界**（git worktree）
3. 可审查结果的 prompt：outcome / scope / constraints / verification / handoff
4. 持久上下文放仓库（AGENTS.md）
5. 推理前准备环境
6. 按任务形状派发
7. **监控状态转换，不是每个 token**
8. **验证是独立阶段**
9. 按依赖顺序合并
10. **并发预算**
11. 失败转成自动化（复合改进）

### 1.5 沉默是最贵的失败

GetOnStack 真实事故：市场调研多 agent 系统，**$127/周 → $47,000 / 4 周**。
两个 agent（Analyzer + Verifier）互相礼貌对话，把误分类的错误当成"换个参数重试"，**11 天无人发现**。仪表盘全绿，日志在滚。

同类工具（heartbeat、silent-agent-watchdog、agent-watchdog）的共同做法：
- 循环检测（同一 tool+args 连续 N 次 → halt）
- 预算保护（max_budget_usd）
- 心跳超时（CoordinatorWatchdog 10s 一轮，逐级升级）

### 1.6 Context rot

Chroma 的研究与 ZenML 的实战分析：上下文退化在 **50K–150K tokens** 就开始，与厂商标称上限无关。

---

## 二、本机真实环境诊断（实测，非推断）

### 2.1 今天到底卡在哪

pM（OpenCode 工人）屏幕长期停在 `Build · DeepSeek V4 Flash`，界面不动。最初判断是"卡在 TUI 选择器"，**错了两次**，直到拿到错误分布才看清：

12 小时内的错误分布：

```
provider circuit open: sensenova-file-02      12,711 次
channel rate gate: rate limit (rpm_bucket)    10,048 次
provider circuit open: sensenova-file-02       2,079 次（另一时段）
channel rate gate: no available channel        1,429 次
provider circuit open: sensenova-tokenplan       854 次
provider circuit open: sensenova-file-06/08/04   746/693/688 次
```

**根因不是上下文太大，是 channel 池大面积熔断 + RPM 桶被打满。**

### 2.2 容量实测（这是方案的地基）

对 `deepseek-v4-flash` 做受控压测（3-token 请求，排除 token 因素）：

| 并发 | 3 轮成功率 | 错误 |
|---|---|---|
| 1 | 67% | `inference exceeds tpm/rpm limit` |
| 2 | 50% | 同上 |
| 4 | **83%** | `inference exceeds tpm/rpm limit` |
| **8** | **17%** | **`rpm exhausted`**（措辞变了＝硬桶耗尽） |

单线程 3 秒间隔连测 10 次：**8/10 成功，2 次 429**。

**结论：这个池子的安全并发 ≈ 4，到 8 就断崖。当前正好 4 个工人——已经贴着天花板，任何重试都会溢出。**

### 2.3 上下文不是瓶颈（推翻旧假设）

网关历史 14 天数据，按 prompt_tokens 分桶：

| 区间 | 请求数 | 成功率 |
|---|---|---|
| 0–16K | 6,810 | 86.3% |
| 60K–200K | **37** | **100.0%** |
| 200K–820K | 49 | **100.0%** |

**60K–200K 的大请求 100% 成功，最大成功到 820K。** 所以"139.4K 撞上下文上限"这个判断是错的。

上下文仍然有间接影响：大请求耗时 20–30 秒，占用渠道更久，在 RPM 紧张的池子里更容易把窗口占满。

### 2.4 网关自己的限流参数是虚的

```
PROVIDER_CHANNEL_MAX_CONCURRENCY=8
PROVIDER_CHANNEL_MIN_INTERVAL_MS=0
PROVIDER_CHANNEL_RPM=300          ← 实测上游撑不到这个数
```

`model_limits` 表里三个工人模型的标定：

| model | rpm | tpm | concurrency | safe_context_window | health |
|---|---|---|---|---|---|
| deepseek-v4-flash | 0 | 0 | 0 | **5333**（明显错，实测 66K 能过） | 0.925 |
| deepseek-v4.1-flash | 0 | 0 | 0 | 0 | 0.539 |
| beam-free | 0 | 0 | 0 | 0 | 0.676 |

**网关不知道自己的真实容量，所以既不能预警也不能拒绝，只能撞墙后熔断。** 这是最该补的基础设施缺口。

### 2.5 我们已有的真实产出数据

router-re 项目 12 个任务，全部 done：

- 耗时中位数 **3.3 分钟**，均值 9.7，**最长 49.2 分钟**
- 分布严重右偏：11 个 ≤ 19 分钟，2 个超长
- 两个超长的都是 **LATE**（超时判 failed 后产出才出现）
- 13 次派发 / 12 个 done → 重试率 8.3%
- `WAIT` 7 次、`RELEASE` 2 次、`FAIL` 1 次
- **所有任务都是 1–2 个文件** → 粒度小、独立性强，这是队列模式能成立的根本原因

**关键洞察：中位数 3.3 分钟说明拆分是对的；长尾 49 分钟说明"没有终止条件"才是最大风险。**

---

## 三、生产模式设计

五层，每层的参数都由上文数据推出。

```
L4 学习层    失败 → 检查/测试/规则（复合改进）
L3 验收层    四层判据 + 独立验证（换模型审 diff）
L2 调度层    队列 + blocked 检测 + 水位门控 + 退避重试 + 依赖排序
L1 任务层    任务图 / 粒度规则 / 终止条件（硬预算）
L0 容量层    实测并发上限 / RPM 校正 / 请求节奏控制
```

### L0 容量层（必须先做，否则上面全白搭）

| 参数 | 值 | 依据 |
|---|---|---|
| 同时干活工人数 | **4**（不加） | 实测并发 4→83%，8→17% |
| 每工人同时任务数 | **1** | 已有安全阀 |
| 失败重试退避 | **30s → 60s → 120s → 240s** | 立即重试会继续打满 RPM 桶 |
| 每模型 RPM 预算 | 实测标定后写入 `model_limits` | 现在全是 0 |
| 单请求上下文目标 | **< 100K** | context rot 从 50–150K 起；也是把占用时长压下来的手段 |

**关键纠正**：网关 `PROVIDER_CHANNEL_RPM=300` 必须按实测下调。现在 4 个工人 + 重试就会撞 `rpm_bucket`（今天 10,048 次）。

### L1 任务层

任务必须声明四类之一（`kind`）：

| kind | 含义 | 能否并发 |
|---|---|---|
| `independent` | 不共享文件 | ✅ 可并发 |
| `dependent` | 依赖别的任务产出 | ❌ 等上游 done |
| `competing` | 多个工人做同一件，只留一个 | ✅ 但要独立验证挑一个 |
| `shared-state` | 改同一 schema/接口/生成文件 | ❌ 串行 |

**粒度硬规则**（来自 2.5 的右偏分布）：

- 单任务目标 **≤ 3 分钟**，上限 **≤ 20 分钟**
- 超过 20 分钟 → 强制拆分（今天两个 LATE 都是这个区间）
- 单任务 **1–2 个文件**，超过 3 个文件必须拆
- prompt 必须含五要素：`outcome / scope / constraints / verification / handoff`

**终止条件是硬预算，不是建议**（对应 MAST FM-1.5，这是 $47,000 事故的直接成因）：

```json
{"max_wall_minutes": 20, "max_tool_calls": 60, "max_identical_calls": 3}
```

超预算 → 调度器强制 RELEASE，不管它自认为做完没有。

### L2 调度层

现有已经修好的（本次提交）：blocked 态优先于 idle、上下文水位门控、迟到产出翻回 done、status 默认值。

还需要补三件：

1. **退避重试**：失败任务不立即回 pending，带 `retry_after = now + backoff(n)`
2. **依赖排序**：派发时跳过 `depends_on` 未全部 done 的任务
3. **状态转换监控**（不是逐 token）：只看 `pending / dispatched / blocked / done / failed` 五个状态的迁移

### L3 验收层

现有四层判据（`expect` → `min_lines` → `must_contain` → `must_build`）**只证明"东西在、能编译"，不证明"行为对"**。

必须补：

1. **行为探针**：带随机值的请求打端点，断言响应回显该值（抓 `_ = req` 这类空壳）
2. **独立验证**：diff 交给**另一个模型/工人**审，不让实现者自审（对应 MAST FM-3.2/FM-3.3 占 21.3%）
3. **端到端**：对 UI/交互改动必须真实渲染验证，单元测试绿不算

### L4 学习层

每次失败必须落成三选一，否则同样的坑会重现：

- 一条 `must_contain` 检查
- 一个回归测试
- 一条 AGENTS.md 规则

MAST 14 模式可以当检查清单用（`pip install agentdash` 可自动标注轨迹）。

---

## 四、落地清单

| 优先级 | 动作 | 文件 | 验收方式 |
|---|---|---|---|
| P0 | 标定真实 RPM/TPM 写入 DB | `scripts/probe_capacity.py` → `model_limits` | 压测成功率 ≥ 90% |
| P0 | 网关 RPM 从 300 下调到实测值 | 17号机 `.env` | 今日 `rpm_bucket` 错误归零 |
| P1 | 任务加 `kind` / `depends_on` / `max_wall_minutes` | `ops/tasks.jsonl` + 调度器 | 依赖任务不被提前派 |
| P1 | 失败退避重试 | `dispatch_workers.py` | 日志出现 `RETRY <id> after Ns` |
| P2 | 硬预算强制 RELEASE | `dispatch_workers.py` | 无任务超 20 分钟 |
| P2 | 独立验证工人 | 新增验证任务 | diff 由非实现者审 |
| P3 | 失败 → 检查项沉淀 | `AGENTS.md` + `test_*` | 同类失败不重复 |

---

## 五、这套模式的边界（诚实说明）

1. **它不会让单个任务更快**。收益来自"4 个窗口并行 + 不互相污染上下文"。
2. **它对有界小任务是负收益**（论文实证：token 1.41–3.31×，收益小甚至为负）。一个文件的小改，直接自己动手比派活快。
3. **并发上限由 review 决定**。如果你没时间看产出，加第 5 个工人只会让失败率上升。
4. **上游容量是硬约束**。4 并发 83%、8 并发 17%，这不是调参能解决的，要加 channel 或换供应商。
5. **失败率的绝大部分来自架构**（41.8% 系统设计），所以持续投入应该在队列、判据、终止条件上，而不是换模型。

---

## 六、引用

- Cemri et al., *Why Do Multi-Agent LLM Systems Fail?* arXiv:2503.13657（MAST，1,642 traces，14 模式）
- Anthropic, *How we built our multi-agent research system*（orchestrator-worker，+90.2%，15× token）
- Li et al., *When Sub-Agents Work in Parallel: Promises and Pitfalls of Dynamic Concurrency* arXiv:2610.10263（354 任务/$20K，有界任务负收益）
- Agentastic, *A Practical Workflow for Parallel Coding Agents*（七阶段、并发预算）
- Chroma, *Context Rot*；ZenML, *1200 production deployments*（$47K/11 天事故）
- 本机实测：2026-10-27/28 网关错误分布、并发压测、任务耗时分布
