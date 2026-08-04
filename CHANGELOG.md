# Changelog

## 0.12.0

### English

- Added a persisted System / Light / Dark appearance setting and aligned shell,
  install, report, form, and sidebar surfaces with semantic light and dark tokens.
- Refined the light theme's sidebar control and Home management banner so the
  icon, heading, description, and update action remain readable on light surfaces.
- Refreshed the English and Chinese showcase assets at a uniform 1440×900 size;
  long screens are represented by scroll-separated frames, including report details.
- Fixed root-scoped update status joins and selected-source checks so every Skill
  receives its latest checked state across `.codex/skills` and `.agents/skills`.
- Added clickable report details with findings, clusters, ignored state, and
  Codex conclusions.
- Accepted valid repository-root `SKILL.md` sources, including the
  `book-to-skill` URL shape, while retaining staging containment and hash checks.
- Made oversized Codex context review more resilient with local manifest counts,
  per-chunk retries, explicit coverage warnings, a review deadline, and chunk
  progress reporting. Large binary-only contexts now retain bounded metadata.
- Added model-specific reasoning tiers, including `max` and `ultra` only when
  the current Codex CLI advertises them; the new default is GPT-5.6 Luna at
  extra-high effort.

### 中文

- 新增“跟随系统 / 浅色 / 深色”外观设置，并统一侧栏、安装流程、报告、表单和深色控件的语义色板。
- 修正浅色模式的侧栏收起按钮和首页管理横幅，保证图标、标题、说明与检查更新按钮清晰可读。
- 重新生成中英文展示图，统一为 1440×900；超出一屏的设置、安全和报告页面按滚动位置拆成多帧。
- 修复按根目录区分的更新状态和选中来源检查，两个 Skills 根目录中的每个 Skill 都能显示最新检查结果。
- 扫描报告支持点击查看风险命中、风险分组、忽略状态和 Codex 复核结论。
- 支持仓库根目录 `SKILL.md`，修复 `book-to-skill` 这类根目录 Skill 的安装失败，同时保留暂存目录、哈希和路径边界校验。
- 增强超大 Codex 上下文复核：本地清单计数、分块重试、覆盖率告警、总超时和分块进度；纯二进制上下文保留有界元数据。
- 推理档位按当前 Codex CLI 的模型能力动态显示，模型支持时提供 `max` 和 `ultra`；新默认模型为 GPT-5.6 Luna，档位为超高。

## 0.11.0

### English

- Added first-class management for both `.codex/skills` and `.agents/skills`,
  with Codex as the default installation target and root-qualified state,
  updates, groups, scans, quarantine, restore, and rollback.
- Reworked the desktop shell around a compact, collapsible sidebar, system
  appearance, system accent color, clearer feedback, and keyboard-visible focus.
- Consolidated Codex review execution behind one bounded runner shared by
  project scans, installation analysis, security review, and update workflows.
- Added local development and verified deployment scripts. Every release build
  refreshes `build/bin`; deployment outside the project remains an explicit step.
- Hardened plan integrity, root-path validation, read-only system boundaries,
  v1 state migration, and cross-root recovery behavior.

### 中文

- 将 `.codex/skills` 与 `.agents/skills` 一并纳入管理；新安装默认选择 Codex 目录，
  状态、更新、分组、扫描、隔离、恢复和回滚均按根目录区分。
- 重做桌面外壳：侧栏可收起，跟随系统明暗与强调色，反馈更直接，键盘焦点更清楚。
- 将 Codex 审核统一为一个有超时、输出上限和结构校验的执行模块，并复用于项目扫描、
  安装分析、安全复核与更新流程。
- 新增本地开发和校验部署脚本；每次发布构建都会刷新 `build/bin`，项目外安装仍需明确执行。
- 加强计划完整性、目录边界、只读系统目录、旧状态迁移和跨根恢复。

## 0.10.2

### English

- Reorganized the sidebar into Home, Assets, Security, Activity, and Settings.
- Added grouped tabs for asset and activity views.
- Reframed installation as a four-step **Add project** flow with secondary actions tucked into **More options**.
- Refreshed the six-frame English and Chinese carousels and aligned the README,
  user guides, troubleshooting, Agent entry point, and release instructions
  with the new workflow.

### 中文

- 将侧栏整理为“首页、资产、安全、活动、设置”五个一级入口，并为资产和活动提供分组标签页。
- 将添加项目入口改为“来源、理解与计划、检查与确认、安装与结果”四步流程；技术选项收进“更多选项”。
- 安装结论统一为可安装、需要确认、已阻止和检查未完成，并保留安装前复核与失败恢复能力。

- 重新截取并生成中英文六帧轮播图，统一更新 README、用户指南、故障排查、Agent 入口和发布说明。

## 0.10.1

### English

- Added separate six-frame English and Chinese interface carousels.
- Kept browser demo data in the selected interface language.
- Simplified installation copy and renamed Codex-assisted analysis to **Enhanced project scan**.
- Limited release packages to the two published carousels and updated safe frontend dependencies.

### 中文

- 分别制作六帧英文和中文界面轮播图。
- 浏览器演示数据跟随界面语言。
- 精简安装文案，并将 Codex 辅助分析统一为“增强项目扫描”。
- 发布包只包含两套正式轮播图，并更新安全范围内的前端依赖。

## 0.10.0

### English

- Unified GitHub and local installation into **Source → Assess → Review → Apply**.
  Every source now passes a persisted local assessment before optional Codex work.
- Added bounded local snapshots, project classification, documentation and
  install-marker review, exact Skill discovery, layered checks, and fail-closed
  validation when a source, digest, target, or assessment changes.
- Added assessment cards, a four-step progress view, clear gate feedback, and
  the CLI `install --plan-id ID --assess` review command.
- Tightened risk decisions: Critical cannot be ignored; High requires individual
  confirmation and a reason; report-wide ignore skips High, Critical, and unknown severities.
- Hardened snapshot path containment and multi-target rollback so files, source
  locks, group layouts, and transaction journals recover together on failure.

### 中文

- GitHub 和本地安装统一为“来源 → 评估 → 复核 → 执行”；所有来源必须先通过
  持久化本地评估，增强项目扫描保持显式可选。
- 新增有边界的本地快照、项目分类、说明与安装线索检查、准确的 Skill 发现和分层门禁；
  来源、摘要、目标或评估变化时失败关闭。
- 新增评估卡片、四步进度、明确的门禁反馈，以及 CLI `--assess` 只读复核命令。
- Critical 不可忽略；High 必须逐簇确认并填写原因；报告级批量忽略会跳过 High、
  Critical 和未知等级。
- 加固路径边界与多目标回滚；失败时统一恢复文件、来源锁、分组布局和事务日志。

## 0.9.0

- 新增可复用的 Codex 项目扫描，采用“本地扫描 → 有界文件摘要 → 重点文件深度分析”的分层流程，统一输出项目概述、安全结论、注意事项和安装方法。
- 为 Codex 上下文加入 800 KiB 单次输入上限、最多 64 个分块、敏感路径隐藏、大文本截断和元数据限额，避免大型 Skills 仓库触发输入长度上限。
- 将计划安装改为明确的授权流程：先只读扫描，用户确认后才制定安装计划，再依据显式权限执行安装；拒绝授权时不会创建计划或写入系统。
- 安全扫描复用项目扫描中的本地发现与安全结论，同时保留覆盖率、隐藏、截断和遗漏统计，避免把部分分析误表示为完整保证。
- CLI 与桌面端统一项目扫描、计划创建、恢复和来源绑定语义；安装计划绑定扫描摘要及有效期，来源或扫描结果变化时拒绝执行。

## 0.8.1

- 修复计划安装的结构化输出 Schema，使所有字段符合严格输出要求，并在调用 CLI 前执行本地 Schema 自检。
- Codex CLI 失败时从 JSONL 中安全提取错误事件，界面显示实际原因，不再只显示 `exit status 1`；正常模型输出和仓库内容不会进入错误诊断。
- 同一仓库存在内容不同的同名 Skill 时，错误会列出冲突路径；识别到 `skills-codex` 镜像后，可在安装窗口一键改用建议的 Codex 子目录。
- 增强项目扫描失败后仍保留来源、风险结果和 Skill 选择，可直接切换到标准安装；复杂配置失败不再阻止基础 Skills 安装。

## 0.8.0

- 新增计划安装：安全打包完整仓库上下文，结构化展示项目概览、安装要求、步骤、权限和实时进度；GUI 与 CLI 均采用先审阅、再执行的两阶段流程。
- Codex 风险复核和安装分析改用“打包上下文、关闭 Shell”的隔离会话，避免不可信仓库内容诱导 CLI 读取工作区外文件或凭据。
- 自动执行仅限经过本地校验的 Skills 安装、官方 PyPI 精确版本 Wheel 隔离环境和受管 Codex MCP 配置；仓库脚本、自由命令及未知操作不会自动运行。
- Python 依赖在分析阶段通过仅允许 PyPI 官方域名的本地代理解析为完整 Wheel 锁，并逐个核对官方元数据中的文件名、下载地址和 SHA-256；源码包与直接链接被拒绝，原生 Wheel 需要独立高风险权限，执行阶段不再联网。
- 必需的人工步骤不再阻止其他安全步骤；应用先完成受支持部分，再以“部分完成”列出待人工处理内容。
- 为计划安装加入仓库上下文哈希、配置指纹、显式权限、父子事务、自动备份、取消、重启恢复和逆序回滚；来源、计划或 Codex MCP 配置在检查点后发生变化时拒绝写入。
- 安装窗口内直接显示 GitHub 403、网络错误和技术详情，并保留输入与分析结果以便重试；新增固定反馈区和清晰的执行时间线。
- 加固 GitHub ZIP 与 Python Wheel 处理，限制文件数量及压缩/解压大小，拦截路径穿越、符号链接、异常归档和来源身份不匹配。
- 超过单次输入上限的仓库会按稳定文件边界分块复核，再按 Skill 统一汇总；每个文本文件只进入一个分块，读取期间的路径逃逸或文件替换会被拒绝。
- 分析进度统一为五个固定阶段，Run ID、序号和完成数保持单调；任务可隐藏到后台，重新打开后恢复，应用退出造成的分析中断会明确提示。
- 限流错误新增恢复倒计时和凭据入口；失败重试前会精确恢复原 Skill 子集、权限与项目目录，并要求重新核对。
- “部分完成”的人工步骤和回滚入口会保留在历史页，待恢复事务不会被最近记录挤出；MCP 配置使用语义 TOML 校验并绑定规范项目目录。

## 0.7.7

- 新增简体中文与 English 界面语言设置，首次运行和旧配置默认使用中文；切换立即生效并自动保存。
- 补齐概览、Skills、分组、更新、安全、历史、隔离、报告、设置及安装/更新对话框的英文界面。
- Codex 辅助复核会跟随界面语言生成进度、总览和每个 Skill 的结论；扫描规则、路径和用户自定义内容保持原文。
- 更新中英文使用文档与界面展示，增加英文设置界面。

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
