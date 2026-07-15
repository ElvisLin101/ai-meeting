# AI-Meeting Agent Knowledge

这里是仓库内的 Code Agent 知识工程区, 参考 Skills 编程模式组织:

- 根目录 `AGENTS.md`: 全局入口、模块路由、硬性约束。
- `../../skills/*/SKILL.md`: 模块级主知识文件, 控制在能快速读完的范围内。
- `references/*`: 按需加载的细节, 例如路由表、模型表、风险登记。
- `rules/*`: 当前项目通用工程规则。

使用方式:

1. 从 `AGENTS.md` 判断应该读取哪个 Skill。
2. 读根目录 `skills/` 下的 Skill 主文件后再决定是否读取 references。
3. 修改代码后运行 `scripts/knowledge-check.sh diff`。
4. 如果代码行为变化影响到文档, 同步更新对应 Skill 和 references。

维护原则:

- Skill 不是普通说明文档, 它要告诉 Agent 在哪里读代码、如何判断影响面、哪些地方不能误判。
- 引用文件只放细节, 不把所有内容堆进 Skill 主文件。
- 发现文档与代码冲突时, 以代码为准并修正文档。
