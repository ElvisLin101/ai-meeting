# Placeholder And Drift Register

这个文件登记"看起来有接口, 但业务尚未完整实现"的位置。修改相关模块时, 先判断是否正在替换这些占位逻辑。

## Agent

- ~~`api/handlers/agent_handler.go`: `AgentController.Chat` 保存用户消息并异步触发 memory 压缩判断, 但不调用模型, 不保存 assistant 回复。~~ **已完成: Agent Chat SSE 闭环, 对接 DeepSeek, 双消息持久化。**
- ~~`services/agent/agent_service.go`: `CreateConversationWithTitle` 入参有 `agentID`, 但当前 `AgentID` 固定为 1。~~ **已完成: 创建会话支持指定 agentID(DTO 新增 `agent_id` 字段), 空默认 1, 非空时校验配置存在且启用(`resolveAgentID`), 校验失败返回 -4001。**
- ~~`services/agent/agent_service.go`: `GetConversationHistoryWithContext` 已接 memory, 但 handler 未使用。~~ **已移除: 该函数已删除, Agent 侧不再使用压缩记忆(见下方"Agent 侧已移除压缩机制")。**

## AI

- `api/handlers/ai_handler.go`: `AiMessageController.ChatStream` 已接 SSE, 但流式中断时不会保存不完整 assistant 回复。
- ~~`repositories/mysql/message_repository.go`: `AiMessage.Sequence` 通过查询当前最大值后加一生成, 高并发同会话写入时可能重复。~~ **已完成: 统一 `messages` 与 `TurnArchive` 的 seq 改用 MySQL `session_sequences` 原子分配(`LAST_INSERT_ID` 技巧), 并发不重复; 相关唯一索引见 `repositories/mysql/database.go` AutoMigrate。**
- `clients/ai_model_client.go`: 模型调用按 OpenAI-compatible endpoint 解析, 非兼容 provider 需要适配。
- `config/config.yaml`: `ai.deepseek.api_key` 当前预留为空, 本地运行真实模型前需要填入或通过 `AI_DEEPSEEK_API_KEY` 覆盖。

## Interview

- `services/interview/interview_service.go`: `ExtractInterviewQuestions`, `AnswerInterviewQuestion`, `GetNextQuestion`, `GetCurrentQuestion`, `RestoreInterviewSession`, `GetSessionInterviewQuestions`, `GetSessionTotalScore`, `GetSessionInterviewSuggestions`, `GetSessionResumeScore`, `GetRadarChartData` 已接入真实实现。神态评估已移除。
- ~~`services/interview/interview_service.go`: `SaveInterviewRecordFromRedis` 是空实现。~~ **已完成: 从 MySQL `TurnArchive` 汇总轮次算平均分, 取最后一轮作报告概要, 写入 `InterviewRecord`。**
- ~~`api/handlers/interview_handler.go`: `PreviewResume` 只返回固定提示。~~ **已完成: 从 MySQL 读 `ResumePath` → 解析 PDF 返回文本。**
- ~~`InterviewSessionFacade.CreateSession` 写 `InterviewSession`, 但 `PageConversations` 读 `AgentConversation`。~~ **已完成: `CreateSession` 双写 AgentConversation 列表条目(标题"模拟面试"), 保证新建面试会话在列表中可见; 列表条目创建失败时补偿删除 InterviewSession, 保持要么全成功要么失败。**

## Memory

- `services/ai/ai_memory_service.go`: `SetCompressionThreshold` 是运行时内存配置, 服务重启后恢复默认阈值。
- ~~`services/ai/ai_memory_service.go`: 只做当前进程内 `sync.Map` 防重复压缩, 多实例部署时仍可能并发压缩同一 AI 会话。~~ **已完成: 已接入分布式 SingleFlight, 全集群同一 session 只压缩一次。**
- Agent 侧已移除压缩机制, 不再涉及压缩并发或阈值风险; 上下文未来由状态机管理。

## User/Auth

- ~~`services/user/user_service.go`: 密码明文比较。~~ **已完成: 注册 bcrypt 哈希存储; 登录兼容旧明文(`verifyPassword` 检测非 bcrypt 前缀直接比较)并成功后自动迁移为哈希。**
- ~~`api/middleware/auth.go`: 缺失或无效 token 默认放行。~~ **已完成: 受保护路由统一挂 `RequireAuth`(见 `api/routes/routes.go` 的 `authed` 组), 无有效 JWT 直接 401; 公开接口仅 login/register/check-login/is-admin/has-username。**
- ~~`api/handlers/user_handler.go`: 管理员设置缺少操作者权限校验。~~ **已完成: `AddAdmin` 要求操作者为已登录管理员, 否则 `-403`。**

## 新增基础设施（无占位）

- `services/agent/agent_scene.go`: 4 个业务场景枚举, 已完整实现, 尚未被面试模块使用。
- `services/agent/agent_properties_loader.go`: 启动缓存 + 场景解析器, 已完整实现。
- `pkg/singleflight/singleflight.go`: 分布式 SingleFlight, 已完整实现; 流式心跳已接入 AI 侧压缩路径（`AiMemoryService` 压缩走 `CallConfiguredAIChatStream`, `onChunk` 内调 `writer.Write` 刷新 `progressKey` 时间戳, follower 据此判停滞换主）。Agent 侧不压缩, 不走 SingleFlight。
- `pkg/singleflight/singleflight.go` 换主竞态(已修复): 新 leader 抢锁成功后清空 `progressKey`, 但 fn 首段输出前存在空窗口, 迟到的旧 follower 会误判新 leader 卡死并 cancel 它, 引发连环换主。**修复**: `runAsLeader` 清理后立即写占位进度 `0:<now>`(TTL LockTTL*2), 消除空窗口。集群压测 `cmd/sfcluster` 暴露此 bug, 修复后换主场景 exec=2 且 0 失败。
- `cmd/sfcluster/main.go`: 集群(多实例)压测工具, coordinator + N 个独立 worker 进程共享一个 Redis, 验证跨实例去重(exec=1)与换主故障转移(exec=2)。一键脚本 `scripts/sfcluster-run.sh`。
- `repositories/redis.go`: 全局 `SingleFlight` 实例, 在 `InitRedis` 中初始化。

替换任意占位逻辑后, 从本文件移除或改写对应条目。

## Agent 模块（新增）

- `services/agent/loop/loop.go`: 手写 ReAct 循环, 终止条件(最终回答/max_steps/重复动作/连续工具失败/超时)已实现。
- `services/agent/tools/`: 工具注册/执行(JSON Schema 校验、超时、panic 恢复、结果截断、审计) + PDF/搜索/沙箱文件/面试工具。
- `services/agent/tools/batch.go`: 单步 tool call 批调度; 全部工具声明 `ConcurrencySafe` 时并行, 否则整批串行; 结果按原始 call 顺序回填, 失败隔离。
- `services/agent/plan/`: Plan-and-Execute 编排 (planner/scheduler/service/replanner/ledger); 规划失败回退线性 ReAct; 任务图与结果落 `agent_runs.plan_json`; tool 任务强制只读; replan 在轮次边界串行应用, 受 `max_replans`/计划哈希约束; 幂等账本仅在单 run 进程内有效, 不支持重启续跑。
- `services/agent/context/`: 四层上下文(短期/远期压缩/用户画像/Agent 自我定位) + 画像异步提炼 + 压缩。
- `services/agent/run/service.go`: run 编排、MySQL 落库、SSE 事件、SingleFlight 去重/幂等锁。
- `repositories/mysql/`: 会话/消息/压缩快照/画像/身份/run/step/面试行模型。
- `repositories/redis_hot.go`: 热数据 key 与 MySQL 回填。
- 已知边界: 流式 run 的 SingleFlight 仅做幂等(不复用 chunk); 画像提炼为异步+节流; 换主机制为概率性收敛; 工具批调度尚无 `depends_on` 依赖图与 async 模式。
