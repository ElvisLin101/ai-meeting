# AI 记忆演进：向量召回

## 现状与缺口

当前 AI 侧记忆是**单层有损模型**: 全部历史消息按 80/20 分割, 旧的 80% 调 DeepSeek 生成 ≤500 字摘要, 新的 20% 保留原文窗口, 拼成最终上下文。详见 `memory-context-flow.md`。

压缩摘要管「整体上下文连续性」, 但管不了「事实级精准召回」:

- 摘要是有损的, 早期消息里的具体细节（技术栈版本、项目约束、团队规模、预算数字）被压进摘要后语义融化, 无法精确检索回原文。
- 用户后续追问「我之前说我用的什么版本」时, 压缩记忆答不准。
- 记忆按 `_id=ai:{sessionId}` 隔离, 跨会话完全不可见。

## 向量召回补什么

和压缩记忆**并行加一层**, 两者互补不互替:

```
全部历史消息 ─┬─ [压缩]   → 摘要 + 近期窗口    (现有, 管上下文连续性)
              └─ [向量化] → 向量库              (新增, 管事实精准召回)
```

- 压缩记忆回答: 「我们刚才在聊什么」→ 塞 system prompt。
- 向量召回回答: 「用户之前提过的某个具体事实」→ 检索 top-k 相关片段塞上下文。

最终 context 拼装: `摘要 + 向量召回片段 + 近期窗口`。

## 演进方案（从轻到重）

### 方案 A: 复用 MongoDB 向量搜索（推荐起步）

MongoDB 5.0+ 支持 `$vectorSearch`。不引入新基础设施, 改动最小。

- 新增集合 `ai_memory_vectors`, 文档结构: `{session_id, message_id, sequence, role, content, embedding, created_at}`。
- 写入时机: 与压缩并行, 消息落 MongoDB `ai_messages` 后异步生成 embedding 写入。
- 读取时机: `AiMemoryService.GetContext` 中, 除读摘要 + 近期窗口外, 拿当前用户消息做向量检索, top-k 片段拼入上下文。
- embedding 模型: 走 DeepSeek embedding API 或单独接 embedding 服务。

缺点: Mongo 向量检索性能和召回质量不如专用向量库, 数据量大时有差距。

### 方案 B: 引入专用向量库

Milvus / Weaviate / Qdrant 选一。检索质量好, 但多一个基础设施要部署运维。项目已有 Docker Compose（MySQL + Mongo + Redis）, 再加一个向量库不算太重, 但运维成本上升。

### 方案 C: 记忆分层（完整架构）

```
L1 近期窗口   (Redis)     — 最近 N 条原文, 零延迟
L2 压缩摘要   (Redis+Mongo) — 整体上下文, 有损
L3 向量召回   (向量库)     — 事实级精准检索
L4 全量历史   (Mongo)     — 审计兜底, 不直接喂模型
```

当前项目已有 L1/L2/L4, 加 L3 补齐。这是长程记忆的成熟分层模式。

## 涉及的代码锚点

实现时需要改动的位置:

- `services/ai/ai_memory_service.go`: `GetContext` 增加向量检索拼装, `CompressContext` 附近增加向量写入触发。
- `services/ai/ai_chat_service.go`: `finishAiChat` 中消息落库后增加异步 embedding 生成。
- `repositories/mongo/`: 新增 vector repository, 或新增集合的读写。
- `clients/`: 新增 embedding client（若不复用 DeepSeek）。
- `models/`: 新增 vector 文档模型。
- `config/config.yaml`: 新增 embedding 模型配置。

## 注意事项

1. **embedding 成本**: 每条消息都向量化会多一次 API 调用。可只对 user 消息或「事实密度高」的消息做, 不必全量。
2. **召回片段去重**: 向量召回的片段可能和近期窗口重叠（同一条消息既在窗口里又被召回）, 拼装时按 `message_id` 或 `sequence` 去重, 避免浪费 token。
3. **压缩与向量协同**: 被压缩掉的消息, 其向量应保留——摘要丢了细节, 但向量还能找回, 这正是加向量召回的意义。
4. **跨会话隔离**: 向量检索默认按 `session_id` 过滤, 避免 A 会话召回 B 会话内容。若要做「用户级长期记忆」（跨会话）, 需单独设计权限和隐私边界。
5. **阈值单位**: 现有压缩阈值用字节长度（`len()`）而非 token 数, MongoDB 字段名 `total_token_count` 实际存字节数。接入向量召回后若需要更精确的 token 预算控制, 应一并修正此处。

## 修改时必查

- `docs/agent-knowledge/references/memory-context-flow.md`: 现有压缩记忆完整流程。
- `services/ai/ai_memory_service.go`: 现有 `GetContext` / `CompressContext` / `buildContextWithWindow` 逻辑。
- `repositories/mongo/compressed_context_repository.go`: 现有持久化模式参考。
- `models/compressed_context.go`: 现有模型字段参考。

## 验证建议

- 构造长对话使消息被压缩, 再追问早期具体事实, 检查向量召回能否命中原文片段。
- 检查召回片段与近期窗口不重复。
- 清空向量库后检查降级行为（应回退到纯压缩摘要, 不阻断对话）。
