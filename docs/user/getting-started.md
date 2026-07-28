# 入门

程序安装目录通过 `scripts/install.ps1 -InstallDirectory` 指定。数据、操作
日志、报告、备份、隔离区和暂存区都能在 GUI“设置”页分别改为绝对路径；
保存路径设置后请重启应用。

如需便携模式，把 `packaging/portable.marker` 复制到两个可执行文件旁边；
CSM 会把配置和数据默认放在该目录的 `data` 子目录中。

若 Windows 执行策略阻止安装脚本，可使用：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

## 1. 启动

运行 `CodexSkillManager.exe`，程序会读取：

```text
%USERPROFILE%\.codex\skills
```

`.system` 会显示为只读系统分组。

## 2. 首次管理

当前已知的 CareerForge 和 code-review-graph Skills 可以运行：

```powershell
csm bootstrap
```

管理只记录来源、路径和哈希，不重新下载文件。也可以在 Skills 页选中“未管理”
项目，点击“分析并管理”；程序会自动识别 GitHub 来源并按仓库分组，无法确认远程
来源时则建立独立的本地分组。

## 3. 安装新 Skill

点击“安装 Skill”，粘贴 GitHub 仓库、目录或 `SKILL.md` 链接。程序会：

1. 解析仓库和版本；
2. 下载到暂存目录；
3. 发现 Skills；
4. 本地安全扫描；
5. 让用户选择单个、多个或全部；
6. 经确认后安装。

## 4. 更新

更新中心首先只检查 Commit，并显示明确状态和上次检查时间。发现新版本后，可以
全选、反选或多选来源分组，再在审查窗口内选择每个来源中的单个或多个 Skills。
程序只扫描本次实际会写入的 Skill 文件；仓库首页、CI、测试等不会造成无关误阻止。
每个来源分别建立备份和事务记录，默认不会定时自动安装。

## 5. 卸载与恢复

卸载会将每个明确选中的 Skill 逐个移动到隔离区。隔离区可恢复，不提供批量永久删除。
