# Go Backend Rules

## 基本规范

- 使用 Go 1.24.1, 保持现有 `handlers -> services -> repositories/models` 分层。
- Handler 只处理 HTTP 入参、认证上下文、状态码和 DTO 转换。
- Service 放业务流程和跨仓储编排, 不直接依赖 Gin, 不直接拼 Mongo/MySQL 查询。
- MySQL 读写放 `repositories/mysql`, MongoDB 读写放 `repositories/mongo`。
- Redis 当前仍在根 `repositories`, 通过 `repositories.RedisClient` 使用。
- 外部 HTTP/API 调用放 `clients`, 不放在 service 或 repository。
- 错误返回给调用方, 不在普通业务流程里 `panic`。
- 统一错误码: 业务错误用 `pkg/ecode`（`ecode.New(code, msg)` / `ecode.Wrap`）, handler 出口统一走 `api/resp` 的 `Respond`/`Fail`, 错误响应 `{"code": N, "error": "msg"}`, 禁止散落 `c.JSON(status, gin.H{"error": ...})`。
- 新增复杂函数时优先传 `context.Context`; 现有代码大量未传 context, 新增可逐步改善, 不做无关大重构。

## 数据和安全

- 用户隔离查询必须带 `username` 或 `user_id`, 不能只按 `session_id` 查询敏感数据。
- 认证相关改动必须同时检查 `api/middleware/auth.go` 和具体 handler 的上下文键读取方式。
- 密码已 bcrypt 哈希存储与比较（`services/user/user_service.go: verifyPassword`, 旧明文登录成功自动迁移）; AI/API 密钥仍直接存储在表字段中, 修复时同步更新 user-auth 和 ai Skill。
- 上传文件已做大小限制(10MB, `http.MaxBytesReader`)与 UUID 重命名防路径穿越（`api/handlers/agent_handler.go`/`api/handlers/interview_handler.go`）; 新增上传接口必须同样处理, 并校验类型(magic bytes)。

## 变更检查

- 改路由: 更新 `docs/agent-knowledge/references/routes-map.md`。
- 改模型: 更新 `docs/agent-knowledge/references/data-models.md`。
- 改长上下文、压缩、消息顺序: 更新 memory Skill 和 `memory-context-flow.md`。
- 把占位逻辑替换成真实实现: 更新 `placeholder-risk-register.md`。
- 提交前运行 `scripts/knowledge-check.sh diff`。
