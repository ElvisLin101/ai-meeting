# AI-Meeting Agent Knowledge

这里是仓库内的 Code Agent 知识工程区, 参考 Skills 编程模式组织:

- 根目录 `AGENTS.md`: 全局入口、模块路由、硬性约束。
- 根目录 `skills/*/SKILL.md`: 模块级主知识文件, 控制在能快速读完的范围内。
- `references/*`: 按需加载的细节, 例如路由表、模型表、风险登记。
- `rules/*`: 当前项目通用工程规则。

使用方式:

1. 从 `AGENTS.md` 判断应该读取哪个 Skill。
2. 读根目录 `skills/` 下的 Skill 主文件后再决定是否读取 references。
3. 修改代码后运行 `scripts/knowledge-check.sh diff`。
4. 如果代码行为变化影响到文档, 同步更新对应 Skill 和 references。
5. 文档整理后运行 `scripts/knowledge-check.sh full`，检查全部 Go 文件锚点。

`diff` 同时扫描暂存、未暂存和未跟踪文件，建议按模块去重；覆盖当前 `services/<模块>/` 目录及 Agent callback/plan/context、统一 MySQL 仓储和 SingleFlight。脚本不修改文档，也不判断业务语义是否正确。

维护原则:

- Skill 不是普通说明文档, 它要告诉 Agent 在哪里读代码、如何判断影响面、哪些地方不能误判。
- 引用文件只放细节, 不把所有内容堆进 Skill 主文件。
- 发现文档与代码冲突时, 以代码为准并修正文档。
- 当前流程与未来设计分开标注；压测结果必须说明输入、依赖和验证范围，不能据此声称强一致或完整恢复。
- 最近全模块代码核对日期：2026-11-23。日期仅记录人工核对，不代表脚本完成语义验证。
