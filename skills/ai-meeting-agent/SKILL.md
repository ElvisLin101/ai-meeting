---
name: ai-meeting-agent
description: 当需求涉及 Agent 会话、Agent Chat、AgentProperties、AgentMessage、文件上传、场景绑定或 Agent 历史消息时使用。
---

# AI-Meeting Agent Skill

## 何时使用

读取本 Skill 的场景:

- `/agents/**` 接口变更。
- Agent 会话创建、结束、分页、历史消息查询。
- Agent Chat SSE 流式聊天、接入 DeepSeek、双消息持久化。
- Agent 配置 CRUD、启动缓存、场景绑定热插拔。
- Agent 文件上传和 `AgentFileAsset` 持久化。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupAgentRoutes`。
- Handler: `api/handlers/agent_handler.go`。
- Service: `services/agent/agent_service.go`。
- 场景枚举: `services/agent/agent_scene.go`（4 个 BusinessAgentScene + 候选名称）。
- 启动缓存 + 场景解析器: `services/agent/agent_properties_loader.go`（sync.Map 内存缓存 + miss 查库 + ResolveRequired）。
- 模型客户端: `clients/ai_model_client.go`（CallConfiguredAIChatStream, DeepSeek/OpenAI 兼容）。
- MySQL 仓储: `repositories/mysql/agent_properties_repository.go`, `repositories/mysql/agent_file_asset_repository.go`。
- MySQL 仓储: `repositories/mysql/conversation_repository.go`(会话), `repositories/mysql/message_repository.go`(消息)。
- DTO: `dto/agent.go`。
- 模型: `models/agent.go`（AgentProperties 含 ApiKey/ApiSecret/ApiFlowId；AgentMessage 含 ResponseTime/ErrorMessage）。
- 记忆: 旧 `AgentChat` 链路不使用压缩记忆；当前 `Agent Run` 链路使用 `services/agent/context/` 的压缩摘要，两者不要混用。

### Agent Run 执行链（新，独立于上面的 AgentChat）

- 路由: `api/routes/routes.go` 中 `setupAgentRunRoutes`。
- Handler: `api/handlers/agent_run_handler.go`（`POST /agent/runs`、`POST /agent/runs/stream`）。
- 编排: `services/agent/run/service.go`（会话归属 → 消息落库 → 四层上下文 → ReAct loop → run/step 落库 → 异步压缩/画像）。
- 循环: `services/agent/loop/loop.go`（终止条件/熔断/工具批调度）。
- 工具: `services/agent/tools/`（`services/agent/tools/tool.go` 契约与 ToolMeta、`services/agent/tools/executor.go` 单工具收口、`services/agent/tools/batch.go` 单步批调度）。
- 上下文: `services/agent/context/`（token 预算 + 四层上下文 + 画像 + 压缩）。
- 注意: 本节是当前 Agent 主链路；上面的 `AgentChat` 是旧 SSE 会话链路，两者不要混用。

### 上下文与记忆（token 预算版）

- 预算: `services/agent/context/tokens.go` 做保守估算（CJK 1 token/字，ASCII 4 字符/token，1.15 安全系数）；`services/agent/context/assembler.go` 按 token 组装 identity/profile/summary/recent，近期消息从最新往前选，放不下就停（最新一条过大才截断）。
- 边界: `compressed_contexts.index` 是 `CoveredThroughSeq`；摘要覆盖 `sequence <= index`，短期只取 `sequence > index`，二者不重叠（`services/agent/context/store.go` 的 `LoadRecentMessagesAfter`）。
- 压缩: `services/agent/context/compress.go` 只压缩对话消息，不压缩 identity/profile/plan/tool schema；按 token 触发，压缩较旧的 80%，保留最近 20%（最小 600 token），增量合并旧摘要；成功后用 tail 重建 Redis 近期列表。
- 存储: 短期 Redis List + MySQL `messages` 全量；中期 Redis summary + MySQL `compressed_contexts`（一会话一行，UPDATE）；长期 Redis JSON + MySQL `user_profiles.profile_json`。
- Redis 裁剪不变量: `PushRecentMessage` 不再 LTRIM；未摘要覆盖的消息不能从 Redis 丢弃，压缩推进边界后再重建列表。
- 配置: `agent.context.*`（model_window_tokens / max_output_tokens / soft_limit_tokens / reserve_tokens / profile_max_tokens / summary_max_tokens / recent_max_tokens / compress_threshold_tokens）。
- ContextReport: `services/agent/context/assembler.go` 产出 `Report`（预算/各段 token/丢弃条数/covered seq/画像事实数），`services/agent/run/service.go` 写入 `agent_runs.context_report_json`，run 详情接口透出。
- L1 候选: `services/agent/context/profile_rules.go` 在每个用户消息落库后异步跑规则，写入 MySQL `profile_candidates`（不调用 LLM、不改正式画像）；`value_hash` 去重。
- L2 consolidation: `services/agent/context/profile_consolidate.go` 每 3 个用户回合或 pending 候选 ≥5 触发；输入当前 active facts、候选和最近 user/assistant 对话，输出 `ProfileDiff`；`services/agent/context/profile_apply.go` 负责 add/update/supersede/ask_confirm，冲突低置信不覆盖；`user_profiles` 用 version CAS 更新，确认项落 `profile_confirmations`，审计落 `profile_events`。
- 画像注入: `services/agent/context/assembler.go` 的 `renderProfile` 只渲染 active facts；旧 `ProfileExtractor` 已不再由 run 服务调用。
- 当前边界: 确认项已有查询/确认接口和调试页入口；工具大结果尚未转 artifact ref。
- 确定性 eval: `internal/eval/cases.json` + `go run ./cmd/eval`，覆盖路由（15）、画像规则（10）、画像冲突（5）；CI 在 race 测试后执行。
- Live eval: `test/eval/live_cases.json` + `go run ./cmd/liveeval`，用真实模型和 MySQL/Redis 跑 10 条任务，输出 `tmp/live_eval_report.json`；默认 `-isolate=true` 使用独立 eval 用户和 `tmp/eval-runs/<label>/sandbox`，每个用例前清空 sandbox；用例支持 `seed_files` 夹具和 `max_plan_steps`，`-judge` 开启 LLM 逐条语义评分；统计 `completed` / `quality_pass` / `avg_quality` / `judge_pass` / `avg_judge_score`；需要配置和 key，不进 CI。
- 配额: `services/agent/run/quota.go` 用 Redis 计数控制单用户并发 run 与每日 token；`agent.quota.*` 配置，0 表示不限制；超限返回 `-4012`/429，Redis 故障降级放行。
- 前端: `static/index.html` Agent tab 支持查看 run 详情（`context_report_json`/`plan_json`）和待确认画像的确认/拒绝。

### 单步工具批调度（BatchExecutor）

- 入口: `services/agent/loop/loop.go` 中 `l.batch.Execute(runCtx, tc, resp.ToolCalls)`。
- 策略: 整批工具都实现 `MetaProvider` 且 `Meta().ConcurrencySafe=true` → 并行（信号量限制 `agent.max_parallel_tools`）；否则整批串行。
- 结果: 按原始 `ToolCalls` 顺序返回，`tool` message 的 `tool_call_id` 与模型返回的 call id 对齐；单个失败不取消同批其他调用。
- 事件: `tool_call` / `tool_result` 新增 `CallID`，便于前端在同一步多个并发调用时关联。
- 只读且无共享状态的工具（`pdf_parse`、`file_list`、`file_read`、`web_search`、`web_fetch`）已声明 `ConcurrencySafe=true`；写文件、面试状态推进默认不声明，保持串行。
- 搜索/抓取: `web_search` 支持 `deepseek` / `tavily` / `serper`；`deepseek` 复用 `ai.deepseek.api_key`，搜索走 `/v1/tools/search`，`web_fetch` 走 `/v1/tools/fetch` 返回 Markdown。`web_fetch` 只放行公网 http/https，拒绝 localhost/内网/元数据 IP/非标准端口，正文截断 12000 rune 并标记不可信。
- 当前边界: 这是"并行批次"，不是带 `depends_on` 的完整 DAG；有先后依赖的调用必须由模型分步发起，后续再引入 GraphScheduler。

### Plan-and-Execute 编排（plan 包）

- 入口: `services/agent/run/service.go` 在创建 run 后先调用 `services/agent/plan/service.go` 的 `Run`；返回 `nil` 表示本次不进入 plan 模式，回退线性 ReAct。
- 规划: `services/agent/plan/planner.go`。`ShouldPlan` 用关键词/长度判断；`Plan` 调 `clients.CallConfiguredAIChat` 输出 JSON，并校验 id/依赖/环/任务数/工具只读。
- 调度: `services/agent/plan/scheduler.go`。ready-set + 有界并行；依赖非 done → skipped；单任务失败不取消独立分支；支持单任务超时与 ctx 取消。
- 执行: `services/agent/plan/service.go`。agent 任务用独立 ReAct loop；tool 任务只允许只读工具；llm 任务无工具单次推理；最后用空工具 loop 合成最终回答。
- 事件: `route` / `plan_ready` / `plan_task_start` / `plan_task_done` / `plan_task_failed` / `plan_task_skipped` / `plan_replanned` / `plan_done`，复用 `services/agent/loop/loop.go` 的 `Event`。
- 前置路由: `services/agent/plan/router.go` 用规则打分决定是否进 plan，不调用 LLM。强多步信号/中信号/长度/多问号/多行加分；纯问候、明确“直接回答”、短且无强信号减分。`services/agent/run/service.go` 只有 `UsePlan=true` 才调用 `planSvc.Run`，并发送 `route` 事件；`StartResult.PlanMode` 返回 `plan` / `plan_fallback` / `direct`。
- 路由配置: `agent.plan.mode`（auto/always/never）、`agent.plan.min_score`、`agent.plan.fast_path_runes`。规则是启发式的，边界请求可能欠/过触发；需要更准时可加 cheap classifier，但不要给每个请求都加一次模型调用。
- 持久化: `agent_runs.plan_json` 保存任务图与每个任务的最终状态；plan 子任务的 step 同时写入 `agent_steps`，用 `task_id` 标识所属子任务（最终合成为 `synthesis`），`step_index` 由 run 级原子序号统一分配，避免并行任务/重试撞唯一键；重试会为同一 `task_id` 追加 step 行；详情接口同时返回 `plan_json` 和 `step_detail`。
- Replan 安全机制:
  - `services/agent/plan/scheduler.go` 的 `RunRound` 在一批任务结束后统一评估失败/hint，多个触发合并成一次；计划修改只发生在轮次边界，由 `services/agent/plan/service.go` 单线程串行应用，不存在并发改 Plan。
  - `services/agent/plan/replanner.go` 只产出增量 `PlanDiff`（add/update/remove），不返回整张新计划；`ApplyDiff` 拒绝修改/删除已完成任务。
  - `PlanDiff` 必须再通过 `Validate`（依赖/环/任务数/只读工具）和计划哈希震荡检测；受 `agent.plan.max_replans` 限制。
  - 被 update/remove 的任务会清除旧结果；新增任务重新生成幂等 key。
- 幂等与重试:
  - `services/agent/plan/ledger.go` 是单 run 内的幂等账本：同 `idempotency_key` 并发只跑一次，成功结果缓存供 replan 复用，失败不缓存以允许重试。
  - `idempotency_key` 由任务语义（type/tool/goal/params/上游 key）计算，模型不能指定。
  - `services/agent/plan/service.go` 只对 `Retryable=true` 的失败重试；默认 `agent.plan.retry_max_attempts=2`、指数退避；调用过非只读工具的 agent 任务不自动重试。
- 当前边界: 幂等账本只在进程内、单 run 内有效，进程重启后无法续跑；任务级结果同时落在 `plan_json` 和 `agent_steps`（按 `task_id` 分组），但没有独立 `agent_plan_tasks` 表；replan 只处理失败/hint，不做每步评估。

## 核心流程

`AgentSessionCreate`

- 路由: `POST /api/xunzhi/v1/agents/sessions`。
- Handler 从 JWT 上下文读取 `username`。
- Service `CreateConversationWithTitle` 生成 UUID 作为 `session_id`。
- 标题默认 `New Conversation`, 首条消息超过 50 字符时截断。
- `agentID` 从请求体 `agent_id` 读取(可选, 空默认 "1"), `resolveAgentID` 校验配置存在且启用后入库, 校验失败返回 `-4001`。

`AgentChat`（已实现 SSE 流式闭环）

- 路由: `POST /api/xunzhi/v1/agents/sessions/:sessionId/chat`。
- Handler 设置 SSE 响应头（`Content-Type: text/event-stream`）。
- Service `AgentChatSSE` 完整流程:
  1. 会话归属校验（`GetAgentConversationBySessionId`）。
  2. 解析智能体配置（`AgentPropertiesLoader.GetByAgentID`，先查 sync.Map 缓存 miss 查库）——**仅用于场景绑定, 不再校验星辰凭证**(apiKey/apiSecret/apiFlowId 校验已移除)。
  3. 保存用户消息到 MySQL（`SaveAgentMessage`）。
  4. 加载历史消息 → 构建 `PromptMessage` 数组（system prompt + 历史对话 + 当前用户消息）。
  5. 调用 `clients.CallConfiguredAIChatStream`（DeepSeek SSE 流式, aiID=0 走 config fallback），通过 `onChunk` 回调 `ctx.SSEvent("message", chunk)` 推送前端。
  6. 保存 assistant 回复（`SaveAgentMessageWithDetail`，含 responseTime 和 errorMessage）。
  7. 更新会话消息计数（`UpdateAgentConversationMessageCount`）。
- 出错时也保存一条错误 assistant 消息（content="Sorry, an error occurred..."，errorMessage=err.Error()）。
- 最终发送 `ctx.SSEvent("end", "[DONE]")`。

`AgentHistory`

- `GET /agents/conversations/:sessionId/messages` 从 MySQL 按 `sequence ASC` 返回完整历史。
- `GET /agents/messages/history` 从 MySQL 按 `created_at DESC` 分页。
- 两个查询都必须保留用户隔离条件。

`AgentProperties`

- 表 `agent_properties` 保存名称、描述、配置、启用状态、apiKey、apiSecret、apiFlowId。
- `GetByPage` 当前固定 `Limit(10)`, 没有使用请求分页参数。
- 启动时 `AgentPropertiesLoader.RefreshActiveAgents` 全量加载到 sync.Map。
- `GetByAgentID` / `GetByAgentName` 先查缓存 miss 查库。
- `ResolveRequired(scene)` 按场景候选名称顺序匹配，支持热插拔。

`BusinessAgentScene`（场景枚举）

- 4 个场景: GeneralAgentChat / InterviewQuestionExtraction / InterviewAnswerEvaluation / InterviewQuestionAsking。
- 每个场景有默认名称 + 别名列表（如评分官: "用户答案评分官" / "面试答案评分官"）。
- `ResolveRequired` 按候选名称顺序从缓存查找，支持运营改数据库不改代码。

`AgentFileUpload`

- 路由: `POST /api/xunzhi/v1/agents/files/upload`。
- 保存路径: UUID 重命名 + 仅保留原扩展名(`filepath.Ext`), 写入 `./uploads/`, 然后写 `agent_file_assets`——已防路径穿越与重名覆盖。
- 上传有 10MB 大小限制(`http.MaxBytesReader`)。类型校验(magic bytes)与消费链路仍未实现。

## 影响检查

- 修改 Agent Chat 时, 同步检查消息计数、会话标题、错误重试和 assistant 消息落库。
- 修改 `AgentMessage.Sequence` 时, 检查 `repositories/mysql/message_repository.go`。
- 修改 Agent 消息存储时, 同时检查 `repositories/mysql/message_repository.go`。
- 修改 Agent 表字段时, 更新 `docs/agent-knowledge/references/data-models.md`。
- 修改路由时, 更新 `docs/agent-knowledge/references/routes-map.md`。
- 修改场景枚举或解析器时, 更新本 Skill 的 BusinessAgentScene 部分。

## 当前风险

- `CreateConversationWithTitle` 忽略传入的 agentID, 固定写 1。
- ~~上传文件路径没有净化。~~ **已完成: UUID 重命名防路径穿越 + 10MB 大小限制; 类型校验(magic bytes)未做。**
- 多个 Service 单例不是并发安全初始化, 当前代码只是简单懒加载。
- `AgentPropertiesLoader` 使用 sync.Map 单实例缓存, 多实例部署时配置变更需重启或手动刷新。
- `GetByPage` 固定查 10 条, 未使用分页参数。
