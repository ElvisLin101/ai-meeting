# AI-Meeting Agent Guide

本仓库采用 Skills 风格的项目知识模式。目标是让 Code Agent 先读项目知识, 再读代码, 并在修改代码时同步校验知识是否过期。

## 工作流

1. 先按下面的路由表判断需求属于哪个模块。
2. 读取对应 `skills/*/SKILL.md`。
3. 只有当 Skill 主文件要求时, 再读取 `docs/agent-knowledge/references/*`。
4. 修改代码前, 用实际代码反向校验 Skill 中的代码锚点和业务规则。
5. 修改代码后运行 `scripts/knowledge-check.sh diff`。如果行为、路由、模型、存储或流程发生变化, 同步更新相关 Skill 或引用文档。

## 项目概览

AI-Meeting 是一个 Go 1.22 + Gin 后端。入口在 `main.go`, 路由集中在 `api/routes/routes.go`, 业务主要分为用户认证、Agent 会话、AI 会话、面试流程、上下文压缩和媒体接口。

核心分层:

- `api/handlers`: HTTP 入参、认证上下文读取、DTO 响应组装。
- `services`: 业务流程和跨仓储编排。
- `models`: GORM 模型和 Mongo 模型。
- `dto`: HTTP 请求/响应结构。
- `repositories/mysql`: MySQL 初始化与用户/AI配置/Agent配置/文件资产读写(会话/消息/面试数据已迁 Mongo)。
- `repositories/mongo`: MongoDB 初始化与集合级读写。
- `repositories`: 其他基础设施客户端, 当前包含 Redis。
- `clients`: 外部 HTTP/API 客户端, 例如 OpenAI-compatible 模型调用。
- `config`: Viper 配置加载。

## Skill 路由

| 需求关键词 | 先读 Skill | 典型代码锚点 |
| --- | --- | --- |
| Agent 聊天、Agent 会话、Agent 配置、文件上传 | `skills/ai-meeting-agent/SKILL.md` | `api/handlers/agent_handler.go`, `services/agent/agent_service.go`, `models/agent.go` |
| 长上下文、压缩摘要、Redis/Mongo 恢复、历史窗口 | `skills/ai-meeting-memory/SKILL.md` | `services/ai/ai_memory_service.go`, `models/compressed_context.go`, `repositories/redis.go`, `repositories/mongo/*.go` |
| 面试会话、题目、评分、建议、简历预览、面试记录 | `skills/ai-meeting-interview/SKILL.md` | `api/handlers/interview_handler.go`, `services/interview/interview_service.go`, `models/interview.go` |
| 登录、注册、JWT、用户资料、管理员 | `skills/ai-meeting-user-auth/SKILL.md` | `api/middleware/auth.go`, `api/handlers/user_handler.go`, `services/user/user_service.go`, `models/user.go` |
| AI 会话、AI 消息、AI 模型配置 | `skills/ai-meeting-ai/SKILL.md` | `api/handlers/ai_handler.go`, `services/ai/ai_service.go`, `models/ai.go` |
| 路由、模型、跨模块影响 | `docs/agent-knowledge/references/routes-map.md`, `docs/agent-knowledge/references/data-models.md` | `api/routes/routes.go`, `models/*.go` |
| 对照 Java 参考项目、判断功能缺口 | `docs/agent-knowledge/references/reference-project-gap-summary.md` | `/Users/mac/develop/projects/AI-Meeting` |

## 全局术语

- `session_id`: 会话业务 ID, 由 UUID 生成, 多数会话和消息接口按它查询。
- `username`: JWT 中的用户名, Agent/AI 用户隔离多数使用它作为 `user_id` 字段值。
- `user_id`: JWT 中的用户 ID 字符串, 面试模块主要读取该上下文键。
- `conversation`: 会话列表记录, 记录标题、状态、消息数。
- `message`: 用户或助手单条消息, 通过 `sequence` 维护会话内顺序。
- `compressed context`: Agent 或 AI 历史消息压缩摘要, Redis 做热缓存, MongoDB 做持久恢复。

## 硬性约束

- 不要只根据 Skill 修改代码。Skill 是路由和背景, 实际代码是最终依据。
- 发现 Skill 与代码不一致时, 在变更说明中标注并更新知识文档。
- 新增或调整 HTTP 接口时, 同步更新 `docs/agent-knowledge/references/routes-map.md`。
- 新增、删除或改名模型字段时, 同步更新 `docs/agent-knowledge/references/data-models.md`。
- 不要把示例返回值当成真实业务实现。当前面试流程、AI 调用、Agent 调用和上下文压缩里存在多个占位实现, 详见 `docs/agent-knowledge/references/placeholder-risk-register.md`。
- Go 代码约束读 `docs/agent-knowledge/rules/go-backend.md`。

## 知识防腐机制

- 反向校验: 每次读 Skill 后都对照代码。若代码锚点不存在或行为不符, 先相信代码, 再修 Skill。
- 知识补充: 如果实现过程中用户补充了新的业务规则, 在提交前沉淀到对应 Skill。
- Diff 检查: 提交前运行 `scripts/knowledge-check.sh diff`, 让脚本提示可能要更新的 Skill。
- 全量巡检: 周期性运行 `scripts/knowledge-check.sh full`, 检查文档引用的 Go 文件是否仍存在。

## 每次思考和回答都要用中文, 并叫我一声小林
