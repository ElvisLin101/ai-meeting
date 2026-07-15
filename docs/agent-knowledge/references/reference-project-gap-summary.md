# Reference Project Gap Summary

参考项目: `/Users/mac/develop/projects/AI-Meeting`

当前 Go 项目: `/Users/mac/develop/GoZero/AI-Meeting`

最后整理: 2026-07-11

## 目的

以后需要对照 Java 参考项目时, 先读这个文档。只有当这里的信息不够时, 再去参考项目打开具体代码或 Skill。

## 当前项目取舍

当前 Go 项目不打算申请或使用讯飞 API Key, 因此不实现参考项目中的实时 ASR 语音转写能力。

具体含义:

- 不实现 `WEBSOCKET /api/xunzhi/v1/xunfei/audio-to-text/{userId}`。
- 不接入讯飞实时 ASR 客户端。
- 不把实时语音转写、音频帧处理、ASR 增量去重作为后续待办。
- 如前端仍需要语音输入, 应另行确定非讯飞方案后再单独设计, 不沿用参考项目的讯飞 ASR 路线。

## 参考项目总览

参考项目是 Spring Boot 3 + Java 17 后端, 技术栈包含 MySQL、MongoDB、Redis/Redisson、Sa-Token、Spring AI、LiteFlow、Resilience4j、SSE、WebSocket、讯飞接口和 CI。

参考项目的主要能力:

- 用户与权限: Sa-Token 登录态, Redis 共享 session, 当前用户注入, 管理员角色, WebSocket 鉴权。
- 通用 Agent: 会话创建、SSE 聊天、星辰工作流调用、上下文历史、用户归属校验、助手消息落库、会话 messageSeq 回写、文件上传。
- 普通 AI 对话: Spring AI 多模型统一接入, SSE/Flux 流式输出, DeepSeek `reasoning_content`, 用户消息和助手消息落 MongoDB。
- 面试主链路: 简历驱动出题、答题评分、追问规则、状态机、幂等、同题锁、运行态恢复、热/冷快照、最终归档、雷达图、神态分析。
- 运行时保护: AI Guard、SingleFlight、Redis Lua、Fencing Token、L1 缓存、超时、重试、并发上限、线程池隔离。
- 媒体能力: 讯飞实时 ASR WebSocket、服务端 WebSocket 推送、长文本 TTS 异步任务和同步等待。
- 工程化: Docker Compose、GitHub Actions、测试覆盖、skills 知识体系。

## 参考项目 Skill 路由

| 参考 Skill | 负责内容 |
| --- | --- |
| `xunzhi-repo-map` | 总路由、接口索引、模块边界 |
| `xunzhi-agent-domain` | Agent 会话、SSE 聊天、历史、归属、文件和 Agent 配置 |
| `xunzhi-interview-domain` | 面试会话、提题、答题、追问、恢复、收尾、工作流 |
| `xunzhi-ai-runtime` | AI Guard、SingleFlight、线程池、限流、配置 |
| `xunzhi-auth-user` | Sa-Token、当前用户、权限、WebSocket 鉴权、数据隔离 |
| `xunzhi-media-domain` | 实时 ASR、WebSocket 推送、长文本 TTS |

## 接口覆盖情况

### 已有同名 REST 接口

当前 Go 项目已经有这些参考项目 REST 路径:

- `/api/xunzhi/v1/users/**`
- `/api/xunzhi/v1/agents/**`
- `/api/xunzhi/v1/agent-properties/**`
- `/api/xunzhi/v1/ai/**`
- `/api/xunzhi/v1/ai-properties/**`
- `/api/xunzhi/v1/interview/**`
- `/api/xunzhi/v1/websocket/**`
- `/api/xunzhi/v1/xunfei/tts/**`

但注意: 路径存在不等于能力完成。Go 项目里很多 handler 仍是 mock 或只做最小落库。

### Go 项目额外接口

Go 项目新增了参考项目没有的记忆阈值接口:

- `GET /api/xunzhi/v1/agents/memory/threshold`
- `PUT /api/xunzhi/v1/agents/memory/threshold`
- `GET /api/xunzhi/v1/ai/memory/threshold`
- `PUT /api/xunzhi/v1/ai/memory/threshold`

它们用于运行时调整 Agent 和 AI 上下文压缩阈值。

### 明确不实现的参考接口

参考项目有真实 WebSocket 端点:

- `WEBSOCKET /api/xunzhi/v1/xunfei/audio-to-text/{userId}`

当前 Go 项目只有 `/websocket/**` 的 HTTP 推送接口。由于本项目不使用讯飞 API Key, 这个实时 ASR WebSocket endpoint 不纳入实现计划。

## 当前 Go 项目主要差距

### 1. 用户与权限

参考项目能力:

- Sa-Token 登录态。
- Redis 持久化 token/session, 支持多实例共享登录态。
- `@CurrentUser` 自动注入当前用户。
- 管理员角色权限服务。
- WebSocket 建连鉴权, 校验 token 和 path `userId` 一致。
- 会话归属校验是正式服务能力。

当前 Go 项目状态:

- 使用自定义 JWT middleware。
- token 无效或缺失时中间件默认放行, 依赖 handler 手动检查。
- 登录密码明文比较。
- `GenerateToken(user.Username, string(rune(user.ID)))` 可能不是十进制 user ID。
- 管理员设置缺少操作者权限校验。
- 没有独立的会话归属校验服务。
- 没有 WebSocket token 鉴权体系。

建议优先级:

- P0: 修正 JWT `user_id` 写法, 引入强制认证分组或 `RequireAuth`。
- P0: 密码哈希和管理员接口权限校验。
- P1: 抽象 conversation ownership service。
- P2: WebSocket 鉴权。

### 2. Agent 会话

参考项目能力:

- `POST /agents/sessions/{sessionId}/chat` 是 SSE 流式接口。
- 聊天前校验 `sessionId` 和用户归属。
- 读取历史消息后调用星辰工作流 `XingChenAIClient.chat`。
- 用户消息和助手消息都落 MongoDB。
- 出错时也会保存默认错误 assistant 消息。
- 成功后发送 `end` 事件 `[DONE]`。
- 回写会话 `messageSeq` / message count。
- `AgentConversation` 和 `AgentMessage` 在参考项目中都是 MongoDB 主记录。

当前 Go 项目状态:

- Agent Chat 只保存用户消息, 返回普通 JSON。
- 已把 `AgentMessage` 迁到 MongoDB `agent_messages`。
- 已做上下文压缩记忆系统, Chat 后异步触发压缩判断。
- 没有真实模型调用。
- 没有 SSE 流式输出。
- 没有 assistant 消息落库。
- 没有会话归属服务。
- `AgentConversation` 仍在 MySQL。
- `CreateConversationWithTitle` 忽略入参 `agentID`, 固定写 1。
- 文件上传只是保存到 `./uploads/filename`, 缺少类型校验、文件名净化、大小限制和消费链路。

建议优先级:

- P0: 实现 Agent SSE chat, 保存 user/assistant 两类消息, 回写会话计数。
- P0: 聊天前校验会话归属。
- P1: Agent 配置到真实 workflow/provider 调用。
- P1: 文件上传安全处理。
- P2: 是否将 `AgentConversation` 也迁到 MongoDB。

### 3. 普通 AI 对话

参考项目能力:

- `/ai/sessions/{sessionId}/chat` 是 `text/event-stream`。
- 基于 Spring AI 封装 `UniversalAiChatHandler`。
- 支持 DeepSeek 等 OpenAI-compatible 模型。
- 支持 `reasoning_content` 思维链内容。
- 用户消息、助手消息、错误消息都持久化。
- 历史消息来自 MongoDB。
- 会话归属校验和会话更新完整。

当前 Go 项目状态:

- AI Chat 已接入 OpenAI-compatible 模型调用, 同时支持普通 JSON 和 SSE 流式输出。
- `AiMessage` 已迁到 MongoDB `ai_messages`。
- 已有 AI 会话级长期记忆: MongoDB 原始消息、Redis 压缩摘要、MongoDB 压缩快照 `_id=ai:{sessionId}`。
- 用户消息和 assistant 回复都会持久化。
- 删除会话会清理 MongoDB `ai_messages` 和 AI 压缩上下文。
- SSE 支持 `message` 增量事件和 `reasoning` 增量事件, 但 reasoning content 当前只流式透出, 不单独持久化。
- 没有错误消息持久化。
- AI 配置 CRUD 基础可用, 但没有 provider handler/factory。

建议优先级:

- P1: 完善 reasoning content 字段持久化和前端展示协议。
- P1: 为模型调用增加 timeout/retry/concurrency guard 和错误 assistant 消息策略。
- P2: provider handler/factory。

### 4. 面试主链路

参考项目能力:

- `InterviewSession`、`InterviewQuestion` 等核心对象走 MongoDB。
- 简历提题调用工作流, 写题目、建议和简历分。
- 答题链路有 `InterviewAnswerPipeline`:
  - 参数校验。
  - `requestId` 幂等。
  - 当前题校验。
  - 同题锁。
  - AI 评分。
  - 追问规则。
  - 主问题计分。
  - 失败回滚。
  - 快照刷新。
- LiteFlow 规则链决定是否追问。
- Redis 存热运行态, MongoDB 存热/冷快照和轮次归档。
- 恢复支持 `READ_ONLY` / `READ_WRITE_REQUIRED` 和不同恢复范围。
- `finishSession` 从运行态落正式记录。
- 雷达图、总分、建议优先读运行态, 再回退记录。
- 神态分析接 AI/图片结果。
- PDF 简历预览返回 `application/pdf`。

当前 Go 项目状态:

- 路由基本齐全。
- 大多数面试 service 返回固定示例值。
- `CreateSession` 写 `interview_sessions`, 但 `PageConversations` 查 `AgentConversation`。
- 面试历史查 MySQL `AgentMessage`, 与当前 Agent Mongo 消息不一致。
- 没有题目表、题目状态、追问状态机、运行态、幂等、锁、快照、归档。
- `SaveInterviewRecordFromRedis` 是空实现。
- `PreviewResume` 只返回固定 JSON, 不是 PDF。

建议优先级:

- P0: 修正面试会话列表/历史的真相源, 不要混用 AgentConversation。
- P0: 设计 InterviewQuestion/FlowState/Record 的模型和状态机。
- P1: 提题、答题评分、追问、总分和建议真实化。
- P1: Redis + Mongo hot/cold snapshot 恢复。
- P1: `finishSession` 和 `SaveInterviewRecordFromRedis` 真正归档。
- P2: 神态分析和 PDF 简历预览。

### 5. 运行时保护

参考项目能力:

- AI Guard: 超时、重试、并发上限、熔断。
- SingleFlight: Redis Lua、Fencing Token、owner/follower、结果回放、接管、L1 缓存。
- 分 stage 策略: scoring、followup、extraction、demeanor 不同 TTL 和并发。
- 线程池隔离。
- 运行配置热刷新/索引文档。

当前 Go 项目状态:

- 没有 AI Guard。
- 没有 SingleFlight。
- 没有 distributed lock / fencing token。
- 异步 goroutine 没有统一线程池/worker pool。
- 没有分 stage 配置。

建议优先级:

- P1: 先为 AI 调用加 timeout/retry/concurrency guard。
- P2: 面试高成本 AI 调用接 SingleFlight。
- P2: 为压缩、AI、面试任务引入 worker pool。

### 6. 媒体与实时通信

参考项目能力:

- WebSocket `/xunfei/audio-to-text/{userId}`。
- 建连鉴权。
- 支持控制命令: `ping`, `start_transcription`, `stop_transcription`, `get_status`。
- 状态事件: `connected`, `heartbeat`, `transcription_started`, `transcription`, `final`, `error` 等。
- 讯飞实时 ASR 集成, 有分段增量去重和 final 事件。
- 服务端主动推送消息。
- 长文本 TTS 真实异步任务、查询、同步等待。

当前 Go 项目状态:

- `/websocket/**` HTTP 接口只是 mock 返回。
- 缺少真正 WebSocket endpoint。
- TTS 三个接口只返回 mock task/status。
- 不计划实现讯飞实时 ASR, 因此不需要讯飞 ASR 客户端、二进制音频帧处理和 ASR 增量去重。
- 没有真实 TTS 客户端。
- 没有在线用户连接表、心跳、事件协议。

建议优先级:

- P1: WebSocket 在线用户表和推送服务。
- P1: TTS 真实任务客户端。
- 不实施: 讯飞实时 ASR WebSocket 和 ASR 增量稳定化策略。

### 7. 数据模型和持久化

参考项目倾向:

- 用户/权限/AgentProperties/AiProperties 等配置类走 MySQL。
- Agent/Ai conversation/message 走 MongoDB。
- 面试 session/question/runtime snapshot/turn archive 走 MongoDB。
- 面试最终报告记录走 MySQL。
- Redis 负责运行态、锁、缓存、SingleFlight。

当前 Go 项目状态:

- AgentMessage 已迁 MongoDB。
- AgentConversation 仍 MySQL。
- AiConversation 仍 MySQL。
- AiMessage 已迁 MongoDB。
- InterviewSession/InterviewRecord 仍 MySQL。
- CompressedContext 已用 MongoDB 覆盖写入: Agent `_id=sessionId`, AI `_id=ai:{sessionId}`。

建议优先级:

- P1: 明确 Go 版数据真相源策略。
- P1/P2: 面试运行态引入 Redis + Mongo snapshot。
- P2: 是否迁 AgentConversation/AiConversation 到 MongoDB, 取决于前端和查询需求。

### 8. 工程化

参考项目能力:

- Docker Compose: MySQL + MongoDB + Redis + App。
- `.env.example`。
- GitHub Actions 后端 CI。
- 单元测试、压力测试、恢复一致性测试。
- Skills 文档和自动生成索引脚本。

当前 Go 项目状态:

- 没有 Docker Compose。
- 没有 CI。
- 基本无测试。
- 已建立本地 skills 知识体系。
- `scripts/knowledge-check.sh` 已有, 但当前目录不是 git 仓库时 diff 检查会跳过。

建议优先级:

- P1: 加 docker-compose 本地一键启动 MySQL/Mongo/Redis。
- P1: 补核心 service 单测。
- P2: CI 和接口契约测试。
- P2: 生成式 API 索引脚本。

## 功能缺口总表

| 模块 | 路由覆盖 | 真实能力缺口 | 优先级 |
| --- | --- | --- | --- |
| Auth/User | 基本覆盖 | 强认证、密码哈希、角色权限、WebSocket 鉴权、归属服务 | P0 |
| Agent | REST 覆盖, 多 memory threshold | SSE chat、模型调用、assistant 落库、会话归属、会话计数、文件安全 | P0 |
| AI | REST 覆盖, SSE chat, 多 memory threshold | provider handler、reasoning_content 持久化、错误消息策略、运行时保护 | P0/P1 |
| Interview | 路由覆盖 | 几乎全业务仍占位: 提题、评分、追问、状态机、幂等、恢复、归档 | P0/P1 |
| Runtime | 无独立接口 | AI Guard、SingleFlight、worker pool、限流、重试、熔断 | P1/P2 |
| Media | REST 覆盖 | 讯飞 ASR 明确不实现; TTS/推送为 mock | P1 |
| Persistence | 部分完成 | AI/Interview 仍未按参考项目使用 Mongo/Redis 运行态 | P1 |
| DevOps/Test | 少量脚本 | Docker Compose、CI、单测、压力/恢复测试 | P1/P2 |

## 建议实施顺序

1. Auth 基线: 修 JWT user_id、强认证策略、密码哈希、管理员权限。
2. Agent 闭环: SSE 模型调用、assistant 消息、归属校验、会话计数。
3. AI 增强: reasoning content 持久化、错误消息策略和运行时保护。
4. 面试数据模型: InterviewSession/Question/FlowState/Record 的真相源和状态机。
5. 面试答题流水线: 幂等、锁、评分、追问、推进、归档。
6. 运行时保护: AI Guard 先行, SingleFlight 后接。
7. 媒体: WebSocket 推送和真实 TTS。讯飞实时 ASR 不实现。
8. 工程化: docker-compose、CI、核心测试和生成式接口索引。

## 下次如果还需要看参考项目

优先打开:

1. `skills/xunzhi-repo-map/references/generated-api-index.md`
2. `skills/xunzhi-agent-domain/references/session-flow.md`
3. `skills/xunzhi-interview-domain/references/answer-pipeline.md`
4. `skills/xunzhi-interview-domain/references/restore-and-finalize.md`
5. `skills/xunzhi-media-domain/references/realtime-asr.md` 仅用于理解参考项目, 当前 Go 项目不按该方案实现 ASR。
6. `skills/xunzhi-ai-runtime/references/ai-singleflight.md`
7. `skills/xunzhi-auth-user/references/data-isolation.md`

避免一上来全仓库搜索, 除非这些文档和当前代码明显不一致。
