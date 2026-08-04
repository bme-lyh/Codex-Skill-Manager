# 中文文档

当前 GUI 使用五项一级导航：**首页、资产、安全、活动、设置**。其中资产和活动
提供二级标签页。添加项目统一为 **来源 → 理解与计划 → 检查与确认 → 安装与结果**；
应用会自动理解项目并运行必选的本地分层检查，结论为 **可安装、需要确认、已阻止、
检查未完成** 之一。

按任务阅读：

1. [快速开始](user/getting-started.md)：下载、首次运行和四步添加项目。
2. [GUI 指南](user/gui-guide.md)：五项导航、各页面和完整操作流程。
3. [安全扫描](user/security-scanning.md)：风险等级、分层检查和可选增强项目扫描。
4. [备份、隔离与回滚](user/backups-and-quarantine.md)：恢复方式和跨盘存储。
5. [CLI 参考](user/cli-reference.md)：自动化命令和 JSON 输出。
6. [私有 GitHub 仓库](user/private-repositories.md)：凭据和限流。
7. [故障排查](user/troubleshooting.md)：启动、GitHub、Codex、安装结果和恢复问题。

Codex 审核入口位于添加项目窗口首屏。完成本地检查后，人工一次确认即可进入受控安装；
安装完成后，窗口会重新读取最新状态，并可从 **历史与回滚**
继续恢复。旧 Page 路由和 Wails/API 兼容边界仍保留。

英文文档是项目的主要参考，见 [English documentation](en/README.md)。开发者和 Agent
请从 [Agent 开发入口](agent/AGENT-ENTRYPOINT.md) 开始。
