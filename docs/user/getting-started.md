# 入门

使用发布包时，下载标准版或便携版压缩包并解压，然后直接运行
`CodexSkillManager.exe`，不需要执行安装脚本。便携版已经包含
`portable.marker`，配置和运行数据默认保存在程序目录的 `data` 子目录中。

如需从源码构建并安装到指定目录，请在仓库根目录运行：

```powershell
pnpm --dir frontend install --frozen-lockfile
.\scripts\build.ps1
.\scripts\install.ps1 -InstallDirectory "D:\Apps\CodexSkillManager"
```

若 Windows 执行策略阻止源码安装脚本，可使用：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\install.ps1 `
  -InstallDirectory "D:\Apps\CodexSkillManager"
```

数据、操作日志、报告、备份、隔离区和暂存区都能在 GUI“设置”页分别改为
绝对路径；保存路径设置后请重启应用。

## 1. 启动

运行 `CodexSkillManager.exe`，程序会读取：

```text
%USERPROFILE%\.codex\skills
```

`.system` 会显示为只读系统分组。

界面首次启动默认为简体中文。如需 English，打开“设置 → 语言”并选择
**English**；界面会立即切换并自动保存。

## 2. 首次管理

当前已知的 CareerForge 和 code-review-graph Skills 可以运行：

```powershell
csm bootstrap
```

管理只记录来源、路径和哈希，不重新下载文件。也可以在 Skills 页选中“未管理”
项目，点击“管理”；程序会自动识别 GitHub 来源并按仓库分组，无法确认远程
来源时则建立独立的本地分组。

## 3. 安装新 Skill

点击“安装 Skill”，选择 GitHub 或本地目录，再粘贴仓库、目录或 `SKILL.md`
链接。两种安装方式都会先：

1. 将 GitHub 版本解析为不可变 Commit，或确认明确的本地目录；
2. 将 GitHub 内容下载到隔离的暂存目录；
3. 发现 Skills；
4. 本地安全扫描；
5. 让你选择单个、多个或全部。

### 标准安装

适合只需要复制 Skills 的普通仓库。核对来源、Skill 列表和风险后，点击安装。
它不会安装额外工具、配置 MCP 或执行仓库脚本。

### Codex 辅助安装

适合还需要 Python 工具、Codex MCP 等集成的复杂仓库。选择该方式就是本次明确
启用；使用前请在“设置”中检查 Codex CLI，并确保它已安装、已登录且有可用额度。

1. 选择“Codex 一键安装”并分析来源；
2. 应用打包完整来源交给 Codex，并关闭 Shell 工具；大仓库会分块分析后统一汇总；
3. 核对计划，选择 Skills，并逐项批准必要权限；
4. 如果计划包含 MCP，选择它要服务的真实 Git 或 SVN 项目目录；
5. 执行后在时间线中查看当前步骤、进度、错误和恢复建议。

应用只执行经过本地验证的类型化步骤：安装 Skills、从官方 PyPI 安装经过校验的
Python 工具、配置由应用管理的 Codex MCP。它不会执行仓库脚本或 Codex 自由生成
的命令；不能安全自动化的步骤会明确标为“手动”。完整来源上下文会由 Codex CLI
处理，因此私有仓库使用者应在启用前确认自己的数据与额度要求。

分析 Python 工具时，应用会从官方 PyPI 把完整 Wheel 依赖下载到隔离暂存区，用来
锁定包名、版本、文件名和 SHA256；此时不会安装或运行它们。源码包会被拒绝，含
本机代码的 Wheel 会标为高风险并单独请求权限。正式执行只使用已核对的暂存文件，
不会再次联网解析依赖。

人工步骤不会阻止其他安全步骤。应用先完成已批准的自动步骤，再以“部分完成”列出
待人工处理内容；如果没有任何可执行步骤，则只展示计划。托管 Python 工具和 MCP
的自动集成需要 GitHub 来源，以便核对 PyPI 包与仓库的归属；本地目录仍可用于
标准安装和 Codex 分析。

来源解析、GitHub 403、Codex 失败和执行错误都会直接显示在“安装 Skill”窗口中。
限流时会显示恢复倒计时，Codex CLI 不可用时可直接打开设置。可以在窗口内重试、
取消、回滚，或保留已经完成的来源分析后切回标准安装。长任务可隐藏到后台；重新
打开时会恢复进度。失败后需要重新核对精确的 Skill 子集、权限和项目目录。“部分
完成”的人工步骤与回滚入口会保留在历史页。

CLI 也支持同一套两阶段辅助安装，详见 [CLI 参考](cli-reference.md#codex-辅助安装)。

## 4. 更新

更新中心首先只检查 Commit，并显示明确状态和上次检查时间。发现新版本后，可以
全选、反选或多选来源分组，再在审查窗口内选择每个来源中的单个或多个 Skills。
程序只扫描本次实际会写入的 Skill 文件；仓库首页、CI、测试等不会造成无关误阻止。
每个来源分别建立备份和事务记录，默认不会定时自动安装。

## 5. 卸载与恢复

卸载会将每个明确选中的 Skill 逐个移动到隔离区。隔离区可恢复，不提供批量永久删除。
