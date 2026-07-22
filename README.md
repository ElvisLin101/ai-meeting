# AI-Meeting

基于 Go + Gin 的 AI 面试平台后端，包含通用 AI 对话和模拟面试 Agent 两大模块，支持对接 DeepSeek 等 OpenAI 兼容模型，支持简历驱动的面试出题、评分、追问全流程编排。

## 技术栈

| 维度 | 技术 |
|------|------|
| 语言 | Go 1.22 |
| Web 框架 | Gin |
| ORM | GORM（MySQL：用户/权限、AI/Agent 配置、文件资产、面试报告） |
| 文档存储 | MongoDB（会话、消息、压缩上下文、面试运行态热冷快照/轮次归档） |
| 缓存 | Redis（在线运行态、热缓存、分布式锁、SingleFlight） |
| 配置 | Viper |
| 认证 | JWT |
| AI 模型 | DeepSeek（OpenAI 兼容协议，全链路统一） |
| PDF 解析 | ledongthuc/pdf（简历文本提取） |

## 项目结构

```
├── api/                    # HTTP 路由、中间件、Handler
│   ├── handlers/           # 请求处理与 DTO 组装
│   ├── middleware/         # JWT 认证、CORS
│   └── routes/             # 路由注册
├── services/               # 业务逻辑层（按业务域拆分）
│   ├── agent/              # Agent 对话 + 场景绑定 + 启动缓存
│   ├── ai/                 # AI 对话 + 记忆压缩
│   ├── interview/          # 面试模块
│   │   ├── flow/           # 状态机 + 答题流水线 + 幂等 + 补偿队列
│   │   ├── runtime/        # Redis 缓存层 + 快照/恢复服务
│   │   └── evaluation/     # AI 评分/出题/追问 + JSON 解析容错
│   ├── metric/             # 异步指标服务
│   └── user/               # 用户管理
├── clients/                # 外部 API 客户端
│   ├── ai_model_client.go  # DeepSeek/OpenAI 兼容调用（普通 + SSE 流式 + JSON mode）
│   └── resume_parser.go    # PDF 简历解析
├── pkg/
│   ├── singleflight/       # 分布式 SingleFlight
│   └── lock/               # 通用分布式锁
├── repositories/           # 数据访问层
│   ├── mysql/              # MySQL 仓储（用户/配置/指标日志）
│   ├── mongo/              # MongoDB 仓储（会话/消息/运行态快照/归档）
│   └── redis.go            # Redis 初始化 + SingleFlight 全局实例
├── models/                 # 数据模型（GORM + BSON）
├── dto/                    # 请求/响应结构体
├── config/                 # 配置加载
├── static/                 # 前端页面
├── skills/                 # Skill 知识工程文档
└── docs/agent-knowledge/   # reference 文档（路由表/数据模型/运行态治理/风险登记）
```

## 核心亮点

### 1. 面试长会话运行态治理 — 可恢复的状态机

**问题**：模拟面试是 10-20 分钟的长会话，运行态（题号/追问轮次/分数/轮次日志）高频变化且强状态依赖。Redis 缓存过期或实例抖动会导致系统丢失"进行到哪一步"的认知，出现题号错乱、重复评分。

**方案**：Redis 在线态 + Mongo 热冷快照 + 轮次归档三层存储，Lazy Rehydrate 按需恢复。

```
Redis 在线运行态（TTL 24h）
  ↓ miss 后从 Mongo 重建
Mongo 热快照（CAS 乐观锁）    ← flow/分数/最近20轮窗口
Mongo 冷快照（无 CAS）         ← 题面/建议/简历上下文
Mongo 轮次归档（不可变）       ← 完整轮次历史
```

**关键设计**：

- **热冷分层快照**：热快照存高频流程态（CAS 乐观锁 + 单调性校验），冷快照存低频材料（无 CAS），ACTIVE 阶段跳过冷层刷新避免写放大
- **Lazy Rehydrate 恢复**：Redis miss 后从 Mongo 热快照恢复 flow/分数/追问题，冷快照恢复材料，TurnArchive 恢复完整轮次历史
- **分数提交失败回滚**：先拍 flow 快照后推进，分数提交失败时 RestoreFlow 回滚，保证"题号没推进但也没计分"的一致性
- **turn log 异步补偿**：写失败入 Redis 队列，定时重试最多 6 次，不阻断主流程返回
- **幂等三层防线**：Redis replay key（24h）→ 热快照 lastMutationId → TurnArchive 软回放

**代码位置**：`services/interview/flow/`、`services/interview/runtime/`

### 2. 分布式 SingleFlight — AI 调用去重 + 流式心跳

**问题**：负载均衡下，前端网络抖动导致同一用户同一 prompt 的请求分散到不同实例，每个实例各调一次 AI，成本翻倍。

**方案**：基于 Redis SET NX + Pub/Sub 实现分布式请求合并。

```
实例A 抢到锁 → 成为主节点 → 执行 AI 调用 → 写结果 → Pub 通知
实例B 没抢到 → 成为从节点 → Sub 等待 → 读结果复用
```

**关键设计**：

- **主从选举**：Redis `SET NX` 抢锁，第一个成功的是主节点，其余为从节点
- **AI 流式输出作心跳**：主节点每收到 AI 一段输出就写 Redis 进度（`字节数:时间戳`），从节点检测进度变化，30s 无变化判定卡死
- **自动换主**：从节点检测到主节点卡死后写 `cancelKey`（携带旧主 nodeID 做身份校验），旧主 cancel context 断开 SSE 不浪费 token，从节点重新抢锁成为新主
- **降级策略**：Redis 不可用时自动降级为本地 SingleFlight（`sync.Map` + `WaitGroup`）

**代码位置**：`pkg/singleflight/singleflight.go`

### 3. AI 记忆压缩 — 长对话上下文管理

**问题**：长对话场景下全量历史消息超出模型 token 限制，且每次都传全部历史浪费成本。

**方案**：MongoDB 原始消息 + Redis 热缓存 + Mongo 冷恢复三级存储 + 80/20 压缩策略。

```
MongoDB 原始消息（真相源）
  ↓ 80/20 压缩
Redis 压缩摘要（热缓存，7 天 TTL）
  ↓ 持久化
MongoDB 压缩快照（冷恢复）
```

**关键设计**：

- **80/20 压缩**：旧 80% 消息通过 AI 流式生成摘要（temperature=0.2），新 20% 保留原文，用 `index` 标记分界点
- **Redis 优先**：读取时先查 Redis 热缓存，miss 从 MongoDB 恢复并异步回填 Redis
- **分布式去重**：压缩请求接入分布式 SingleFlight，全集群同一会话只压缩一次，流式 chunk 作心跳
- **本地兜底**：AI 压缩失败时截取首尾 450 字作为 fallback，不因模型故障导致记忆链断裂
- **AI 侧专属**：记忆压缩仅服务 AI 对话；Agent 对话走 DeepSeek，不压缩，上下文由面试状态机管理

**代码位置**：`services/ai/ai_memory_service.go`

### 4. Skill 知识工程 — AI 辅助开发的文档防腐

**问题**：AI 辅助开发时文档容易和代码脱节，改了代码忘了改文档，文档变成"过期的谎言"。

**方案**：结构化的 Skill 知识体系 + 反向校验机制。

```
AGENTS.md（全局路由表 + 硬性约束）
  ↓ 按关键词路由
skills/<module>/SKILL.md（5 个模块级 Skill）
  ├── 代码地图（文件路径锚点）
  ├── 核心流程（业务链路描述）
  ├── 影响检查（改动时需同步检查什么）
  └── 当前风险（占位/未完成项登记）
  ↓ 按需加载
docs/agent-knowledge/references/（路由表、数据模型、运行态治理、风险登记）
```

**关键机制**：

- **反向校验**：改代码前用实际代码校验 Skill 中的代码锚点，不一致时先信代码再修文档
- **同步更新**：新增接口同步更新 `routes-map.md`，改字段同步更新 `data-models.md`，完成占位同步更新 `placeholder-risk-register.md`
- **防腐脚本**：`knowledge-check.sh` 检查文档引用的 `.go` 文件是否仍存在，`diff` 模式根据 git 变更推荐需更新的 Skill

**代码位置**：`AGENTS.md`、`skills/`、`docs/agent-knowledge/`、`scripts/knowledge-check.sh`

## 已实现功能

### AI 对话

- SSE 流式聊天（支持 DeepSeek reasoning_content 深度思考）
- 多模型支持（7 个预设模板 + 自定义 endpoint/apiKey）
- 双消息持久化（MongoDB）
- 上下文记忆压缩（80/20 策略 + 分布式 SingleFlight 去重 + 流式心跳）

### Agent 对话

- SSE 流式聊天（DeepSeek config fallback）
- 场景绑定热插拔（4 个业务场景 + 候选名称匹配 + 启动缓存）
- 双消息持久化 + 会话归属校验（MongoDB）

### 模拟面试

- **简历驱动出题**：PDF 上传解析 / 文本输入 → DeepSeek 流式出题 → 写 Redis → 初始化状态机
- **AI 评分**：DeepSeek 流式评分 + JSON mode + schema 校验 + 失败重试 + 字段别名归一化
- **追问机制**：纯 Go 规则链（完成态→上限→AI建议→低分→缺失点）+ DeepSeek 生成追问题
- **状态机**：5 阶段（INIT/ASKING/EVALUATING/FOLLOW_UP/COMPLETED）+ 合法转移约束 + CAS 乐观锁
- **答题流水线**：幂等 → 题级锁 → ensureRuntime → 评分 → 追问判定 → 推进flow → 计分 → turn log → 标记成功
- **运行态恢复**：Redis miss 从 Mongo 热冷快照 + 轮次归档重建
- **快照持久化**：答题 commit 后异步刷新热快照（CAS + 幂等短路）+ turn 归档
- **异步补偿**：turn log 写失败入队列重试
- **面试报告**：finalize 时从 TurnArchive 汇总写入 InterviewRecord
- **查询接口**：当前题/总分/题目/建议/简历分/雷达图（四维）/简历预览/恢复会话

### 基础设施

- 分布式 SingleFlight（主从选举 + 流式心跳 + 换主 + 降级）
- 通用分布式锁（SetNX + Lua 释放）
- 异步指标系统（channel + 批量 flush MySQL，全链路埋点）
- DeepSeek 客户端（普通调用 + SSE 流式 + JSON mode + reasoning 透传）
- PDF 简历解析（ledongthuc/pdf）
- Skill 知识工程体系（5 个 Skill + 反向校验 + 防腐脚本）

## 快速开始

### Docker Compose 一键启动

```bash
# 1. 复制配置模板并填入密码和 API key
cp config/config.example.yaml config/config.yaml

# 2. 一键启动（App + MySQL + Redis + MongoDB）
docker-compose up -d

# 3. MySQL 首次启动需手动建库（应用启动时会 AutoMigrate 建表，但不会建 database）
docker exec -it ai-meeting-mysql-1 mysql -uroot -p123456 -e "CREATE DATABASE IF NOT EXISTS ai_meeting DEFAULT CHARSET utf8mb4;"

# 4. 重启 app 容器让它建表后正常运行
docker-compose restart app

# 5. 访问前端页面
open http://localhost:8080
```

### 本地开发

```bash
# 1. 启动 MySQL / Redis / MongoDB
# 2. MySQL 首次需手动建库
#    mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS ai_meeting DEFAULT CHARSET utf8mb4;"
# 3. 复制配置模板
cp config/config.example.yaml config/config.yaml
# 4. 填入连接信息和 DeepSeek API key
# 5. 运行（启动时 AutoMigrate 自动建表）
go run main.go
# 服务监听 :8080
```

## License

MIT
