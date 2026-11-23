# Go Backend Rules

## 基本规范

- 使用 Go 1.24.1, 保持现有 `handlers -> services -> repositories/models` 分层。
- Handler 只处理 HTTP 入参、认证上下文、状态码和 DTO 转换。
- Service 放业务流程和跨仓储编排，新增逻辑通过仓储接口注入，不直接拼 SQL；旧 Agent SSE 链路仍使用 Gin，调整时以实际代码为准。
- 持久化读写放 `repositories/mysql`; Redis 热数据放 `repositories/redis_hot.go`; 不要在 service 里直接拼 SQL。
- Redis 当前仍在根 `repositories`, 通过 `repositories.RedisClient` 使用。
- 外部 HTTP/API 调用放 `clients`, 不放在 service 或 repository。
- 错误返回给调用方, 不在普通业务流程里 `panic`。
- 统一错误码: 业务错误用 `pkg/ecode`（`ecode.New(code, msg)` / `ecode.Wrap`）, handler 出口统一走 `api/resp` 的 `Respond`/`Fail`, 错误响应 `{"code": N, "error": "msg"}`, 禁止散落 `c.JSON(status, gin.H{"error": ...})`。
- 新增复杂函数时优先传 `context.Context`; 现有代码大量未传 context, 新增可逐步改善, 不做无关大重构。

## 数据和安全

- 用户隔离查询必须带 `username` 或 `user_id`, 不能只按 `session_id` 查询敏感数据。
- 认证相关改动必须同时检查 `api/middleware/auth.go` 和具体 handler 的上下文键读取方式。
- 受保护路由统一挂 `RequireAuth`(`routes.SetupRouter` 的 `authed` 组): **新增接口默认放受保护组**, 仅登录/注册/check-login/is-admin/has-username 放公开组。
- 密码已 bcrypt 哈希存储与比较（`services/user/user_service.go: verifyPassword`, 旧明文登录成功自动迁移）; AI/API 密钥仍直接存储在表字段中, 修复时同步更新 user-auth 和 ai Skill。
- 上传文件已做大小限制(10MB, `http.MaxBytesReader`)与 UUID 重命名防路径穿越（`api/handlers/agent_handler.go`/`api/handlers/interview_handler.go`）; 新增上传接口必须同样处理, 并校验类型(magic bytes)。
- 现有面试 PDF 上传已校验魔数，Agent 通用上传尚未校验；不能把新增接口要求当成旧接口已有保证。
- 统一消息表为 `messages`，会话表为 `conversations`（`scene=ai/agent/interview`）。删除子消息也必须受会话归属保护，不能仅依赖父记录删除条件。
- Agent 后台任务管理当前是每任务 goroutine + cancel/WaitGroup，无固定 worker 数或并发上限；新增异步任务需明确取消、超时和退出等待。

## 变更检查

- 改路由: 更新 `docs/agent-knowledge/references/routes-map.md`。
- 改模型: 更新 `docs/agent-knowledge/references/data-models.md`。
- 改长上下文、压缩、消息顺序: 更新 memory Skill 和 `memory-context-flow.md`。
- 把占位逻辑替换成真实实现: 更新 `placeholder-risk-register.md`。
- 提交前运行 `scripts/knowledge-check.sh diff`。
- 文档更新后运行 `scripts/knowledge-check.sh full`；脚本只能验证文件锚点和变更路由，语义仍需反向核对代码。
