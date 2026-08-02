# CLI 参考

用法：

```text
csm [--config 绝对路径] [--json] 命令
```

全局选项可以写在命令前后。`--json` 会返回统一结构，其中包含
`schemaVersion`、`command`、`status`，以及可选的 `data` 和 `error`；
`csm --json version` 也遵循这一结构。直接运行 `csm` 会显示内置命令列表并返回
退出码 2；当前没有单独的 `help` 命令。

## 只读命令

```powershell
csm discover
csm dashboard
csm audit [--skill NAME]
csm check [--group GROUP_ID ...] [--force]
csm history
csm reports
csm doctor
csm version
```

不指定 `--skill` 时，`audit` 会检查全部非系统 Skills，并按应用中的分组记录结果；指定后只检查该 Skill。

`check` 返回每个来源的结构化状态、上次检查时间、远端 Commit 和待更新 Skills。
可重复使用 `--group ID` 只检查指定来源，使用 `--force` 绕过五分钟短期缓存。
限流或临时错误不会清除上一次成功状态。

```powershell
csm github-auth
csm codex status
csm codex review --report SCAN_ID [--skill NAME ...]
```

`github-auth` 验证凭据并显示 REST API 剩余额度。`codex status` 检查独立 CLI、登录
状态和复核能力兼容性；登录成功时还会返回当前 CLI 的可见模型目录。`codex review`
对指定扫描报告执行可选只读语义复核；可重复使用 `--skill` 只复核明确选定的 Skills，
省略时复核报告目标中识别到的全部 Skills。结果按 Skill 分开，并包含分组任务状态和耗时。
兼容性依据实际命令能力判断，不绑定 CLI 版本号。
模型目录读取失败属于非致命状态，不会把本来兼容的复核功能判定为不可用。该功能必须先在配置中启用
`codexReview.enabled`，默认模型 `default` 表示 Codex 当前默认先进模型，默认推理强度
为 `medium`。
CLI 更新使用两阶段计划：

```powershell
csm update --group "github:owner/repository"
csm install --plan-id PLAN --skill skill-a --apply
```

第一步解析远端不可变 Commit，并且只扫描该来源中已安装的 Skills；第二步显式选择
目标并应用。GUI 在更新中心额外提供跨来源多选与批量编排。

## 管理已有 Skills

```powershell
csm bootstrap
csm manage --skill existing-a --skill existing-b
csm manage --plan-id MANAGE_PLAN --skill existing-a --apply
```

`manage` 第一条命令只生成来源识别和安全分析计划；第二条命令显式应用计划。
管理只建立本地快照和来源锁，不移动现有文件。旧的 `adopt` 名称仅为兼容保留。

## 管理分组

```powershell
csm group create --name "常用工具"
csm group rename --id GROUP_ID --name "开发工具"
csm group reorder --id GROUP_A --id GROUP_B
csm group move --group GROUP_ID --skill skill-a --skill skill-b
```

分组操作只修改本地布局，不改变真实来源；它们都会写入事务日志并支持回滚。

## GitHub 安装计划

```powershell
csm install --url "https://github.com/owner/repo"
csm install --url "https://github.com/owner/repo/tree/main/skills/example"
csm install --url "https://github.com/owner/repo" --ref v1.2.0
```

## 本地安装计划

```powershell
csm install --local "D:\skills\package"
```

创建计划后，先读取并核对后端保存的分层评估，再决定是否应用：

```powershell
csm --json install --plan-id PLAN --assess
```

不带 `--assist` 时，上述命令始终使用标准的两阶段 Skill 安装，不安装额外工具，
也不修改 Codex MCP。

## Codex 辅助安装

CLI 使用三个明确阶段。第一步只进行只读项目扫描，返回项目概述、安全结论、证据覆盖
和声明式安装方式：

```powershell
csm --json install --url "https://github.com/owner/repo" --assist
```

核对返回的 `id`、`security`、`installationMethods` 和上下文覆盖信息。确认继续后，
第二步明确授权 Codex 生成结构化安装计划：

```powershell
csm --json install --assist --project-scan-id PROJECT_SCAN --create-plan
```

再核对计划中的 `id`、`skills`、`steps`、`permissions`、`warnings` 和
`needsProjectRoot`。第三步明确选择 Skills，并逐个传入计划中要批准的权限 ID：

```powershell
csm --json install --assist --plan-id ASSISTED_PLAN --apply `
  --skill skill-a `
  --grant PERMISSION_ID `
  --project-root "D:\work\my-project"
```

只传计划实际列出的权限；`--all` 可代替多个 `--skill`。只有计划要求 MCP 时才传
`--project-root`，它必须是真实的 Git 或 SVN 工作目录。项目扫描阶段不会下载依赖
或生成安装计划；不能在创建辅助计划时同时使用 `--apply`。

输入 `--assist` 本身就是本次明确启用，不受安全中心“Codex 风险复核”总开关
控制；它仍使用设置中的 CLI 路径、模型和推理强度，并消耗 Codex 额度。CLI 返回
最终结构化计划或结果；桌面界面另外提供实时进度、取消和恢复展示。内部计划快照
不是稳定的外部文件接口，应始终通过命令返回的 ID 继续操作。

## 应用计划

```powershell
csm install --plan-id PLAN `
  --skill skill-a `
  --skill skill-b `
  --apply
```

High/Critical 风险默认阻止写入。Critical 不可忽略；High 必须通过 `warning`
逐簇提供 `--reason` 并使用 `--confirm-deterministic` 明确确认。执行含已接受 High
风险的计划时还需 `--accept-high-risk` 作最终确认；它不能创建决定或单独放行风险。

## 隔离卸载

```powershell
csm remove skill-a
csm remove skill-a skill-b
```

不接受通配符，也不支持 `--all`。

## 恢复和回滚

```powershell
csm restore --skill skill-a --transaction TX
csm rollback --transaction TX
```

## 定时检查

```powershell
csm schedule --enabled=true --frequency=weekly --at=09:00
```

## 忽略或恢复安全警告

从 `csm reports --json` 取得 finding 的 `fingerprint`、`ruleId` 和 `file` 后：

```powershell
csm warning --fingerprint HASH --rule CSM-INJ-001 --file "skill/SKILL.md"
csm warning --fingerprint HASH --rule CSM-INJ-001 --file "skill/SKILL.md" --restore
```

Medium 及以下的 `--reason` 可选。High 必须填写非空原因并逐簇明确确认；Critical
不可忽略。所有决定都会进入本地事务日志。

风险中心推荐按簇操作：

```powershell
csm warning --cluster RISK_ID --rule CSM-NET-001 --file-class documentation `
  --fingerprint HASH1 --fingerprint HASH2
```

一次预览或处理报告中的全部匹配风险簇：

```powershell
csm warning --report SCAN_ID --dry-run
csm warning --report SCAN_ID
csm warning --report SCAN_ID --restore
```

报告级批量操作只处理 Medium 及以下风险。High 必须逐簇使用
`--confirm-deterministic` 和非空 `--reason`，Critical 不可忽略；执行含已接受 High
风险的标准计划还需 `--accept-high-risk` 作最终确认。
`--dry-run` 会返回明确的风险簇目标而不修改状态。

## 输出与退出码

- `0`：成功。
- `1`：运行失败或安全策略阻止。
- `2`：命令、参数或必填选项无效。

无效参数会在对应操作执行前结束。JSON 模式仍会返回统一错误结构，不会混入
Flag 解析器的额外文本。
