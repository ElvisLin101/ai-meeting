# AI 记忆演进：尚未实现的向量召回

代码核对日期：2026-11-23。本文为后续方案评估，仓库当前没有 embedding client、向量索引或召回路径。

## 当前能力

普通 AI 使用摘要与近期 MySQL 原文窗口，按字节阈值压缩；Agent Run 使用 token 预算、摘要、近期 Redis/MySQL 消息、跨会话结构化画像和身份。两条链路均将原文保存在统一 `messages`，摘要有损。

摘要适合保持话题连续性，结构化画像适合稳定事实；早期原文中的具体版本、数字、约束仍可能无法精准找回。向量召回可以补充相关片段，但不能替代权限校验和原文证据。

## 可评估的方案

| 方案 | 适用条件 | 代价与边界 |
| --- | --- | --- |
| 小规模应用内相似度检索 | 样本较少，用于验证召回价值 | 向量可存 MySQL，经用户/会话过滤后在应用计算相似度；不具备原生向量索引性能 |
| 专用向量库（如 Qdrant / Milvus） | 数据规模与延迟要求需要索引 | 增加部署、索引、删除同步与备份成本 |
| 支持向量的数据库或托管服务 | 已有明确版本与能力支持 | 先验证实际版本、索引方式、过滤语义及费用，再决定接入 |

项目 Docker Compose 当前是 App + MySQL 8 + Redis 7。不能声称 MySQL 5.0+ 支持 `$vectorSearch`，该语法不是本项目 MySQL 的能力。现有 DeepSeek chat client 也不能当成 embedding API；需要独立选择并验证 embedding provider、维度、模型版本和接口。

## 建议接入边界

1. 以 messages 的 message_id / sequence 为原文引用，异步生成 embedding，记录 user_id、session_id、模型版本与索引状态。
2. 查询时先强制用户隔离，默认限定会话；跨会话检索作为明确产品能力单独定义。
3. 将原文片段与摘要、近期窗口拼接，按 message_id 去重，分配独立 token 预算并标注出处。
4. 召回失败降级为现有摘要/近期窗口；消息删除同步清理索引，失败需可重试。
5. 验证召回准确率、误召回、延迟、成本，再决定扩大索引范围。

## 实现时先读

- `services/ai/ai_memory_service.go`、`services/ai/ai_chat_service.go`：普通 AI 装配和消息提交。
- `services/agent/context/assembler.go`、`services/agent/context/store.go`：Agent 装配与存储边界。
- `repositories/mysql/message_repository.go`、`repositories/mysql/compressed_context_repository.go`：原文与摘要持久化。
- `models/agent_runtime.go`：快照字段；AI TotalLength 为字节，Agent 为估算 token。
- `memory-context-flow.md`：当前流程；`placeholder-risk-register.md`：当前故障边界。

## 验证目标

长对话压缩后仍能检索早期具体事实；召回不重复近期窗口、不越过用户/会话权限；索引缺失不阻断聊天，删除与重建可验证。上述目标尚未实现，不能写入已实现能力列表。
