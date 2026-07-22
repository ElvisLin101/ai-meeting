# 项目能力演进清单

最后整理: 2026-07-22

## 目的

本文档盘点 AI-Meeting 项目的能力覆盖现状与待办，作为功能演进清单。每节按「目标能力 / 当前实现 / 待办」组织，用于判断某项能力是否已落地、还差什么。

## 当前项目取舍

本项目不打算申请或使用讯飞 API Key，因此不实现实时 ASR 语音转写能力。

具体含义:

- 不实现 `WEBSOCKET /api/xunzhi/v1/xunfei/audio-to-text/{userId}`。
- 不接入讯飞实时 ASR 客户端。
- 不把实时语音转写、音频帧处理、ASR 增量去重作为后续待办。
- 如前端仍需要语音输入，应另行确定非讯飞方案后再单独设计。

## 目标能力总览

本项目对标的能力覆盖:

- 用户与权限: 登录态、共享 session、当前用户注入、管理员角色、WebSocket 鉴权。
- 通用 Agent: 会话创建、SSE 聊天、模型调用、上下文历史、用户归属校验、助手消息落库、会话 messageSeq 回写、文件上传。
- 普通 AI 对话: 多模型统一接入、SSE 流式输出、DeepSeek `reasoning_content`、用户消息和助手消息落 MongoDB。
- 面试主链路: 简历驱动出题、答题评分、追问规则、状态机、幂等、同题锁、运行态恢复、热/冷快照、最终归档、雷达图。
- 运行时保护: AI Guard、SingleFlight、分布式锁、Fencing Token、L1 缓存、超时、重试、并发上限。
- 媒体能力: 服务端 WebSocket 推送、长文本 TTS 异步任务和同步等待。
- 工程化: Docker Compose、CI、测试覆盖、skills 知识体系。

## 接口覆盖情况

### 已有 REST 接口

当前项目已有这些 REST 路径:

- `/api/xunzhi/v1/users/**`
- `/api/xunzhi/v1/agents/**`
- `/api/xunzhi/v1/agent-properties/**`
- `/api/xunzhi/v1/ai/**`
- `/api/xunzhi/v1/ai-properties/**`
- `/api/xunzhi/v1/interview/**`
- `/api/xunzhi/v1/websocket/**`
- `/api/xunzhi/v1/xunfei/tts/**`

注意: 路径存在不等于能力完成。Agent/AI/面试主链路已接真实实现，媒体(TTS/WebSocket)仍为 mock。

### 记忆阈值接口

AI 侧有运行时压缩阈值接口:

- `GET /api/xunzhi/v1/ai/memory/threshold`
- `PUT /api/xunzhi/v1/ai/memory/threshold`

用于运行时调整 AI 上下文压缩阈值。Agent 侧不使用压缩记忆，无此接口。

### 明确不实现的接口

实时 ASR WebSocket endpoint:

- `WEBSOCKET /api/xunzhi/v1/xunfei/audio-to-text/{userId}`

由于本项目不使用讯飞 API Key，这个实时 ASR WebSocket endpoint 不纳入实现计划。当前仅有 `/websocket/**` 的 HTTP 推送接口。

## 当前项目主要差距

### 1. 用户与权限

目标能力:

- 登录态中间件，支持多实例共享。
- 当前用户自动注入。
- 管理员角色权限服务。
- WebSocket 建连鉴权，校验 token 和 path `userId` 一致。
- 会话归属校验作为正式服务能力。

当前实现:

- 使用自定义 JWT middleware。
- token 无效或缺失时中间件默认放行，依赖 handler 手动检查。
- 登录密码明文比较。
- `GenerateToken(user.Username, string(rune(user.ID)))` 可能不是十进制 user ID。
- 管理员设置缺少操作者权限校验。
- 没有独立的会话归属校验服务。
- 没有 WebSocket token 鉴权体系。

待办:

- P0: 修正 JWT `user_id` 写法，引入强制认证分组或 `RequireAuth`。
- P0: 密码哈希和管理员接口权限校验。
- P1: 抽象 conversation ownership service。
- P2: WebSocket 鉴权。

### 2. Agent 会话

目标能力:

- `POST /agents/sessions/{sessionId}/chat` 是 SSE 流式接口。
- 聊天前校验 `sessionId` 和用户归属。
- 读取历史消息后调用模型。
- 用户消息和助手消息都落 MongoDB。
- 出错时也会保存默认错误 assistant 消息。
- 成功后发送 `end` 事件 `[DONE]`。
- 回写会话 `messageSeq` / message count。
- `AgentConversation` 和 `AgentMessage` 均为 MongoDB 主记录。

当前实现:

- Agent Chat 已实现 SSE 流式闭环，调用配置的模型，保存 user/assistant 两类消息并回写会话计数。
- `AgentMessage` 在 MongoDB `agent_messages`。
- `AgentConversation` 已迁 MongoDB `agent_conversations`。
- 已做上下文压缩记忆系统，Chat 后异步触发压缩判断（Agent 侧当前不压缩，仅 AI 侧压缩）。
- 没有独立的会话归属服务。
- `CreateConversationWithTitle` 忽略入参 `agentID`，固定写 1。
- 文件上传只是保存到 `./uploads/filename`，缺少类型校验、文件名净化、大小限制和消费链路。

待办:

- P0: 聊天前校验会话归属。
- P1: Agent 配置到真实 workflow/provider 调用。
- P1: 文件上传安全处理。

### 3. 普通 AI 对话

目标能力:

- `/ai/sessions/{sessionId}/chat` 是 `text/event-stream`。
- 统一多模型接入封装。
- 支持 DeepSeek 等 OpenAI-compatible 模型。
- 支持 `reasoning_content` 思维链内容。
- 用户消息、助手消息、错误消息都持久化。
- 历史消息来自 MongoDB。
- 会话归属校验和会话更新完整。

当前实现:

- AI Chat 已接入 OpenAI-compatible 模型调用，同时支持普通 JSON 和 SSE 流式输出。
- `AiMessage` 已迁 MongoDB `ai_messages`。
- `AiConversation` 已迁 MongoDB `ai_conversations`。
- 已有 AI 会话级长期记忆: MongoDB 原始消息、Redis 压缩摘要、MongoDB 压缩快照 `_id=ai:{sessionId}`。
- 用户消息和 assistant 回复都会持久化。
- 删除会话会清理 MongoDB `ai_messages` 和 AI 压缩上下文。
- SSE 支持 `message` 增量事件和 `reasoning` 增量事件，但 reasoning content 当前只流式透出，不单独持久化。
- 没有错误消息持久化。
- AI 配置 CRUD 基础可用，但没有 provider handler/factory。

待办:

- P1: 完善 reasoning content 字段持久化和前端展示协议。
- P1: 为模型调用增加 timeout/retry/concurrency guard 和错误 assistant 消息策略。
- P2: provider handler/factory。

### 4. 面试主链路

目标能力:

- `InterviewSession`、`InterviewQuestion` 等核心对象走 MongoDB。
- 简历提题调用模型，写题目、建议和简历分。
- 答题链路有完整 pipeline: 参数校验、`requestId` 幂等、当前题校验、同题锁、AI 评分、追问规则、主问题计分、失败回滚、快照刷新。
- 规则链决定是否追问。
- Redis 存热运行态，MongoDB 存热/冷快照和轮次归档。
- 恢复支持 `READ_ONLY` / `READ_WRITE_REQUIRED` 和不同恢复范围。
- `finishSession` 从运行态落正式记录。
- 雷达图、总分、建议优先读运行态，再回退记录。
- PDF 简历预览返回文本。

当前实现:

- 路由齐全，面试主链路已落地: 出题、答题评分、追问、状态机、运行态恢复、热冷快照、轮次归档、幂等补偿。
- `CreateSession` 写 Mongo `interview_sessions`，但 `PageConversations` 查 Mongo `agent_conversations`，`GetConversationHistory` 查 Mongo `agent_messages`（会话列表与历史消息分属不同集合，非同一真相源）。
- `SaveInterviewRecordFromRedis` 已实现: 从 Mongo `TurnArchive` 汇总轮次算平均分，写 `InterviewRecord`。
- `PreviewResume` 已实现: 从 Mongo 读 `ResumePath` → 解析 PDF 返回文本。
- `InterviewQuestion` 已有 Mongo 仓储并被 `question_cache` 使用。
- 神态分析未实现。

待办:

- P0: 修正面试会话列表/历史的真相源，不要混用 AgentConversation。
- P1: 神态分析（如需要）。
- P2: 面试最终报告记录是否补充更多维度。

### 5. 运行时保护

目标能力:

- AI Guard: 超时、重试、并发上限、熔断。
- SingleFlight: 分布式协调、owner/follower、结果回放、接管、L1 缓存。
- 分 stage 策略: scoring、followup、extraction、demeanor 不同 TTL 和并发。
- worker pool 隔离。
- 运行配置热刷新。

当前实现:

- 已有分布式 SingleFlight: 主从选举 + 流式心跳 + 换主 + 降级回调，AI 侧压缩路径已接入（`pkg/singleflight/`）。
- 已有通用分布式锁，面试侧题级锁已接入（`pkg/lock/`）。
- 没有 AI Guard（超时/重试/熔断）。
- 没有 fencing token。
- 异步 goroutine 没有统一 worker pool。
- 没有分 stage 配置。

待办:

- P1: 为 AI 调用加 timeout/retry/concurrency guard。
- P2: 面试高成本 AI 调用接 SingleFlight。
- P2: 为压缩、AI、面试任务引入 worker pool。

### 6. 媒体与实时通信

目标能力:

- 服务端主动推送消息。
- 长文本 TTS 真实异步任务、查询、同步等待。
- 在线用户连接表、心跳、事件协议。

当前实现:

- `/websocket/**` HTTP 接口只是 mock 返回。
- 缺少真正 WebSocket endpoint。
- TTS 三个接口只返回 mock task/status。
- 不计划实现讯飞实时 ASR，因此不需要讯飞 ASR 客户端、二进制音频帧处理和 ASR 增量去重。
- 没有真实 TTS 客户端。
- 没有在线用户连接表、心跳、事件协议。

待办:

- P1: WebSocket 在线用户表和推送服务。
- P1: TTS 真实任务客户端。
- 不实施: 讯飞实时 ASR WebSocket 和 ASR 增量稳定化策略。

### 7. 数据模型和持久化

目标策略:

- 用户/权限/AgentProperties/AiProperties 等配置类走 MySQL。
- Agent/Ai conversation/message 走 MongoDB。
- 面试 session/question/runtime snapshot/turn archive 走 MongoDB。
- Redis 负责运行态、锁、缓存、SingleFlight。

当前实现:

- `AgentConversation` 已迁 MongoDB `agent_conversations`。
- `AgentMessage` 已迁 MongoDB `agent_messages`。
- `AiConversation` 已迁 MongoDB `ai_conversations`。
- `AiMessage` 已迁 MongoDB `ai_messages`。
- `InterviewSession`/`InterviewRecord`/`InterviewQuestion` 均在 MongoDB。
- `CompressedContext` 用 MongoDB 覆盖写入: AI `_id=ai:{sessionId}`（Agent 侧不写压缩快照）。
- 用户/AiProperties/AgentProperties/文件资产仍在 MySQL。

待办:

- P1: 明确数据真相源策略，保持各模块一致性。
- P2: 评估是否需要进一步整合。

### 8. 工程化

目标能力:

- Docker Compose: MySQL + MongoDB + Redis + App。
- `.env.example`。
- CI。
- 单元测试、压力测试、恢复一致性测试。
- Skills 文档和自动生成索引脚本。

当前实现:

- 已有 Docker Compose 和 Dockerfile。
- 已建立本地 skills 知识体系。
- `scripts/knowledge-check.sh` 已有 full/diff 两种模式。
- 没有 CI。
- 基本无测试。

待办:

- P1: 补核心 service 单测。
- P2: CI 和接口契约测试。
- P2: 生成式 API 索引脚本。

## 功能缺口总表

| 模块 | 路由覆盖 | 真实能力缺口 | 优先级 |
| --- | --- | --- | --- |
| Auth/User | 基本覆盖 | 强认证、密码哈希、角色权限、WebSocket 鉴权、归属服务 | P0 |
| Agent | REST 覆盖 | 会话归属、Agent 配置真实调用、文件安全 | P0/P1 |
| AI | REST 覆盖, SSE chat | provider handler、reasoning_content 持久化、错误消息策略、运行时保护 | P0/P1 |
| Interview | 路由覆盖 | 会话列表/历史真相源统一、神态分析 | P0/P2 |
| Runtime | 无独立接口 | AI Guard、worker pool、fencing token、分 stage 配置 | P1/P2 |
| Media | REST 覆盖 | 讯飞 ASR 明确不实现; TTS/推送为 mock | P1 |
| Persistence | 基本完成 | 真相源策略文档化 | P1/P2 |
| DevOps/Test | 少量脚本 | CI、单测、压力/恢复测试 | P1/P2 |

## 建议实施顺序

1. Auth 基线: 修 JWT user_id、强认证策略、密码哈希、管理员权限。
2. Agent 归属: 会话归属校验、文件安全。
3. AI 增强: reasoning content 持久化、错误消息策略和运行时保护。
4. 面试真相源: 会话列表/历史真相源统一。
5. 运行时保护: AI Guard 先行，worker pool 后接。
6. 媒体: WebSocket 推送和真实 TTS。讯飞实时 ASR 不实现。
7. 工程化: CI、核心测试和生成式接口索引。
