# Changelog

## 0.7.6

- 精简概览、Skills、分组、更新、安全、历史、报告和设置页面的标题、说明与操作文案，移除营销式口号和重复提示。
- 调整顶部栏和概览区的高度与留白，保持左侧文字品牌区简洁，并替换 Windows 应用图标。
- 使用无隐私的多分组示例数据重新截取关键界面，更新中英文项目首页的“界面展示”轮播图。
- 加固 ZIP 解压路径校验，阻止路径穿越和目标目录外的任意文件写入。

## 0.7.5

- 安全中心的分组、Skill 和 Codex 结论默认折叠，并调整字号、留白和选择工具间距；移除“本地安全基线”中与结果区重复的 Codex 入口。
- Codex 复核状态在切换页面时保持，进度事件使用单调序号过滤迟到更新，解决结果丢失和进度前后跳动。
- 分组复核默认串行执行；失败或缺少 Skill 结论时自动串行重试一次，每次尝试独立计算超时。
- Codex 提示限制为简单的单条只读命令，避免 PowerShell 循环、管道和批量命令触发只读策略拒绝；CLI 诊断长度也受到限制。
- 设置页的“存储位置”与“GitHub 凭据与限额”卡片底部对齐。

## 0.7.4

- Codex 复核改为按应用分组执行：同一分组内选中的 Skills 始终共享一个完整上下文任务，不再按固定数量拆散；不同分组可受控并行。
- 输入 Codex 的本地规则线索改为计数概览，仅保留规则、等级、风险簇 ID、命中数和文件数，不再重复发送证据正文或文件列表。
- 安全中心新增按分组的 Skill 检查队列、推荐选择、全选、反选和清空；已检查且内容未变化的 Skill 默认跳过，未检查或内容变化的 Skill 默认加入。
- 扫描报告和风险提示改为“分组 → Skill → 警告”结构，并保存每个 Skill 的内容指纹与上次检查时间。
- 简化中英文项目首页，将“界面轮播”改为“界面展示”，并补充 Codex CLI、登录和额度消耗说明。

## 0.7.3

- Codex 复核改为按 Skill 输出独立摘要、结论、置信度、关注项、证据文件和关联风险簇，多 Skill 仓库不再只有一段混合总评。
- 新增可配置分批并行调度，默认每批 4 个 Skill、最多 2 批并行；安装和更新只复核当前选中的 Skills，减少无关上下文和等待时间。
- 复核界面新增准备、排队、运行、部分完成和完成状态，实时显示当前 Skill 组、批次进度、完成数量、分析活动数和耗时。
- Codex CLI 复核前改用轻量能力预检，不再重复载入模型目录；生成目录、依赖目录和版本控制元数据不会进入上下文盘点。
- 结构化报告新增 `skillReviews`、`batches`、`totalSkills` 和 `durationMillis`，并保留原风险簇结果以兼容既有报告。
- Release 包中的 Agent Skill 统一位于 `agent-skill/codex-skill-manager`，移除重复副本。

## 0.7.2

- 统一全部风险级别的人工处理：单簇或全量均可一键忽略，原因可选，确定性规则不再要求额外确认；批量决定以单个事务原子写入并可恢复。
- 移除独立的 High 风险接受步骤；High/Critical 仍默认阻止写入，但统一通过人工忽略状态解除。
- 将本地文本规则扫描改为有上限的并行处理，并保持稳定排序和原始规则证据。
- Codex 辅助复核改为在完整目标目录的只读上下文中盘点和读取仓库；本地规则簇仅作为补充线索，并在结果中记录上下文模式与文件数。
- 将界面轮播提升到 1440×900 原生画布，避免把源截图缩小到 1000px。

## 0.7.1

- 移除 Codex 模型选择框下方的英文模型描述和默认推理强度文本，恢复设置页紧凑一致的表单布局。
- 为 Codex CLI 检查与复核设置保存按钮增加独立上间距，避免按钮紧贴推理强度选择框。

## 0.7.0

- Windows 图形界面启动 Codex CLI、GitHub CLI 和计划任务工具时统一隐藏控制台窗口，解决后台状态检查偶发弹出黑框的问题。
- Codex 复核完成后可一键采纳“误报、文档/示例或需人工覆盖”的建议；操作前展示目标，确定性底线要求二次人工确认，每个风险簇均保留独立事务与恢复入口。
- CLI 登录成功后从 `codex debug models` 动态读取当前可见模型，不再维护易过期的硬编码列表；读取失败不会影响既有复核能力。
- Codex 状态增加模型目录及目录读取错误字段，并补充模型过滤和解析测试。

## 0.6.2

- 修复 Codex 风险复核命令中全局审批与沙箱参数的位置，兼容当前 CLI 的 `exec` 语法。
- 不绑定任何 Codex CLI 版本号，改为检查风险复核真正依赖的命令能力；缺少能力时展示具体参数和更新建议。
- 设置页会在首次打开及浏览器授权返回应用后自动刷新 CLI 登录与兼容状态，并显示检查时间。
- 区分“未安装”“尚未登录”和“已登录但能力不兼容”，避免把命令兼容错误误报为登录失败。

## 0.6.1

- 修复 WindowsApps 中不可执行的应用内置 Codex CLI 抢占命令搜索结果的问题；应用现在会逐一验证候选并自动选用独立 npm CLI。
- Codex CLI 自定义路径会在保存后验证其是否真正可执行，并提供更准确的路径或权限错误说明。
- 统一设置页 GitHub 凭据与 Codex 状态操作按钮的字号、尺寸和间距。
- 恢复版本化 release 输出，同时提供标准版与便携版目录。

## 0.6.0

- GitHub 凭据验证、API 限额展示、限流倒计时、短期/ETag 缓存、指定来源重试，以及上一次成功状态保留。
- 后端按规则和文件用途生成风险簇，支持簇级人工决定和确定性底线的审计覆盖。
- 可选 Codex CLI 语义复核，可配置模型和推理强度，使用只读临时会话与结构化输出。
- 危险可执行文件、符号链接和明确破坏行为默认由本地规则阻止；只有人工额外确认可以覆盖。

## 0.5.2

- 安装、更新和安全中心新增五级风险概述，显示每个级别的总命中数、待核查数和处理状态。
- Critical 改为默认阻止但可人工解除：必须逐条核查、填写非空原因并写入本地审计记录；应用前后端都会重新核对尚未忽略的风险。
- 风险明细按严重程度排序，支持分批加载全部唯一发现，并可在安装或更新审查窗口中直接忽略和恢复。
- 重新设计设置页布局，统一输入框与界面字体比例，缩小控件和按钮，并把“保存设置”放入存储位置卡片的紧凑底栏。
- 更新 Windows 应用图标。

## 0.5.1

- 修复 Skills 位于 C 盘、应用备份目录位于 D 盘时，Windows 跨盘移动失败导致的更新中断；备份和隔离内容会自动落到 Skills 根目录内的同盘隐藏目录，事务记录仍保存真实恢复路径。
- GitHub 仓库中存在路径不同但内容完全相同的同名 Skill 时，自动去重并优先选择规范路径；若同名内容不同则明确阻止并要求指定路径。
- 扫描限额错误改为报告所选 Skills 的总文件数和配置总上限，不再显示容易误解的“剩余文件数”。
- 扫描提前失败时也会正确汇总风险级别和完成状态。
- 大型扫描报告在界面中按稳定指纹去重并限制首屏渲染数量，完整原始发现仍保存在本地报告中。

## 0.5.0

- 更新中心新增来源多选、全选、反选、清空和跨来源批量更新。
- 更新计划只扫描实际会写入的 Skill 目录，不再让仓库级文档、测试或工作流误阻止更新。
- 更新审查窗口显示风险文件、命中内容、原因和处理建议。
- 移除 GUI 左上角 Logo，改用清晰的文字品牌区，并统一优化卡片、弹窗和交互反馈。
- 移除分组页冗余关系图，保留更直接的分组与 Skill 拖动管理界面。
- 完善中英双语 GitHub 首页、用户文档、贡献指南和自动化检查配置。

## 0.4.0

- Redesigned the sidebar brand lockup for a cleaner application identity.
- Replaced raw update Commit badges with persistent, reader-friendly states,
  timestamps, remote Commit details, and per-Skill update availability.
- Added two-phase interactive updates in the GUI with per-Skill selection,
  immutable Commit resolution, security review, backup, and rollback.
- Added per-Skill installed Commit tracking so partial group updates remain
  accurate.
- Added GUI access to single-Skill scanning, environment diagnostics, version
  information, known-Skill bootstrap management, and local GitHub credentials.
- Added automated coverage confirming GitHub installs create source groups.

## 0.3.0

- Updated the application and Windows executable logo.
- Replaced user-facing “纳管” terminology with clearer “管理” wording.
- Added Select All, Invert Selection, and Clear Selection to the Skills page.
- Removed decorative Skill initials from the table and added complete hover/
  keyboard-focus details for every information column.
- Added source detection with confidence and evidence during management of
  existing Skills, with automatic source-based grouping and isolated local
  fallback groups.
- Added manual group creation, rename, drag ordering, and drag-to-move Skills.
- Kept source provenance independent from user layout and made every group
  mutation journaled, backed up, and rollback-capable.

## 0.2.0

- Added a two-stage GUI and CLI workflow to analyze and adopt existing
  unmanaged Skills without moving or rewriting their content.
- Changed the security badge to count unique active high/critical findings
  from the latest scan per target instead of counting historical reports.
- Added persistent per-finding ignore/restore decisions, clearer risk titles,
  matched evidence, confidence, recommendations, and ignore reasons.
- Added consistent running, success, and failure feedback for refresh, update
  checks, scans, installs, adoption, quarantine, restore, rollback, settings,
  and scheduled-check configuration.

## 0.1.2

- Adopted the user-provided Codex Skill Manager logo for the Windows executable
  and the desktop sidebar brand.

## 0.1.1

- Fixed the first-launch white window caused by empty Go slices being encoded
  as JSON `null`.
- Normalized dashboard, scan and quarantine collections in both backend and
  frontend contracts.
- Added a visible frontend error boundary instead of leaving an empty window.
- Added regression coverage for clean first-run state.

## 0.1.0

- Initial Windows desktop and CLI implementation.
- GitHub URL and local-directory installation plans.
- Public/private GitHub authentication resolution.
- Deterministic source grouping and relationship visualization.
- Built-in local security scanner.
- SQLite transaction and scan history.
- Automatic backups, quarantine removal, restore and rollback.
- Optional scheduled update checks.
- User and Agent documentation.
