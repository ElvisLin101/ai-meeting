# 项目能力与后续差距

代码核对日期：2026-11-23。本文总结当前仓库能力及待办；没有重新调研外部参考项目，也不将历史设计目标视为现有实现。

## 当前能力

| 领域 | 已实现 | 实际边界 |
| --- | --- | --- |
| Agent 执行 | ReAct、终止/熔断、工具执行与批调度、run/step 持久化、SSE | 批并行仅对整批声明 ConcurrencySafe 的工具；大结果仍截断 |
| Plan | 规则路由、任务依赖图、有界并行、增量 replan、重试及单 run 幂等账本 | 无整体 timeout，账本不支持重启续跑；agent 任务可有副作用 |
| 生命周期回调 | direct 与 plan 子 loop 的 run/LLM/tool 元数据回调及 Logging | 未覆盖 planner/replanner/plan 直调 tool；同步回调无 panic 隔离 |
| Agent 记忆 | identity/profile/summary/recent、估算 token 预算、增量压缩、画像 CAS/确认/审计 | 各段上限不等于全部输入硬预算；画像触发阈值与旧文档不同 |
| 普通 AI | JSON/SSE 聊天、configured 模型、字节阈值压缩摘要 | 无向量召回，中断不保存不完整 assistant |
| 统一存储 | MySQL conversations/messages、原子 sequence、快照与画像/run/面试表 | 删除子消息归属约束仍需修复；跨存储写入不统一为事务 |
| 面试 | 出题/评分/追问、状态机 CAS、条件回滚、幂等/题锁、归档/基础恢复 | hot 命中不补 cold/turn，无恢复等级和刷新协调器 |
| 并发去重 | Redis SingleFlight、AI 压缩/出题流式进度、非流式 run 合并 | 缓存轮询、context、续租、ownership/fencing 与 panic 清理有缺口 |
| 工程 | JWT/bcrypt、错误码、仓储接口注入、确定性/live eval、CI | 后台管理无固定并发上限；CI 未接知识检查 |

## 优先治理方向

1. 修复统一会话删除的子消息授权约束，并覆盖未拥有会话的请求。
2. 以故障测试驱动 SingleFlight：follower 轮询结果、首请求断连、续租失败、不响应 cancel 的旧 leader、所有权写入及 panic 清理；让实际业务路径明确进度策略。
3. 修复出题 key 的内容哈希与 follower 类型解码；验证业务去重，区分出题和评分路径。
4. 验证面试热快照分数/轮次完整性、局部 Redis key 缺失和并发恢复；再实现恢复互斥、等级和刷新协调器。
5. 为 Plan 定义整体 deadline 和有副作用任务策略；按需求评估持久任务账本和重启续跑。
6. 明确上下文硬预算、画像跨会话 sequence 与合并触发条件；增加后台任务并发控制和完整退出管理。
7. 加固 URL DNS/重定向校验、Agent 上传类型校验；按真实用例评估向量召回和 artifact 引用。

## 文档和验证

- 入口 `AGENTS.md`，模块 `skills/*/SKILL.md`，细节在同目录 routes-map/data-models/memory-context-flow/interview-runtime-governance/placeholder-risk-register。
- `scripts/knowledge-check.sh diff` 覆盖暂存、未暂存、未跟踪文件，`full` 检查 Go 文件锚点；均不替代语义核对。
- CI 当前执行 gofmt/build/vet/race/eval/覆盖率阈值 35%；live eval 需要真实模型和存储，不进 CI。
- sfprobe/sfcluster 历史数据是模拟执行测量，不能证明线上业务完整可靠或故障旧主被隔离。新的可靠性结论应保留输入、失败注入和差异化结果证据。
