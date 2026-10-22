# CareerAgent

![CI](https://img.shields.io/github/actions/workflow/status/ElvisLin101/ai-meeting/ci.yml?branch=main&label=CI)
[![Go Version](https://img.shields.io/badge/Go-1.24.1-00ADD8?logo=go&logoColor=white)](https://go.dev/)

> 一个面向长任务的自主 Agent 后端：手写 ReAct 循环 + Function Calling 工具编排 + 四层上下文管理。
> 求职/面试是示例领域；结构化模拟面试作为「确定性流程工具」接入，用来展示自主编排与确定性编排的边界。
> 不依赖任何 Agent 框架，存储统一为 Redis（热数据）+ MySQL（真相源 / 全量历史 / 恢复源）。

## 技术栈

| 维度 | 技术 |
|------|------|
| 语言 | Go 1.24.1 |
| Web 框架 | Gin |
| 关系存储（真相源） | MySQL + GORM（会话/消息/画像/身份/run/step/压缩快照/面试数据） |
| 热缓存 | Redis（近期对话、压缩摘要、画像、身份、运行态、分布式 SingleFlight） |
| 配置 | Viper |
| 认证 | JWT |
| AI 模型 | DeepSeek（OpenAI 兼容协议，Function Calling） |
| 文件工具 | 独立沙箱目录（路径校验 + 原子写 + 写前备份） |
| PDF 解析 | ledongthuc/pdf |

## 项目结构

```
├── api/                    # 路由、中间件、Handler
├── services/
│   ├── agent/
│   │   ├── loop/           # ReAct 循环（终止条件/熔断/工具调度）
│   │   ├── tools/          # 工具注册与执行（PDF/搜索/文件/面试）
│   │   ├── context/        # 四层上下文 + 画像提炼 + 压缩
│   │   └── run/            # run 编排、持久化、SingleFlight
│   ├── interview/          # 确定性面试流程（状态机 + 答题流水线）
│   ├── ai/                 # 通用 AI 对话与记忆
│   ├── metric/             # 异步指标
│   └── user/               # 用户管理
├── clients/                # 模型客户端（普通/流式/JSON/Function Calling）
├── pkg/
│   ├── singleflight/       # 分布式 SingleFlight
│   ├── lock/               # 分布式锁
│   └── ecode/              # 统一错误码
├── repositories/
│   ├── mysql/              # MySQL 真相源仓储
│   └── redis_hot.go        # Redis 热数据（可重建）
├── models/                 # GORM 模型
├── dto/                    # 请求/响应
├── config/                 # 配置
├── static/                 # 调试用单页前端
└── docs/agent-knowledge/   # 知识文档
```

## 核心设计

### 1. 自主 Agent：手写 ReAct 循环 + 工具编排

**为什么手写而不是套框架**：Agent 的关键不是「调通 function calling」，而是**什么时候停、工具失败怎么办、上下文怎么不爆**。这些边界只有自己实现才能讲清楚。

```
组装四层上下文 → 调模型(tools) → 无 tool_calls ? 最终回答
                              → 有 tool_calls ? 执行工具 → 结果回填 → 继续
```

**终止条件**：

- 模型给出最终回答
- 达到最大步数（默认 8）
- 连续重复同一工具 + 参数（防死循环）
- 工具连续失败达到上限（默认 3）
- ctx 取消 / 总超时（默认 120s）

**工具执行统一收口**：JSON Schema 校验（required + 顶层类型）、单工具超时、panic recovery、结果 32KB 截断、审计日志。

**单步工具批调度**：同一条 assistant 消息里的多个 tool call 默认并行；只有整批工具都声明 `ConcurrencySafe` 才并行，任一工具未声明（如写文件、面试状态推进）则整批串行。结果按原始 call 顺序回填，单个工具失败不取消同批兄弟调用。并行上限由 `agent.max_parallel_tools` 控制（默认 4）。

**Plan-and-Execute（可选）**：复杂任务先由 planner 生成带 `depends_on` 的任务图，再按依赖并行执行；每个任务内部复用 ReAct（agent）、单次只读工具（tool）或纯文本推理（llm），最后统一合成。规划失败/不需要计划时自动回退线性 ReAct。配置见 `agent.plan.*`，任务图与结果持久化在 `agent_runs.plan_json`。

**Replan 与重试（谨慎模式）**：任务失败或返回 `ReplanHint` 时，调度器在一批任务结束后统一评估，多个失败合并为一次 replan；计划修改只在轮次边界单线程串行应用，已完成任务冻结。diff 必须通过依赖/环/只读工具校验，并受 `max_replans` 和计划哈希震荡检测约束。每个任务有基于语义的 `idempotency_key`，同 key 的成功结果在本次 run 内复用；失败任务只有 `Retryable=true` 才重试（默认最多 2 次，指数退避），调用过有副作用工具的 agent 任务不会自动重试。

**前置路由**：不是每个请求都进 Plan。`services/agent/plan/router.go` 用规则打分（强多步信号、中信号、长度、多问号/多行加分；纯问候、明确要求直接回答、短且无强信号减分），只有 `score >= min_score` 才调用 planner。`agent.plan.mode=auto|always|never` 可强制开关；SSE 的 `route` 事件和返回的 `plan_mode` 字段会暴露本次选择。

**内置工具**：

| 工具 | 能力 | 安全边界 |
|---|---|---|
| `pdf_parse` | 解析沙箱内 PDF | 扩展名 + `%PDF-` 魔数 + 10MB 上限 |
| `web_search` | 真实搜索 API（Tavily/Serper） | 超时、条数/长度限制、结果标记为不可信 |
| `file_list/read/write/edit` | 沙箱内文本文件操作 | 拒绝绝对路径/`..`/符号链接逃逸；原子写 + 写前备份 |
| `interview_*` | 结构化模拟面试 | 包装确定性状态机 + 会话归属校验 |

### 2. 四层上下文管理

| 层 | 内容 | 存储 |
|---|---|---|
| L1 短期 | 近期全量对话（按预算取最近消息） | Redis List（miss 从 MySQL 重建） |
| L2 远期 | 80/20 压缩摘要 | Redis + MySQL 快照 |
| L3 跨会话 | 结构化用户画像（偏好/目标/约束/背景 + 置信度） | Redis + MySQL |
| L4 自我定位 | Agent persona（角色/能力边界/风格）+ 低频自省 | Redis + MySQL |

拼装顺序：`system(persona) → 用户画像 → 压缩摘要 → 近期对话`。预算按 token 计算（`agent.context.*`），不足时优先裁剪近期对话，persona 与画像不丢弃、只按上限截断。

- **画像 L1/L2**：每个用户消息先用规则筛出候选（MySQL `profile_candidates`，不调用 LLM、不直接改画像）；每 3 个用户回合或候选 ≥5 时跑一次 L2 consolidation，输入“当前画像 + 候选 + 最近对话”，输出增量 diff，冲突时 supersede 或写入 `profile_confirmations` 等用户确认。
- **短期/中期边界**：`compressed_contexts.index` 是 `CoveredThroughSeq`；摘要覆盖 `sequence <= index`，短期只取 `sequence > index`，二者不重叠。
- **压缩**：只压缩对话消息，不压缩 identity/profile/plan/tool schema；按 token 触发，压缩较旧的 80%，保留最近 20% 原文（有最小尾部），增量合并旧摘要，避免每次重压全部。
- **存储**：短期 Redis List + MySQL 全量消息；中期 Redis 摘要 + MySQL 一会话一行；长期 Redis JSON + MySQL `user_profiles.profile_json`（带 version CAS），候选/确认/审计分别落 MySQL。

### 3. 确定性编排工具：状态机不删，降级为工具

模拟面试仍由显式状态机驱动（`INIT/ASKING/EVALUATING/FOLLOW_UP/COMPLETED` + CAS 乐观锁 + 条件回滚 + 幂等三层防线）。

差异在于：**Agent 判断何时进入结构化面试，进入后由状态机保证一致性**。这体现了架构判断——评分、计分、流程推进这类强一致任务不交给模型自由发挥。

### 4. 分布式 SingleFlight — 跨实例 Agent run 去重

Agent run 动辄数十秒，前端重试概率高。基于 Redis 实现跨实例请求合并：

- 主从选举（`SET NX`）、流式输出作心跳、停滞后自动换主、双通道取消（Pub/Sub + 轮询）、Redis 不可用降级本地
- 非流式 run 走 SingleFlight 合并；流式 run 用 Redis 幂等锁拒绝重复提交（SingleFlight 无法复用流式 chunk）

**集群实测**（`cmd/sfcluster`：N 个独立进程共享一个 Redis）：

| 场景 | 实例数 | 每实例并发 | 总请求 | 实际执行 | 结果一致 | 失败 |
|---|---|---|---|---|---|---|
| 去重 | 3 | 50 | 150 | 1 | 150/150 | 0 |
| 换主（首个 leader 假死） | 3 | 50 | 150 | 2 | 150/150 | 0 |

> 集群压测还发现并修复了一个换主竞态：新 leader 抢锁后清空 `progressKey`，但首段输出前存在空窗口，迟到的旧 follower 会误判新 leader 卡死并 cancel 它。修复方式：新 leader 清理后立即写占位进度（见 `pkg/singleflight/singleflight.go`）。

### 5. 存储：MySQL 真相源 + Redis 热数据

```
写: MySQL(事务) → Redis(失败仅告警, 不阻断)
读: Redis → miss → MySQL 回填并重建
```

- 会话/消息/压缩快照/画像/身份/run/step/面试数据全部落 MySQL
- 序列号用 `session_sequences` 的 `INSERT ... ON DUPLICATE KEY UPDATE seq=seq+1` 原子分配
- Redis 全部带 TTL，丢失后可从 MySQL 重建；已移除 MongoDB

## 已实现功能

### Agent
- `POST /agent/runs` 非流式执行、`POST /agent/runs/stream` SSE
- `GET /agent/runs/:runId` 查看 run 与 step 回放、`GET /agent/runs` 分页
- SSE 事件：`step` / `tool_call` / `tool_result` / `message` / `done` / `error`
- 每步落 `agent_steps`，支持分析「Agent 为什么跑偏」

### 面试（确定性工具）
- 简历驱动出题、AI 评分、追问规则链、状态机、运行态恢复（Redis + MySQL 快照/归档）
- 面试报告、当前题/总分/建议/雷达图

### 通用 AI 与基础设施
- SSE 流式聊天、记忆压缩、统一错误码、异步指标、优雅停机
- 用户认证（JWT + bcrypt）、上传校验

## 快速开始

### Docker Compose

```bash
cp config/config.example.yaml config/config.yaml   # 填入 MySQL/Redis 密码与 DeepSeek key
docker-compose up -d                                # App + MySQL + Redis
open http://localhost:8080
```

### 本地开发

```bash
# 1. 启动 MySQL / Redis（MySQL 需建库）
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS ai_meeting DEFAULT CHARSET utf8mb4;"
# 2. 配置
cp config/config.example.yaml config/config.yaml
# 3. 运行（启动时 AutoMigrate 自动建表）
go run main.go
```

> `ai.deepseek.api_key` 为空时，依赖模型的功能不可用；`agent.search.api_key` 为空时 `web_search` 工具不会注册。
> 文件工具只能操作 `agent.sandbox_root`（默认 `./sandbox`）内的相对路径。

## 测试与 CI

```bash
go build ./...
go test ./...
go vet ./...
scripts/knowledge-check.sh diff
```

- 单元测试覆盖：循环终止/熔断/重复动作、工具参数校验、沙箱路径逃逸、上下文预算与画像合并、路由鉴权
- 分布式组件：SingleFlight 单测 + 真实 Redis 集成测试 + 集群压测脚本
- CI：gofmt → build → vet → race 测试 → 覆盖率

## 已知边界

- 流式 run 的 SingleFlight 只做幂等，不复用 chunk（如实说明）
- 换主机制基于租约，安全性是概率性收敛，不是强一致（边界见 `docs/agent-knowledge/references/`）
- 画像「每次模型调用提炼」采用异步 + 节流，避免成本翻倍
- 网页搜索依赖外部 API key 与网络

## License

[MIT](LICENSE)
