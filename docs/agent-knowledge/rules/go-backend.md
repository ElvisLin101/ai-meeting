# Go Backend Rules

## 基本规范

- 使用 Go 1.22, 保持现有 `handlers -> services -> repositories/models` 分层。
- Handler 只处理 HTTP 入参、认证上下文、状态码和 DTO 转换。
- Service 放业务流程和跨仓储编排, 不直接依赖 Gin, 不直接拼 Mongo/MySQL 查询。
- MySQL 读写放 `repositories/mysql`, MongoDB 读写放 `repositories/mongo`。
- Redis 当前仍在根 `repositories`, 通过 `repositories.RedisClient` 使用。
- 外部 HTTP/API 调用放 `clients`, 不放在 service 或 repository。
- 错误返回给调用方, 不在普通业务流程里 `panic`。
- 新增复杂函数时优先传 `context.Context`; 现有代码大量未传 context, 新增可逐步改善, 不做无关大重构。

## 数据和安全

- 用户隔离查询必须带 `username` 或 `user_id`, 不能只按 `session_id` 查询敏感数据。
- 认证相关改动必须同时检查 `api/middleware/auth.go` 和具体 handler 的上下文键读取方式。
- 当前密码是明文比较, AI/API 密钥也直接存储在表字段中。修复安全问题时要同步更新 user-auth 和 ai Skill。
- 上传文件路径当前是 `./uploads/` 拼接原始文件名。改上传逻辑时要检查路径穿越、重名覆盖和目录创建。

## 变更检查

- 改路由: 更新 `docs/agent-knowledge/references/routes-map.md`。
- 改模型: 更新 `docs/agent-knowledge/references/data-models.md`。
- 改长上下文、压缩、消息顺序: 更新 memory Skill 和 `memory-context-flow.md`。
- 把占位逻辑替换成真实实现: 更新 `placeholder-risk-register.md`。
- 提交前运行 `scripts/knowledge-check.sh diff`。
