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

- 全局挂载在 `routes.SetupRouter`, 只做 JWT 解析(有效则设置 `username`/`user_id`), 缺失/无效不拦截——公开接口需要放行。
- **受保护路由已挂 `RequireAuth`**(见 `routes.SetupRouter` 的 `authed` 组): Agent/AI/Interview/Media 全部 + 用户模块的 logout/资料/管理员/分页, 无有效 JWT 直接 401。
- token 有效时设置 `username` 和 `user_id`。
- `RequireAuth` 检查 `username`, 401 响应 `{"code":-101,"error":"Unauthorized"}`。

`Login`

- 路由: `POST /api/xunzhi/v1/users/login`。
- `UserService.Login` 按 `username` 和 `status=1` 查询。
- 密码 bcrypt 哈希存储与比较（`services/user/user_service.go: verifyPassword`; 旧明文数据登录成功后自动迁移为哈希）。
- `GenerateToken` 传入 `strconv.FormatUint(uint64(user.ID), 10)` 作为 `user_id`。

`Register`

- 路由: `POST /api/xunzhi/v1/users/register`。
- 当前先查重, 再创建普通用户, `status=1`, `is_admin=false`。
- 查重时除 `gorm.ErrRecordNotFound` 以外的 DB 错误目前会继续创建用户, 修复时要保持错误语义清晰。

`UserAdmin`

- `IsAdmin` 根据 JWT 中 `username` 查 `is_admin`。
- `AddAdmin` 用 `ShouldBindJSON(&username)` 绑定原始 JSON 字符串, **操作者必须为已登录管理员**(否则 `-403`)。

## 修改指南

- 改 JWT claim 时, 同步检查 Agent/AI 使用 `username` 的地方和 Interview 使用 `user_id` 的地方。
- 给接口增加强认证时, 可以接入 `RequireAuth`; 新增接口默认放受保护组(`authed`), 仅登录/注册/check-login/is-admin/has-username 放公开组。
- 引入密码哈希时, 需要兼容旧密码或提供迁移策略。
- 修改用户模型字段时, 更新 `data-models.md`。

## 当前风险

- ~~密码明文存储和比较。~~ **已完成: bcrypt 哈希 + 旧明文兼容迁移。**
- ~~中间件默认放行, 安全边界依赖各 handler 手动检查上下文。~~ **已完成: 受保护路由挂 `RequireAuth` 强制认证(公开接口除外)。**
- ~~管理员设置缺少操作者权限校验。~~ **已完成: `AddAdmin` 校验操作者为管理员。**
