---
name: ai-meeting-user-auth
description: 当需求涉及登录、注册、JWT、用户上下文、用户资料、管理员权限或 AuthMiddleware 时使用。
---

# AI-Meeting User Auth Skill

## 何时使用

读取本 Skill 的场景:

- 登录、注册、登出、检查登录状态。
- JWT 生成、解析、过期时间、上下文键。
- 用户查询、分页、资料更新、管理员设置。
- 调整认证中间件或给接口增加强认证。

## 代码地图

- 路由: `api/routes/routes.go` 中 `setupUserRoutes`。
- 中间件: `api/middleware/auth.go`。
- Handler: `api/handlers/user_handler.go`。
- Service: `services/user/user_service.go`。
- MySQL 仓储: `repositories/mysql/user_repository.go`。
- DTO: `dto/user.go`。
- 模型: `models/user.go`。

## 核心流程

`AuthMiddleware`

- 全局挂载在 `routes.SetupRouter`。
- 缺少 Authorization header 或 Bearer 格式错误时, 当前行为是 `c.Next()`, 不会拦截。
- token 有效时设置 `username` 和 `user_id`。
- `RequireAuth` 已定义, 但当前路由没有使用。

`Login`

- 路由: `POST /api/xunzhi/v1/users/login`。
- `UserService.Login` 按 `username` 和 `status=1` 查询。
- 当前密码是明文比较。
- `GenerateToken` 当前调用传入 `string(rune(user.ID))`, 这不是十进制 ID 字符串。改动前要确认前端和面试模块如何使用 `user_id`。

`Register`

- 路由: `POST /api/xunzhi/v1/users/register`。
- 当前先查重, 再创建普通用户, `status=1`, `is_admin=false`。
- 查重时除 `gorm.ErrRecordNotFound` 以外的 DB 错误目前会继续创建用户, 修复时要保持错误语义清晰。

`UserAdmin`

- `IsAdmin` 根据 JWT 中 `username` 查 `is_admin`。
- `AddAdmin` 当前用 `ShouldBindJSON(&username)` 绑定原始 JSON 字符串。

## 修改指南

- 改 JWT claim 时, 同步检查 Agent/AI 使用 `username` 的地方和 Interview 使用 `user_id` 的地方。
- 给接口增加强认证时, 可以接入 `RequireAuth`, 但要保留登录/注册/公开接口的放行策略。
- 引入密码哈希时, 需要兼容旧密码或提供迁移策略。
- 修改用户模型字段时, 更新 `data-models.md`。

## 当前风险

- 密码明文存储和比较。
- 中间件默认放行, 安全边界依赖各 handler 手动检查上下文。
- `user_id` 的 token 写法可能不是预期 ID。
- 管理员设置缺少操作者权限校验。
