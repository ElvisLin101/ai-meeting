# Data Models Map

存储架构: **MySQL 为真相源（全量历史/恢复源）+ Redis 为热数据（可丢失可重建）**。已移除 MySQL。

## User

`models/user.go`

- `User` -> `users`
- 关键字段: `ID`, `Username`, `Password`, `Email`, `Phone`, `IsAdmin`, `Status`, `CreatedAt`, `UpdatedAt`
- 注意: 密码为 bcrypt 哈希；历史明文密码登录时自动迁移。

## 统一会话与消息

`models/conversation.go`

- `Conversation` -> MySQL `conversations`
  - `SessionID`(唯一), `UserID`, `Scene`(ai/agent/interview), `AgentID`, `Title`, `Status`, `MessageCount`
  - 原 AI/Agent/面试各自的会话集合统一到此表。
- `Message` -> MySQL `messages`
  - `SessionID`, `UserID`, `Role`(user/assistant/system/tool), `Content`, `Sequence`, `Metadata`(JSON), `CreatedAt`
  - 唯一索引 `(session_id, sequence)`；`Metadata` 存 `tool_calls`/`tool_call_id`/`response_time` 等结构化扩展。
- `SessionSequence` -> MySQL `session_sequences`
  - `SessionID` 主键, `Seq`；用 `INSERT ... ON DUPLICATE KEY UPDATE seq = LAST_INSERT_ID(seq + 1)` 原子分配。

Redis 热数据（`repositories/redis_hot.go`）：

- `agent:session:{sid}:recent` List：近期对话（LTRIM 200），miss 从 MySQL 重建
- `agent:session:{sid}:summary` / `:summary_idx`：压缩摘要与边界
- `user:{uid}:profile`：用户画像 JSON
- `agent:{aid}:identity`：Agent 自我定位 JSON

## Agent 运行

`models/agent_runtime.go`

- `AgentRun` -> MySQL `agent_runs`
  - `RunID`(唯一), `SessionID`, `UserID`, `AgentID`, `Input`, `Output`, `PlanJSON`, `Status`, `StopReason`, `Steps`, `Tokens`, `Error`, 起止时间
  - `PlanJSON` 保存 Plan-and-Execute 的任务图与每个任务的最终状态；线性 ReAct 时为空。
  - `PlanJSON` 内任务包含 `idempotency_key`、`attempts`、`status`、`result`、`error`；计划包含 `version`、`replan_count`。
- `AgentStep` -> MySQL `agent_steps`
  - `RunID` + `StepIndex` 唯一；`Thought`, `Action`, `ActionInput`, `Observation`, `Status`, `DurationMs`, `Error`
  - 用于按 run 回放，回答「Agent 为什么跑偏」。
- `AgentProperties` -> MySQL `agent_properties`（配置元数据，启动缓存到 `AgentPropertiesLoader`）
- `AgentFileAsset` -> MySQL `agent_file_assets`
- `CompressedContextRecord` -> MySQL `compressed_contexts`（Agent 中期摘要）
  - 一个会话一行；再次压缩 UPDATE 同一行。
  - `Index` 是 `CoveredThroughSeq`：摘要覆盖 `sequence <= Index`，短期消息只取 `sequence > Index`，二者不重叠。
  - Redis key: `agent:session:{sessionID}:summary` / `:summary_idx`；短期消息 `agent:session:{sessionID}:recent`。

## 跨会话结构化数据

- `UserProfile` -> MySQL `user_profiles`
  - `UserID` 唯一；`ProfileJSON` 结构见 `services/agent/context/types.go` 的 `UserProfileData`
  - 字段: facts（结构化事实）/ preferences / goals / constraints / background / confidence / version / schema_version / last_consolidated_seq / updated_at
  - MySQL 是画像真相源，Redis `user:{uid}:profile` 只是缓存；L2 用 version CAS 更新。
- `ProfileCandidate` -> MySQL `profile_candidates`
  - L1 规则候选：`user_id`, `session_id`, `message_id`, `field`, `quote`, `rule`, `confidence`, `status`(pending/promoted/rejected)
  - `value_hash` 唯一，保证同一条消息同一字段不会重复入候选；L2 consolidation 才写正式画像。
- `ProfileConfirmation` -> MySQL `profile_confirmations`
  - L2 冲突/模糊项：`user_id`, `field`, `key`, `value`, `question`, `status`(pending/confirmed/rejected)
- `ProfileEvent` -> MySQL `profile_events`
  - 每次 L2 consolidation 的审计：`user_id`, `reason`, `diff_json`, `created_at`
- `AgentIdentity` -> MySQL `agent_identities`
  - `AgentID` 唯一；`IdentityJSON` 结构见 `AgentIdentityData`
  - 字段: name / role / capabilities / boundaries / interaction_style / self_reflection / version
  - 核心 persona 静态，`self_reflection` 仅低频更新，防人设漂移。

## AI

`models/ai.go`

- `AiProperties` -> MySQL `ai_properties`（配置元数据）
- `AiConversation` / `AiMessage` 为历史响应模型，数据实际存 `conversations` / `messages`（scene=ai），服务层做转换。
- AI 压缩快照 -> MySQL `compressed_contexts`（Redis 摘要 key: `memory:ai:{sessionId}:summary` / `:index`）。

## Interview

领域模型：`models/interview.go`；MySQL 行模型：`models/interview_sql.go`。

- `InterviewSessionRow` -> MySQL `interview_sessions`（`SessionID` 主键）
- `InterviewRecordRow` -> MySQL `interview_records`
- `InterviewQuestionRow` -> MySQL `interview_questions`
- `InterviewHotSnapshotRow` -> MySQL `interview_session_runtime_hot_snapshot`
  - `FlowJSON`/`FollowUpQuestionsJSON`/`RecentTurnsJSON` 以 JSON 文本存储；`SnapshotVersion` 做 CAS
- `InterviewColdSnapshotRow` -> MySQL `interview_session_runtime_cold_snapshot`
- `InterviewTurnArchiveRow` -> MySQL `interview_session_turn_archive`
  - 唯一索引 `(session_id, request_id)` 幂等、`(session_id, seq)` 单调；seq 由 `session_sequences`（前缀 `turn:`）分配
- 面试会话列表复用 `conversations`（scene=interview）

## MetricLog

`models/metric_log.go`

- `MetricLog` -> MySQL `metric_logs`
- 关键字段: `Module`(ai_call/singleflight/state_machine/snapshot/repair/idempotency/lock/agent/agent_tool), `Event`, `SessionID`, `Success`, `ErrorType`, `IsRetry`, `DurationMs`, `Extra`(JSON)

## Repository Clients

- MySQL: `repositories/mysql.DB`
- Redis: `repositories.RedisClient`
- AI provider config: `config.AppConfig.AI`
- Agent 运行时配置: `config.AppConfig.Agent`（max_steps/tool_timeout/sandbox_root/search 等）

改任一模型字段或表名时, 同步更新对应 Skill。
