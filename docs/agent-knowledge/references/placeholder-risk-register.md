# 实现边界与文档漂移登记

代码核对日期：2026-11-23。本文件登记当前实现缺口，已完成能力不继续标作占位。修复时更新条目，并以测试证据说明边界。

## 已校正的过期结论

- Agent Chat 已有 SSE 模型调用与 user/assistant 持久化；会话创建支持指定 agent_id，默认 1 并校验配置；服务单例使用 sync.Once。
- 旧 Agent Chat 不压缩，但 Agent Run 已有 token 预算四层上下文、增量摘要、候选/画像 CAS、确认接口与调试页。
- 会话和消息统一存 MySQL conversations/messages，用 scene 区分；AiConversation/AgentConversation 是兼容模型，面试创建双写 interview_sessions 与 scene=interview 列表记录，列表失败有补偿删除。
- 用户密码 bcrypt、旧明文登录迁移、受保护路由 RequireAuth、管理员操作者校验均已实现。
- 消息和 turn archive 序列使用 session_sequences 原子分配；面试评分、出题、追问、简历预览、记录归档已接真实流程。

## 统一存储与用户隔离

- `repositories/mysql/conversation_repository.go`：DeleteConversation 按 session_id + user_id 删除会话，但子 messages 只按 session_id 删除。仓储未保证父子删除共享归属约束，需核对全部调用入口并修复。
- `models/agent_runtime.go`：compressed_contexts 仅 session_id 唯一，memory_scope 不是独立唯一维度；AI 当前存原始 sessionID。不能按旧 ai: 前缀文档新增隔离逻辑。
- `TotalLength` 在 AI 路径存字节数，在 Agent 路径存估算 token 数，跨路径统计不能直接合并。

## SingleFlight

代码：`pkg/singleflight/singleflight.go`、`repositories/redis.go`。

- followerGotResult 与“继续等待”状态均为 0，轮询结果命中可能被忽略；Pub/Sub 通知路径成功不代表轮询兜底正确。
- leader 执行 context 继承首个 HTTP request context，leader 客户端断连可影响共享执行和 follower。
- 续租结果/错误未完整处理；Marshal、Redis 结果写入和部分 Pipeline 错误被忽略。结果写入及清理 progress 缺少 ownership/fencing 校验。
- fn 不响应 cancel 时可能继续执行及续租，接管无法保证旧 leader 停止；panic 路径、锁释放和本地 WaitGroup 等待取消尚不完整。
- AI 压缩与面试出题更新 writer，Agent run 与画像合并未更新；业务长任务可能被 30s 停滞检测误判。
- 新 leader 初始占位进度减少首段输出前的误判窗口，未解决上述问题。历史 sfprobe/sfcluster 使用模拟函数，换主中旧/新 leader 返回相同结果且旧主响应取消，不能证明旧主隔离或全部业务路径可靠。
- 流式 Agent run 使用独立 Redis 幂等锁，不共享 SSE chunk；不能称为流式 SingleFlight 结果复用。

## Agent

- `services/agent/context/assembler.go`：system 各段独立截断，再给 recent 剩余预算，启发式 token 估算不是包含全部输入的严格硬预算。
- `services/agent/context/store.go`：Redis miss 近期消息 MySQL 回填最多 500 条；恢复不等于装入所有未覆盖消息。
- `services/agent/context/profile_consolidate.go`：仅在无候选且 user 消息不足 3 条时跳过，候选阈值 5 未使用；用户级 LastConsolidatedSeq 与会话级序列比较存在边界。
- `services/agent/context/workers.go`：每任务 goroutine + cancel/WaitGroup，无固定 worker/队列/并发上限；旧画像 ExtractAsync 与 AI 压缩不在停机等待范围。
- `services/agent/plan/service.go`：TotalTimeout 只用于 synthesis loop，planner/replanner/任务/合成没有整体超时；tool 任务只读，但 agent 任务可调用有副作用工具。幂等账本只在单进程、单 run 内有效，不能重启续跑。
- `services/agent/callback/callback.go`：同步回调要求并发安全，无 panic 隔离；planner/replanner 模型调用及 plan 直调 tool 未覆盖；tool end 在整批执行完成后触发。
- 网页工具目前校验 URL host/显式 IP，未校验 DNS 解析地址与重定向链；公网访问保证需进一步加固。
- `api/handlers/agent_handler.go`：上传已有 UUID 重命名及 10MB 限制，尚无 magic bytes 校验和完整消费链路。
- `services/agent/agent_properties_loader.go`：本地配置缓存不自动跨实例同步；AgentProperties GetByPage 固定 10 条。
- 工具大结果仍截断，未转 artifact ref；工具单步批调度没有 depends_on 图，Plan 的任务图不能当作单步工具 DAG。

## 普通 AI

- `services/ai/ai_memory_service.go`：字节阈值仅当前实例运行时生效，重启恢复默认；摘要有损，无向量召回。
- AI 摘要先写 Redis，再独立 goroutine upsert MySQL，存在未持久化窗口；本地兜底摘要按字节切首尾，可能切坏 UTF-8。
- `services/ai/ai_chat_service.go`：SSE 中断不保存不完整 assistant；AI 压缩独立 goroutine 未纳入 Agent 后台管理。
- `clients/ai_model_client.go`：OpenAI-compatible 协议解析，非兼容 provider 需适配。
- AI/Agent 配置密钥仍明文存储；本地配置或环境变量需提供实际模型凭证。

## 面试

- `services/interview/evaluation/extraction_service.go`：hashContent 仅截前 64 字符，可能产生去重 key 碰撞；SingleFlight follower JSON 解码为通用对象，直接断言 *ExtractionResult 不匹配。
- `services/interview/evaluation/evaluation_service.go`：评分为 JSON 流式调用，未接入 SingleFlight；不能套用出题的去重描述。
- `services/interview/runtime/snapshot_service.go`：已有 Redis flow 就返回；hot snapshot 命中只回填 flow/score/followup 并返回，不继续补 cold 材料/turn。无恢复锁、scope、EXACT/READ_ONLY 等级判断或完整主数据兜底。
- 快照刷新归档后最多三次 CAS，失败告警/指标；无 HotRefreshCoordinator、防抖/forceFlush、单调性拒绝或 soft replay。buildHotSnapshot 继承已有 ScoreSum/ScoreCount，未从本轮评分更新，恢复计分完整性仍需验证。
- 面试会话与列表双写只有补偿，未形成事务；记录按 session 查询仅取第一条，不适合多题记录展示。
- 答题推进后失败已做条件 CAS 回滚，turn log 失败已有补偿；修改时禁止回退为无条件覆盖并发进度。

## 用户

- `services/user/user_service.go`：注册查重遇到非 gorm.ErrRecordNotFound 的数据库错误仍继续创建，需完善错误处理。
