# Interview Runtime Governance（面试长会话运行态治理）

> 本文档是面试运行态治理的设计知识库, 不是代码流程说明。先理解"为什么会丢状态"和"分层", 再看"恢复"和"更新"两条链路。

## 1. 问题: 为什么会丢状态

### 1.1 丢状态不是"数据没了", 是"系统不知道进行到哪了"

模拟面试是 10-20 分钟的**在线过程型长会话**, 后端必须持续维护一整套运行态: 当前第几题、是否追问、追问几轮、最近几轮问答、流程节点(出题/作答/评分)、总分和各维度分。只要缺一部分, 系统就无法安全判断"下一步该继续问哪题、该不该追问、该不该累计分数"。

所谓丢状态, 本质是: **系统已无法准确判断这个会话现在进行到哪里、下一步该怎么走、还能不能继续写。**

### 1.2 长会话为什么特别容易丢状态

长会话运行态同时具备六个特征: 跨请求、跨时间、高频变化、依赖顺序、依赖幂等、依赖并发控制。它不是普通缓存:

- 半小时内缓存过期
- 某次请求写成功但补充刷新失败
- 实例抖动导致局部状态没写完
- 并发请求互相覆盖
- 恢复时读到不完整的中间结果

### 1.3 最典型的丢状态时间线

风险点不在某一步失败, 而在 **"运行态已变、检查点未稳"** 这个窗口:

1. 用户答第 5 题, 幂等层抢到 processing
2. ensureRuntime 恢复运行态, 题级锁锁住当前题
3. 评估算分(只算不入账)
4. Redis 运行态先推进: flow 前进、分数写入
5. turn log 写入 Redis
6. 标记成功, 触发快照刷新
7. **若此时 Redis key 过期/被覆盖, 而快照刷新还没完成**: 新请求进来发现题号不完整、turn log 不完整、replay 不完整、恢复材料落后于真实进度
8. 系统尝试重建, 但最新一轮还没沉淀成检查点 → 用户体感"明明答完了, 系统却像没答一样"

### 1.4 结论: 不幻想永不丢, 而是丢了也能恢复

成熟的设计承认运行态会缺失, 提前设计好: 恢复入口、恢复依据、恢复边界。于是有两条链路:

- **恢复机制**: 以 `ensureRuntime(...)` 为核心
- **数据更新机制**: 以 `refreshSnapshot(...)` / Patch + CAS + 幂等补偿 为核心

## 2. 分层: 什么放 Redis, 什么放 Mongo

### 2.1 四层存储 + 主数据源

| 层 | 存储 | 存什么 | 变化频率 | 恢复角色 |
|---|---|---|---|---|
| 在线运行态 | Redis(TTL 24h) | flow/分数/最近轮次/幂等集/材料 | 每轮都改 | 命中即零开销 |
| 热快照 | Mongo `interview_session_runtime_hot_snapshot` | flow/分数聚合/最近20轮窗口/幂等控制字段/snapshotVersion | 每轮答题 commit 后刷 | Redis miss 后第一重建源 |
| 冷快照 | Mongo `interview_session_runtime_cold_snapshot` | 题面/建议/简历上下文/评分材料 | 出题/评价/finalize 时刷 | 补充材料 |
| 轮次归档 | Mongo `interview_session_turn_archive` | 完整不可变的轮次历史, 按 seq 单调 | 每轮一条 | 热快照窗口溢出后的权威源 |
| Mongo 主数据 | Mongo `interview_sessions`/`interview_questions` | 会话身份/题目材料 | 低频 | 快照+归档全空时的最后重建源 |
| MySQL 最终报告 | MySQL `interview_records` | 面试结束总结报告 | finalize 一次 | 不参与运行态恢复, 仅业务展示 |

### 2.2 热冷快照的分工(关键设计)

- **热快照**: 高频变化的流程态, 上 CAS 乐观锁(`snapshotVersion`), 有单调性校验(`lastTurnSeq`/`archiveWatermark`/`scoreCount`/`flow.currentIndex` 回退直接抛异常)。只存最近 20 轮窗口, 靠 `archiveWatermark` 水位线衔接 TurnArchive。
- **冷快照**: 低频变化的材料, **故意不上 CAS**(last-writer-wins), 靠"ACTIVE 阶段跳过冷层刷新"避免每轮重写。高频字段必须防并发覆盖, 低频材料覆盖即可——这个取舍是分层的精髓。

### 2.3 Redis key 命名规则

统一前缀 `interview:` + 业务域 + `:session:` + sessionId:

| key 模式 | 结构 | 用途 |
|---|---|---|
| `interview:questions:session:{sid}` | Hash | 主题号→题目内容 |
| `interview:suggestions:session:{sid}` | Hash | 题号→建议 |
| `interview:flow:session:{sid}` | Hash | 面试流程状态 |
| `interview:follow_up_questions:session:{sid}` | Hash | 追问题号(含-F)→内容 |
| `interview:score:session:{sid}` | Value | 会话总分 |
| `interview:score_sum:session:{sid}` | Value | 累计得分和 |
| `interview:score_count:session:{sid}` | Value | 计分次数 |
| `interview:turns:session:{sid}` | List | 完整轮次日志(JSON) |
| `interview:answer:req:session:{sid}` | Set | 答题 requestId 幂等集 |
| `interview:turn:req:session:{sid}` | Set | turn requestId 幂等集 |
| `interview:resume_context:session:{sid}` | Value | 简历解析上下文 |
| `interview:direction:session:{sid}` | Value | 面试方向 |

幂等独立 key:
- `interview:answer:idempotency:processing:{sid}:{reqId}` — 处理中标记(TTL 300s)
- `interview:answer:idempotency:replay:{sid}:{reqId}` — 成功结果缓存(TTL 24h)

锁 key:
- `interview:runtime:rehydrate:lock:{sid}` — 恢复分布式锁
- `interview:answer:lock:{sid}:{questionNo}` — 题级锁

## 3. 恢复机制: ensureRuntime

### 3.1 核心原则

**恢复只写 Redis, 绝不回写 Mongo 快照。** 避免恢复路径触发写放大, 快照由业务节点的协调器异步刷。

### 3.2 恢复链路

```
ensureRuntime(sessionId, loadMode, scope)
  ├─ 1. isRuntimeReady? 查 Redis(按 scope 检查对应 key 非空)
  │     命中 → 返回 EXACT/CACHE 视图(零开销快速路径)
  ├─ 2. miss → 抢 rehydrate 分布式锁(wait=0, lease=60s)
  │     拿不到(follower) → 轮询 4×80ms 复查 Redis(复用 leader 重建结果)
  ├─ 3. 双重检查(拿锁后再查一次, 防 leader 已建好)
  └─ 4. rebuildRuntime
       ├─ 4a. findSnapshot(热+冷组装) → 够用 → writeSnapshotToCache → RUNTIME_SNAPSHOT
       └─ 4b. 不够 → 下钻 Mongo 主数据(Session/Question + loadPersistedTurns)
              → buildRuntimeMaterial(每个字段多级回退) → writePartialMaterial
              → 全空 → NONE/READ_ONLY(不下钻 MySQL, 宁可只读)
```

### 3.3 关键设计点

1. **leader/follower 模式**: follower 拿不到锁不阻塞等, 轮询 Redis 复用 leader 重建结果, 减少锁竞争。
2. **scope 精细化**: 6 种 scope(FLOW_ONLY/SCORE_ONLY/PLAYBACK_ONLY/MATERIAL_ONLY/HOT_RUNTIME/FULL_RUNTIME), 调用方只请求需要的子集。
3. **置信度分级**: EXACT(缓存/快照命中, 可写) → DERIVED(主数据派生, 可写但推导值) → READ_ONLY(材料不足, 只读) → TERMINAL(会话已结束)。`canWrite()` 只在 EXACT/DERIVED 时为 true, 防止从残缺状态推进业务。
4. **每个字段多级回退**: turns 优先级 `TurnArchive > Redis缓存 > snapshot.recentTurns > record JSON`; 分数 `snapshot → 从turns派生 → record`。任何单点缺失都不让重建失败。

### 3.4 恢复边界

- 四源全空 → NONE
- 缺题目且要求可写 → 降级 TERMINAL/READ_ONLY
- 任何异常 → 兜底 READ_ONLY
- **宁可只读也不让调用方在残缺状态上写。**

## 4. 数据更新机制: refreshSnapshot + Patch + CAS + 幂等补偿

### 4.1 触发: 合并写 + 防抖/强刷二选一

`HotRefreshCoordinator` 是单节点本地协调(非全异步队列):
- 普通中间态(出题/神态评价) → 防抖延迟刷(150ms 窗口, 最大聚合 500ms), 同 session 多次意图合并。
- 答题 commit / finalize → `forceFlush=true`, 调用线程同步等(最多 600ms), 保证检查点及时落盘。
- 跨节点并发靠 Mongo CAS 兜底, 不靠这个协调器。

### 4.2 落盘: CAS + 单调性 + 幂等短路

`refreshSnapshot` 重试循环(最多 3 次):
1. `archiveTurn` 先写 TurnArchive(requestId 幂等查重, 返回 seq 作 watermark)。
2. `buildHotPatch` 构造增量补丁, `snapshotVersion = current+1`。
3. 三个短路: `shouldSkipHotPatch`(全相等跳过)、`isMutationAlreadyApplied`(`lastMutationId==requestId` 跳过)、`seedHotSnapshot`(快照不存在写种子重试)。
4. 单调性校验: `lastTurnSeq/archiveWatermark/scoreCount/flow.currentIndex` 回退直接抛异常(不容忍, 不重试)。
5. CAS 写入: `updateFirst({sessionId, snapshotVersion:expected}, {$set:...})`, `modifiedCount>0` 才成功。失败 → 重读 + 复判 mutation → 退避重试。

**CAS 用 `snapshotVersion` 单字段**, 只防并发覆盖; 乱序到达由单调性校验兜底。冷层故意无 CAS。

### 4.3 幂等补偿: 三层防线

| 层 | 机制 | 覆盖场景 |
|---|---|---|
| ① Redis replay key(24h) | `tryStart` 命中即回放 | 客户端 24h 内重试, 最快 |
| ② 热快照 lastMutationId + TurnArchive | `findReplayResponse` 软回放 | replay key 过期, 重建响应并补写 replay |
| ③ lastCommittedTurnDigest(SHA256) | 题号+答案内容摘要匹配 | requestId 缺失时的兜底 |

### 4.4 答题完整时序

```
幂等检查(tryStart) → ensureRuntime(READ_WRITE) → 题级锁 → 锁后再校验题号
→ 评估(只算不入账) → 推进flow → 写分数(commitScoreAtSuccess) → 写turn log
→ markSucceeded(写replay+删processing) → 刷新快照(forceFlush)
```

- **回滚粒度**: 只有"分数提交失败"回滚 flow(恢复到推进前), 且发生在写 turn log 之前, 不会有"写了 turn log 但分数没入账"的脏数据。
- **turn log 写失败不回滚**, 走异步补偿队列(每 3s 重试, 最多 6 次), 因为响应已组装成功应当返回客户端。
- **markSucceeded 不可逆地先于快照刷新**: 即使快照 CAS 3 次用尽失败, replay key 已写, 客户端重试命中 replay; 此窗口靠软回放(热快照/归档)兜底。

## 5. Go 落地映射

### 5.1 现状评估

Go 侧面试模块会话/题目/记录已迁 Mongo, 运行态状态机 P0 骨架已落地。

**已落地(P0)**:
- 状态机模型: `models/interview_runtime.go`(`InterviewFlowState` + `InterviewTurnLog` + 阶段枚举)
- Redis key 常量: `services/interview/runtime/cache_keys.go`
- flow 缓存 + CAS: `services/interview/runtime/flow_cache.go`(Lua 乐观锁, 5 次重试)
- 分数缓存: `services/interview/runtime/score_cache.go`(Lua 原子累计平均分)
- turn log 缓存: `services/interview/runtime/turn_log_cache.go`(List + requestId 幂等)
- 状态机: `services/interview/flow/flow_state_machine.go`(转移方法 + 合法性校验 + 回滚)
- 追问规则链: `services/interview/flow/follow_up_rule.go`(纯 Go 函数, 5 条短路规则)
- AI 调用: `services/interview/evaluation/`(评分/出题/追问, 走 DeepSeek 流式 + JSON 解析容错)
  - 评分/出题接入分布式 SingleFlight(流式 chunk 作心跳, 同一题/同一简历并发去重)
  - 追问走流式但不接 SingleFlight(输入差异大, 重复概率低)
- 通用分布式锁: `pkg/lock/lock.go`(SetNX + Lua 释放, 题级锁/幂等锁)
- 答题流水线: `services/interview/flow/answer_pipeline.go`(幂等→锁→评分→推进flow→写分→turn log→标记成功)
- 幂等机制: `services/interview/flow/idempotency_service.go`(processing/replay 双 key + requestId 自动生成)

**现成可复用**:
- Redis 客户端 `repositories.RedisClient`
- 分布式 SingleFlight `repositories.SingleFlight`(可复用于流式评分去重, 但常量硬编码不可配)
- DeepSeek 客户端 `clients/ai_model_client.go`(CallConfiguredAIChatStream/Chat, OpenAI 兼容)
- 场景路由 `services/agent/agent_scene.go`(5 场景含面试评分, 但 `ResolveRequired` 从未被调用)
- AgentChatSSE 范式 `services/agent/agent_service.go`(归属校验→解析props→存消息→调DeepSeek→存结果)

**需要新建(P1/P2)**:
- 通用分布式锁(同题锁/幂等锁/状态机推进锁, 语义是"拒绝/排队"不是"去重")
- Mongo CAS / FindOneAndUpdate 带条件(状态机原子推进)
- 原子 sequence 生成(`$inc` 计数器, 当前 `nextXxxSequence` 非原子)
- 面试配置块(singleflight 常量改可配)
- 面试运行态 repository(Mongo 热/冷快照 + TurnArchive 持久化)
- 幂等机制(processing/replay 双 key)
- 答题流水线(pipeline 编排)
- 评分/出题的 AI 调用接 DeepSeek(CallConfiguredAIChatStream, aiID=0 走 config fallback)

### 5.2 singleflight 复用边界

- **可直接复用**: 面试评分/神态的流式 AI 调用去重, key 如 `interview:score:{sessionID}:{questionNo}`, 照搬 `services/ai/ai_memory_service.go` 的 onChunk→writer.Write 接法。
- **需改造**: 分 stage 可配 TTL/超时; 补 Fencing Token 防旧主换主后写库。
- **不能复用**: 同题互斥锁——singleflight 是"去重"语义(follower 拿 leader 结果), 同题锁要"拒绝/排队"语义(其他请求直接失败), 需另建 `pkg/lock`。

### 5.3 落地优先级

- **P0(已完成)**: 状态机模型 + Redis key 常量 + flow cache(CAS) + score cache(Lua 原子) + turn log cache + 状态机转移方法 + 追问规则链
- **P1(部分完成)**: 通用分布式锁 `pkg/lock` ✅ + 幂等机制 ✅ + 答题流水线 ✅ + 评分/出题/追问 AI 调用 ✅ + Mongo 热冷快照 + TurnArchive ✅ + ensureRuntime 恢复 ✅ + refreshSnapshot 更新 ✅ + turn log 异步补偿 ✅ + 面试报告归档 ✅ | 待做: 出题流程的冷快照在 ensureRuntime 中恢复材料的完善、原子 sequence
- **P2(待做)**: singleflight 分 stage 可配 + Fencing Token + worker pool/AI Guard

## 6. 修改时必查

- 设计变更时先回读本文档的分层和两条链路, 确认改动不破坏恢复/更新解耦。
- 新增 Redis key 时遵循 `interview:` 前缀规范, 同步更新本文档 §2.3。
- 改热快照字段时同步检查 CAS 条件(`snapshotVersion`)和单调性校验字段。
- Go 落地每完成一层, 回读 §5 确认复用/新建边界未被违反。
