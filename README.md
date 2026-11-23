# AI-Meeting

![CI](https://img.shields.io/github/actions/workflow/status/ElvisLin101/ai-meeting/ci.yml?branch=main&label=CI)
[![Go Version](https://img.shields.io/badge/Go-1.24.1-00ADD8?logo=go&logoColor=white)](https://go.dev/)

> 一个面向长任务的自主 Agent 后端：手写 ReAct 循环 + Function Calling 工具编排 + 四层上下文管理，并用 Redis 分布式 SingleFlight 做跨实例请求合并、主从选举、流式心跳和自动换主。
> 求职/面试是示例领域；结构化模拟面试作为「确定性流程工具」接入，用来展示自主编排与确定性编排的边界。
> 不依赖任何 Agent 框架，存储统一为 Redis（热数据）+ MySQL（真相源 / 全量历史 / 恢复源）。
> 文档最近按代码核对于 2026-11-23。SingleFlight 与面试恢复仍有实现边界，见下方已知边界和风险登记。

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
│   │   ├── callback/       # 生命周期元数据回调与日志
│   │   ├── loop/           # ReAct 循环（终止条件/熔断/工具调度）
│   │   ├── plan/           # 规则路由、任务图、replan、单 run 幂等
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
├── repositories/mysql/    # MySQL 真相源仓储
├── repositories/redis_hot.go # Redis 热数据（按模块回填）
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
- ctx 取消 / ReAct loop 超时（默认 120s）；Plan 整体未共享这个 deadline

**工具执行统一收口**：JSON Schema 校验（required + 顶层类型）、单工具超时、panic recovery、结果 32KB 字节预算截断（按 UTF-8 边界，不切坏多字节字符）、审计日志。

**单步工具批调度**：只有整批工具都声明 `ConcurrencySafe` 才并行，任一工具未声明（如写文件、面试状态推进）则整批串行。结果按原始 call 顺序回填，单个工具失败不取消同批兄弟调用。并行上限由 `agent.max_parallel_tools` 控制（默认 4）。

**Plan-and-Execute（可选）**：复杂任务先由 planner 生成带 `depends_on` 的任务图，再按依赖并行执行；每个任务内部复用 ReAct（agent）、单次只读工具（tool）或纯文本推理（llm），最后统一合成。规划失败/不需要计划时自动回退线性 ReAct。配置见 `agent.plan.*`，任务图与结果持久化在 `agent_runs.plan_json`。

**Replan 与重试（谨慎模式）**：任务失败或返回 `ReplanHint` 时，调度器在一批任务结束后统一评估，多个失败合并为一次 replan；计划修改只在轮次边界单线程串行应用，已完成任务冻结。diff 必须通过依赖/环/只读工具校验，并受 `max_replans` 和计划哈希震荡检测约束。每个任务有基于语义的 `idempotency_key`，同 key 的成功结果在本次 run 内复用；失败任务只有 `Retryable=true` 才重试（默认最多 2 次，指数退避），调用过有副作用工具的 agent 任务不会自动重试。

**前置路由**：不是每个请求都进 Plan。`services/agent/plan/router.go` 用规则打分（强多步信号、中信号、长度、多问号/多行加分；纯问候、明确要求直接回答、短且无强信号减分），只有 `score >= min_score` 才调用 planner。`agent.plan.mode=auto|always|never` 可强制开关；SSE 的 `route` 事件和返回的 `plan_mode` 字段会暴露本次选择。

**Plan 边界**：scheduler 对任务应用 `TaskTimeout`，`TotalTimeout` 仅用于最终 synthesis loop，没有覆盖规划、重规划、所有任务与合成的整体超时。tool 任务只读，agent 任务仍可调用有副作用工具；幂等账本只在单进程、单 run 内有效，不支持重启续跑。

**生命周期回调**：`services/agent/callback/` 提供 run/LLM/tool 元数据回调，默认 Logging；direct ReAct 和 Plan 的 agent/llm/synthesis 子 loop 已接入，planner/replanner 与 Plan 直调 tool 尚未接入。回调同步执行并要求并发安全，当前无 panic 隔离；tool end 在整批完成后按调用顺序触发。

**内置工具**：

| 工具 | 能力 | 安全边界 |
|---|---|---|
| `pdf_parse` | 解析沙箱内 PDF | 扩展名 + `%PDF-` 魔数 + 10MB 上限 |
| `web_search` | 搜索公开网页（DeepSeek / Tavily / Serper） | 超时、条数/长度限制、结果标记为不可信 |
| `web_fetch` | 抓取网页正文（DeepSeek Fetch，返回 Markdown） | 校验 http/https、host/显式 IP/端口；未覆盖 DNS 解析和重定向链；正文截断并标记不可信 |
| `file_list/read/write/edit` | 沙箱内文本文件操作 | 拒绝绝对路径/`..`/符号链接逃逸；原子写 + 写前备份 |
| `interview_*` | 结构化模拟面试 | 包装确定性状态机 + 会话归属校验 |

### 2. 四层上下文管理

| 层 | 内容 | 存储 |
|---|---|---|
| L1 短期 | 近期全量对话（按预算取最近消息） | Redis List（miss 从 MySQL 重建） |
| L2 远期 | 80/20 压缩摘要 | Redis + MySQL 快照 |
| L3 跨会话 | 结构化用户画像（偏好/目标/约束/背景 + 置信度） | Redis + MySQL |
| L4 自我定位 | Agent persona（角色/能力边界/风格）+ 低频自省 | Redis + MySQL |

拼装顺序：`system(persona) → 用户画像 → 压缩摘要 → 近期对话`。各 system 段先独立截断，再给近期对话剩余预算。token 用启发式估算（CJK 1/字、其他字符约 1/4、乘 1.15），不是精确 tokenizer，也不保证包含工具 schema/计划等所有输入的严格总预算。

- **画像 L1/L2**：每个用户消息先用规则筛出候选（MySQL `profile_candidates`，不调用 LLM、不直接改画像）；L2 仅在“无候选且 user 消息不足 3 条”时跳过，任意 pending 候选都可能触发，阈值常量 5 未使用。输入“当前画像 + 候选 + 最近对话”，输出增量 diff，冲突时 supersede 或写入 `profile_confirmations`，调试页已支持确认/拒绝。
- **短期/中期边界**：`compressed_contexts.index` 是 `CoveredThroughSeq`；摘要覆盖 `sequence <= index`，短期只取 `sequence > index`，二者不重叠。
- **压缩**：只压缩对话消息，不压缩 identity/profile/plan/tool schema；按 token 触发，压缩较旧的 80%，保留最近 20% 原文（有最小尾部），增量合并旧摘要，避免每次重压全部。
- **存储**：短期 Redis List + MySQL 全量消息；中期 Redis 摘要 + MySQL 一会话一行；长期 Redis JSON + MySQL `user_profiles.profile_json`（带 version CAS），候选/确认/审计分别落 MySQL。
- **ContextReport**：每次根上下文装配记录预算、各段 token、丢弃条数、摘要边界和画像事实数，写入 `agent_runs.context_report_json`，run 详情可查。
- **恢复与后台**：Redis recent miss 的 MySQL 回填最多 500 条；Push 不 LTRIM，压缩成功后重建 tail。候选/压缩/合并由 goroutine + cancel/WaitGroup 管理，无固定 worker 或并发上限；AI 压缩与旧画像提取不在其停机等待范围。
- **配额/限流**：`agent.quota.max_concurrent_runs` 控制单用户并发 run，`daily_token_limit` 控制每日 token；Redis 计数，超限返回 `-4012` / 429；Redis 故障时降级放行，不阻断主链路。

### 3. 确定性编排工具：状态机不删，降级为工具

模拟面试仍由显式状态机驱动（`INIT/ASKING/EVALUATING/FOLLOW_UP/COMPLETED` + CAS 乐观锁 + 条件回滚 + 幂等三层防线）。

Agent 可调用结构化面试工具，由显式状态机控制评分后的推进。Redis/MySQL 跨存储写入仍有失败窗口；当前热快照命中只恢复 flow/score/follow-up，不继续补齐冷材料/turn，没有恢复等级和刷新协调器，详见 [面试运行态](docs/agent-knowledge/references/interview-runtime-governance.md)。

### 4. 分布式 SingleFlight — 跨实例 Agent run 去重

**问题**：Agent run 动辄数十秒，负载均衡下前端网络抖动或重试会让同一用户同一 prompt 的请求分散到不同实例；每个实例各跑一次 Agent 和模型调用，成本翻倍。

**方案**：基于 Redis `SET NX` + Pub/Sub 实现跨实例请求合并。

```
实例A 抢到锁 → 成为主节点 → 执行 Agent run → 写结果 → Pub 通知
实例B 没抢到 → 成为从节点 → Sub 等待 → 读结果复用
```

**关键设计**：

- **主从选举**：Redis `SET NX` 抢锁，第一个成功的是主节点，其余为从节点
- **流式进度**：AI 压缩与面试出题用 writer 写 Redis 进度（`字节数:时间戳`），30s 无变化可触发接管；Agent run 与画像合并未更新 writer，长任务可能被误判
- **接管尝试**：取消信号通过 Pub/Sub + cancelKey 轮询传递，旧主依赖执行函数响应 context 取消；当前续租和结果写入 ownership/fencing 不完整，不能保证旧主隔离
- **降级策略**：Redis 不可用时自动降级为本地 SingleFlight（`sync.Mutex` + `map` + `WaitGroup`）
- **流式/非流式分流**：非流式 run 走 SingleFlight 合并；流式 run 用独立 Redis 幂等锁拒绝重复提交，不共享流式 chunk

**代码位置**：`pkg/singleflight/singleflight.go`

**历史模拟测量（本次未重跑）**：`cmd/sfprobe` 对真实 Redis 发起 N 个并发同 key 请求，底层执行为约 300ms 的模拟 AI 流式函数：

| 并发数 | 实际执行次数 | 去重率 | 结果一致 | 失败 | 总耗时 |
|---|---|---|---|---|---|
| 100 | 1 | 99.0%（省 99 次） | 100/100 | 0 | ~362ms |
| 500 | 1 | 99.8%（省 499 次） | 500/500 | 0 | ~404ms |

总耗时约等于单次底层调用耗时，说明并发请求被真正合并；事件统计 `leader_elected=1 / follower_waiting=N-1 / leader_completed=1`，无换主（leader 健康）。复现：

```bash
go run ./cmd/sfprobe --addr localhost:6379 --password 123456 --concurrency 500
```

**历史集群模拟测量（本次未重跑）**：`cmd/sfcluster` 启动 N 个独立 worker 进程（独立 DistributedGroup / nodeID），共享一个 Redis；结果只覆盖脚本的模拟执行场景：

| 场景 | 实例数 | 每实例并发 | 总请求 | 实际执行 | 结果一致 | 失败 | 总耗时 | 执行实例 |
|---|---|---|---|---|---|---|---|---|
| 去重 | 3 | 50 | 150 | 1 | 150/150 | 0 | ~0.9s | 单实例 |
| 换主（首个 leader 假死 40s） | 3 | 50 | 150 | 2 | 150/150 | 0 | ~30.5s | 假死实例 + 接管实例 |

这些历史数据展示了模拟场景的跨实例合并和接管。旧/新 leader 返回相同结果，且旧主响应取消，不能证明旧主被隔离、不响应 cancel 时安全或所有业务路径可靠。复现：

```bash
# 去重场景
bash scripts/sfcluster-run.sh --instances 3 --concurrency 50 --password 123456
# 换主场景（首个 leader 假死触发换主，约 30s）
bash scripts/sfcluster-run.sh --instances 3 --concurrency 50 --password 123456 --failover-worker 0
```

> 新 leader 初始占位进度减少首段输出前的误判窗口；follower 缓存命中状态、请求 context 继承、续租错误、结果 ownership 与 panic 清理仍有缺口，见 [风险登记](docs/agent-knowledge/references/placeholder-risk-register.md)。

### 5. 存储：MySQL 真相源 + Redis 热数据

```
Agent 消息/摘要/画像持久化: MySQL → Redis 热缓存
AI 摘要: Redis → 独立 goroutine upsert MySQL
热数据读取: Redis → miss → MySQL 按各模块逻辑回填
面试答题: Redis 推进 → 轮次归档/快照刷新（存在失败窗口）
```

- 会话/消息/压缩快照/画像/身份/run/step/面试数据全部落 MySQL
- 序列号用 `session_sequences` 的 `INSERT ... ON DUPLICATE KEY UPDATE seq=seq+1` 原子分配
- Redis 热数据使用 TTL，恢复能力取决于模块和检查点完整性；已移除 MongoDB

## 已实现功能

### Agent
- `POST /agent/runs` 非流式执行、`POST /agent/runs/stream` SSE
- `GET /agent/runs/:runId` 查看 run 与 step 回放、`GET /agent/runs` 分页
- `GET /agent/memory/confirmations` 查询待确认画像项、`POST /agent/memory/confirmations/:id/resolve` 确认/拒绝
- SSE 事件：`route` / `step` / `tool_call` / `tool_result` / `message` / `plan_*` / `done` / `error`
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
# 3. 运行（启动时 AutoMigrate 自动建表；迁移失败会阻断启动）
go run main.go
```

> `ai.deepseek.api_key` 为空时 config fallback 不可用，绑定有效 MySQL `AiProperties` 的 AI 会话仍可使用该配置。搜索 provider 默认 `deepseek`，`agent.search.api_key` 留空会复用 `ai.deepseek.api_key`，同时启用 `web_search` 与 `web_fetch`；切到 Tavily/Serper 时需单独配置 `agent.search.api_key`，且只启用 `web_search`。
> 文件工具只能操作 `agent.sandbox_root`（默认 `./sandbox`）内的相对路径。

## 测试与 CI

```bash
go build ./...
go test ./...
go vet ./...
go run ./cmd/eval          # 确定性 eval：路由/画像规则/画像冲突
go run ./cmd/liveeval      # Live eval：真实模型 + MySQL/Redis
go run ./cmd/liveeval -judge  # 追加 LLM 逐条语义评分
scripts/knowledge-check.sh diff
scripts/knowledge-check.sh full
```

`cmd/liveeval` 默认 `-agent-id 0`，模型配置来自 `config/config.yaml` 的 `ai.deepseek`；`-agent-id >0` 时改用 MySQL `ai_properties` 里的启用配置。key 也可用 `AI_DEEPSEEK_API_KEY` 覆盖。未配置 key 时会在启动前直接报错，不会空跑用例。

默认 `-isolate=true`：每轮生成独立 eval 用户（`eval-<timestamp>-<pid>`）和独立 sandbox（`tmp/eval-runs/<label>/sandbox`），每个用例前清空 sandbox，避免文件、画像和历史 run 跨轮污染。需要复用固定用户/沙箱时用 `-isolate=false -username <name> -user-id <id>`。用例可用 `seed_files` 预置简历/JD 等夹具；plan 用例用 `max_plan_steps` 限制总步数，direct 用 `max_steps`。报告输出 `completed`、`quality_pass`、`avg_quality`，开启 `-judge` 后额外输出 `judge_pass`、`avg_judge_score`；关键词 quality 默认阈值 `-quality-threshold 0.7`。

- 单元测试覆盖：循环终止/熔断/重复动作、工具参数校验、沙箱路径逃逸、上下文预算与画像合并、路由鉴权
- 确定性 eval：15 条路由、10 条画像规则、5 条冲突用例，CI 直接跑
- Live eval：`cmd/liveeval` 用真实模型跑 10 条任务，输出 direct/plan 成功率、步数、token、延迟到 `tmp/live_eval_report.json`；默认隔离用户和 sandbox，需要 `config/config.yaml`、MySQL、Redis 和模型 key，不进 CI
- 分布式组件：SingleFlight 单测 + 真实 Redis 集成测试 + 集群压测脚本
- CI：gofmt → build → vet → race 测试 → eval → 覆盖率（低于 35% 失败）
- 防腐：diff 合并暂存、未暂存、未跟踪文件并去重推荐文档；full 检查必要文件和 Go 文件锚点。CI 当前未执行知识检查；脚本不验证业务语义，需按 [AGENTS.md](AGENTS.md) 用代码反向核对。

## 已知边界

- 流式 run 使用独立幂等锁；非流式共享执行继承首请求 context，断连可能影响 follower
- SingleFlight 缓存轮询、续租/ownership/fencing 及 panic 清理有具体缺口，历史模拟测量不构成可靠性保证
- 面试基础恢复尚不完整；热快照分数累计、出题内容哈希/follower 类型解码需治理
- 统一会话删除的子消息仅按 session_id 删除，仓储归属约束需修复
- 普通 AI 压缩按字节阈值，Agent 按估算 token；向量召回尚未实现
- 画像异步合并的候选阈值 5 未生效；后台管理没有固定并发上限
- 网页搜索/抓取依赖外部 API、额度与网络；DeepSeek Search/Fetch 与模型共用 API key，但按请求单独计费

## License

[MIT](LICENSE)
