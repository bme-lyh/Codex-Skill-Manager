# CLI 参考

所有命令支持：

```text
--config <绝对路径>
--json
```

## 只读命令

```powershell
csm discover
csm dashboard
csm audit [--skill NAME]
csm check
csm history
csm reports
csm doctor
```

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
省略时复核报告目标中识别到的全部 Skills。结果按 Skill 分开，并包含批次状态和耗时。
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

## 应用计划

```powershell
csm install --plan-id PLAN `
  --skill skill-a `
  --skill skill-b `
  --apply
```

High/Critical 风险默认阻止写入。`--accept-high-risk` 仅为旧脚本保留，不再单独放行
High 风险；请使用统一的 `warning` 人工决定。原因可选。

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

`--reason` 可选。所有决定都会进入本地事务日志。

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

所有级别和确定性规则使用同一命令；`--confirm-deterministic` 仅为旧脚本兼容保留。
`--dry-run` 会返回明确的风险簇目标而不修改状态。
