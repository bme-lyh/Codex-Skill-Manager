# 参与贡献

感谢你帮助改进 Codex Skill Manager。安全边界和可恢复性优先于功能数量。

## 开始之前

1. 阅读 [AGENTS.md](AGENTS.md) 和 [安全策略](SECURITY.md)。
2. 对较大功能先创建 Issue，说明使用场景、UI/CLI 契约和恢复路径。
3. 从最新主分支创建短生命周期分支。

## 开发要求

- GUI 与 CLI 必须调用同一套 Go Manager API，不在 React 中复制写入逻辑。
- 不执行 Skill 仓库中的脚本，不修改 `.codex/skills/.system`。
- 不引入递归、通配符、多目标或永久批量删除。
- 分支或标签必须在计划阶段解析为不可变 Commit。
- 新增写入功能必须具备：预览/计划、明确目标、备份或隔离、事务日志、结构化 JSON、失败报告和恢复路径。
- 修改命令契约或数据结构时同步更新用户及 Agent 文档。

## 本地检查

```powershell
gofmt -w .
go test ./...
pnpm --dir frontend install
pnpm --dir frontend run build
.\scripts\build.ps1
```

桌面程序必须使用 Wails 构建，不能用普通 `go build` 代替。修改 `skills/codex-skill-manager` 后还需运行仓库内说明的 Skill 校验器。

## Pull Request

- 保持改动聚焦，说明用户可见变化和安全影响。
- 添加覆盖成功、拒绝、失败与恢复路径的测试。
- 更新 `CHANGELOG.md`。
- 不提交 Token、Cookie、私有 Skill 内容、运行数据、构建产物或本地工具链。

## English

Contributions are welcome. Please preserve the invariants in `AGENTS.md`, add tests for success and recovery paths, run Go/frontend/Wails validation, update affected documentation and `CHANGELOG.md`, and keep each pull request focused. Every mutation must remain planned, explicit, backed up or quarantined, journaled, machine-readable, and recoverable.
