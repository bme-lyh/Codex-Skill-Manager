# Codex Skill Manager

> **先看来源，再看风险，确认后安装，随时可恢复。**

Codex Skill Manager 是 Windows 本地管理工具，用于检查、安装、更新、分组和恢复
Codex Skills。每次写入都需明确授权，并可追踪、恢复。

[下载 v0.10.1](https://github.com/bme-lyh/Codex-Skill-Manager/releases/tag/v0.10.1) ·
[快速开始](docs/user/getting-started.md) ·
[使用指南](docs/user/gui-guide.md) ·
[安全策略](SECURITY.md) ·
[English](README.md)

## 界面

[![Codex Skill Manager 界面预览](docs/images/ui-carousel.zh-CN.gif)](docs/images/ui-carousel.zh-CN.gif)

图片使用虚构的 Skill、分组和路径，不包含真实账号或个人数据。

## 主要功能

| 任务 | 应用会做什么 |
|---|---|
| 安装 | 从 GitHub 公共/私有仓库或本地目录安装选中的 Skills |
| 评估 | 读取项目说明，发现 Skills，扫描目标，并标明必选、条件和可选检查 |
| 更新 | 比较来源 Commit，显示实际变更，替换前自动备份 |
| 恢复 | 从备份、隔离区或事务记录中恢复 |
| 整理 | 不移动文件即可管理已有 Skills，并支持创建和调整分组 |
| 自动化 | 提供 `csm` CLI 和随发布包附带的 Codex Skill |

## 标准安装和增强项目扫描

所有 GitHub 和本地来源都经过同一流程：

1. **来源**：固定 GitHub Commit，或创建有边界的本地快照。
2. **评估**：阅读项目说明、识别 Codex Skills、扫描实际目标；来源变化或目标不受支持时停止。
3. **复核**：确认文件、风险、权限和恢复方式。
4. **执行**：只写入批准的目标，并记录完整事务。

**标准安装**只复制选中的 Skill 目录。

复杂项目可选用**增强项目扫描**。它读取项目说明，并提出受支持的 Python 工具或
MCP 步骤。该功能需要已登录的 Codex CLI，会消耗 Codex 额度。本地必选评估仍会先运行。

发现 Codex Skill 后，选中的 Skill 默认安装到
`%USERPROFILE%\.codex\skills`；`.system` 始终只读。没有 Codex Skill 的普通项目
不会写入全局 Skills 目录。不受支持的内容保留为人工步骤。MCP 项目目录必须由用户选择。

## 下载和运行

1. 从 [GitHub Releases](https://github.com/bme-lyh/Codex-Skill-Manager/releases/latest)
   下载标准版或便携版 Windows 压缩包。
2. 解压到固定目录。
3. 运行 `CodexSkillManager.exe`。

便携版把数据保存在程序旁边；标准版使用
`%USERPROFILE%\.codex\skill-manager`。当前程序没有 Windows 代码签名，
SmartScreen 可能提示警告。请只从本仓库下载，并核对 SHA-256：

```powershell
Get-FileHash .\CodexSkillManager-0.10.1-windows-amd64.zip -Algorithm SHA256
```

将结果与 `CodexSkillManager-0.10.1-SHA256SUMS.txt` 中的对应行比较。

### 安装随包附带的 Codex Skill

发布包包含 `agent-skill\codex-skill-manager`。如需把它加入全局 Codex Skills，
可在应用中打开 **安装 Skill → 本地目录**，选择发布包内的 `agent-skill` 目录，
完成评估后安装。源码目录对应 `skills\codex-skill-manager`。

## 安全边界

- 不执行仓库脚本或自由形式命令。
- 高风险（High）必须逐簇确认并填写原因；严重风险（Critical）不可忽略。
- 报告级批量忽略只处理已知的中风险（Medium）及以下风险。
- 替换前备份，卸载后进入隔离区；目标被外部修改后，回滚不会强行覆盖。
- 增强项目扫描默认只读；生成计划、批准权限和执行是分开的操作。

“未发现风险”不等于绝对安全。批准陌生来源前，请阅读[安全策略](SECURITY.md)。

## 从源码构建

需要 Go 1.26、Node.js 22 或更高版本、pnpm 11.9、WebView2 和 Wails v2：

```powershell
pnpm --dir frontend install --frozen-lockfile
.\scripts\build.ps1
```

桌面程序和 CLI 输出到 `build\bin`。GUI 必须通过 Wails 构建。

## 文档

- [中文文档](docs/README.md)
- [English documentation](docs/en/README.md)
- [CLI 参考](docs/user/cli-reference.md)
- [开发者与 Agent 文档](docs/agent/AGENT-ENTRYPOINT.md)
- [贡献指南](CONTRIBUTING.md)

Codex Skill Manager 0.10.1 支持 Windows 10/11，使用 [MIT License](LICENSE)。
