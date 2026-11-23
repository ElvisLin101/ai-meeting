# 面试运行态：当前流程与待实现治理

代码核对日期：2026-11-23。本文以当前代码为依据；末尾的恢复治理目标尚未实现，不能用于推断运行时保证。

## 存储分工

- MySQL：interview_sessions、interview_questions、interview_records、热/冷快照、turn archive；conversations 的 scene=interview 记录用于列表。
- Redis：flow、score、follow-up、questions、suggestions、resume context/score、turn log、processing/replay。
- 领域及行模型：`models/interview.go`、`models/interview_runtime.go`、`models/interview_sql.go`；仓储：`repositories/mysql/interview_repository.go`。

Redis 是运行时热状态，快照是检查点。已写 Redis 的最新轮次未必已进入 MySQL 快照，不能承诺所有 key 丢失后完整恢复最新进度。

## 答题流水线

入口：`services/interview/interview_service.go`；编排：`services/interview/flow/answer_pipeline.go`。

1. 请求未传 request_id 时按输入生成 SHA 请求标识；幂等服务先看 replay，再申请 processing。
2. processing TTL 为 300s，replay TTL 为 24h（`services/interview/flow/idempotency_service.go`）。
3. 恢复运行态并获取题级锁；锁 TTL 300s，facade 执行预算 120s（`pkg/lock/lock.go`）。
4. 评分只产生评估结果，再推进 flow、写 score 和 turn log；状态机用版本 CAS 控制并发（`services/interview/flow/flow_state_machine.go`）。
5. 评分失败回到 ASKING；推进后失败调用 `services/interview/runtime/flow_cache.go` 的 RollbackFlowCAS，仅在 flow 仍为本请求推进后的版本时回滚。不得改成无条件 SaveFlow。
6. turn log 写失败由 `services/interview/flow/turn_repair_service.go` 补偿；成功提交先记录 replay，再启动独立 goroutine，以 10s 预算读取最新 flow 并刷新快照/归档，不阻塞响应。

processing、题锁、flow CAS 各有职责，不构成 MySQL/Redis 跨存储事务。

## 出题与模型调用

`services/interview/evaluation/extraction_service.go` 出题使用 configured AI client、SingleFlight 和 writer 进度；题目、简历上下文等进入 question cache。`services/interview/evaluation/evaluation_service.go` 评分使用 JSON 流式解析，未接 SingleFlight。追问在 `services/interview/evaluation/followup_service.go`。

出题 hashContent 仅取前 64 字符，不能作为可靠内容哈希；follower 解码类型与 *ExtractionResult 断言存在不匹配，详见风险登记。

## EnsureRuntime 实际恢复分支

代码：`services/interview/runtime/snapshot_service.go`。

1. Redis 已有 flow：立即返回，不检查其他运行态 key 是否齐全。
2. 无 flow 且有 MySQL hot snapshot：回填 flow、score、follow-up 后立即返回，不继续补齐 cold 材料和 turn log。
3. 无 hot snapshot：尝试读取 cold snapshot 与 archives 回填对应数据，最后返回 nil；没有统一的完整性判定或写入许可等级。

当前没有恢复锁/rehydrate scope，没有 EXACT / READ_ONLY 等级检查，没有主数据完整兜底。代码中出现的设计常量不代表这些机制已接入。

## RefreshAfterAnswerCommitted 实际刷新

1. 读取当前 hot snapshot；读失败返回，LastMutationID 已等于 requestID 则短路；否则将轮次归档到 MySQL。
2. buildHotSnapshot 构建检查点，有旧快照则最多三次版本 CAS，没有则 upsert。
3. 失败写 warn，CAS 重试耗尽记录 metric；并未通过协调器安排防抖、强制 flush 或定时重试。调用方先记录 replay 再异步刷新，因此可回放成功响应时检查点未必已更新。

当前没有 HotRefreshCoordinator、单调性拒绝或 soft replay。buildHotSnapshot 的 ScoreSum/ScoreCount 继承已有热快照，未从本轮评分累加；需验证丢 Redis 后的计分完整性。

## 已实现与待实现的区别

| 能力 | 当前状态 |
| --- | --- |
| 显式状态机、flow CAS、条件回滚 | 已实现 |
| processing/replay 幂等、题锁 | 已实现，受 TTL 和执行预算约束 |
| turn log 修复、轮次归档、热快照 CAS | 已实现 |
| hot/cold/archive 分支回填 | 已实现基础版本，非完整恢复保证 |
| 恢复互斥锁与完整性等级 | 未实现 |
| 最新已提交轮次的单调快照、完整分数重建 | 需治理和验证 |
| 防抖刷新、forceFlush、soft replay | 未实现 |
| 所有模型路径共享去重 | 未实现，评分未接 SingleFlight |

## 后续治理与验证

- 用真实提交轮次验证快照 ScoreSum/ScoreCount，分别丢失 flow、score、题目、cold 材料和 turn key，核对恢复是否完整。
- 若引入恢复等级，应根据可恢复材料定义允许继续写的条件，缺少关键状态时返回明确错误，不能只依赖常量命名。
- 若引入刷新协调器，应实现版本单调性、退出 flush 和失败重试，并验证多个请求/实例同时刷新。
- 保持条件回滚及幂等 replay 语义，测试推进后写失败与其他请求已提交的场景。
- 新增字段同步 `data-models.md`，新增接口同步 `routes-map.md`，更新 Interview Skill 和风险登记并运行 `scripts/knowledge-check.sh diff`。
