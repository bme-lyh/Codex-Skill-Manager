import type {
  AssistedInstallPlan,
  AssistedInstallProgress,
  AssistedInstallResult,
  CodexCLIStatus,
  Dashboard,
  Finding,
  InstallPreview,
  RiskCluster,
  ScanReport,
  Transaction
} from "./types";

// Anonymous data for browser-only UI previews. The packaged Wails application
// always receives its dashboard, paths, credentials, and reports from Go.
const makeFinding = (
  ruleId: string,
  title: string,
  severity: Finding["severity"],
  file: string,
  line: number,
  category: string,
  clusterId: string,
  evidence: string,
  ignored = false
): Finding => ({
  ruleId,
  title,
  severity,
  confidence: 0.91,
  file,
  line,
  evidence,
  explanation: "该内容可能影响本机环境，需要结合 Skill 的实际用途进行人工判断。",
  recommendedAction: "检查目标、参数和触发条件，确认符合预期后再决定是否忽略。",
  fingerprint: `demo-${ruleId}-${file}-${line}`,
  ignored,
  ignoreReason: ignored ? "匿名演示：已核对为预期的本地构建步骤" : undefined,
  fileClass: file.endsWith(".md") ? "instruction" : "runtime",
  category,
  clusterId,
  deterministic: false
});

const networkFindings = [
  makeFinding(
    "network-access",
    "网络访问与外部请求",
    "medium",
    "scripts/release.ps1",
    28,
    "network",
    "cluster-network",
    "Invoke-RestMethod -Uri $releaseEndpoint"
  ),
  makeFinding(
    "network-access",
    "网络访问与外部请求",
    "medium",
    "SKILL.md",
    64,
    "network",
    "cluster-network",
    "上传发布产物前需要访问 GitHub Releases API"
  )
];

const shellFindings = [
  makeFinding(
    "shell-command",
    "Shell 命令调用",
    "low",
    "SKILL.md",
    41,
    "execution",
    "cluster-shell",
    "运行项目已有的测试与构建命令",
    true
  ),
  makeFinding(
    "shell-command",
    "Shell 命令调用",
    "low",
    "scripts/release.ps1",
    12,
    "execution",
    "cluster-shell",
    "调用 go test 和前端构建流程",
    true
  )
];

export const demoRiskClusters: RiskCluster[] = [
  {
    id: "cluster-network",
    ruleId: "network-access",
    title: "网络访问与外部请求",
    severity: "medium",
    category: "network",
    fileClass: "runtime",
    deterministic: false,
    findingCount: networkFindings.length,
    affectedFiles: ["scripts/release.ps1", "SKILL.md"],
    fingerprints: networkFindings.map(finding => finding.fingerprint),
    sampleFindings: networkFindings,
    ignored: false,
    skillName: "release-helper",
    groupId: "development",
    groupName: "开发工具"
  },
  {
    id: "cluster-shell",
    ruleId: "shell-command",
    title: "Shell 命令调用",
    severity: "low",
    category: "execution",
    fileClass: "instruction",
    deterministic: false,
    findingCount: shellFindings.length,
    affectedFiles: ["SKILL.md", "scripts/release.ps1"],
    fingerprints: shellFindings.map(finding => finding.fingerprint),
    sampleFindings: shellFindings,
    ignored: true,
    ignoreReason: "匿名演示：已核对为预期的本地构建步骤",
    skillName: "release-helper",
    groupId: "development",
    groupName: "开发工具"
  }
];

export const demoScanReport: ScanReport = {
  id: "demo-scan-release-helper",
  target: "release-helper",
  highestSeverity: "medium",
  activeHighestSeverity: "medium",
  findings: [...networkFindings, ...shellFindings],
  filesScanned: 42,
  activeFindingCount: 1,
  ignoredFindingCount: 1,
  status: "completed",
  completedAt: "2026-07-28T10:22:00Z",
  clusters: demoRiskClusters,
  skills: [{
    skillName: "release-helper",
    sourcePath: "release-helper",
    groupId: "development",
    groupName: "开发工具",
    filesScanned: 42,
    highestSeverity: "medium",
    activeFindingCount: 1,
    ignoredFindingCount: 1
  }],
  codexReview: {
    id: "demo-codex-review",
    status: "completed",
    summary: "命中主要来自发布流程示例。未发现明确破坏行为，建议重点复核网络目标和命令参数。",
    overallVerdict: "人工复核后可接受",
    model: "gpt-5.6-luna",
    reasoningEffort: "xhigh",
    contextMode: "full-target-packaged-no-tools",
    contextFileCount: 128,
    startedAt: "2026-07-28T10:21:00Z",
    completedAt: "2026-07-28T10:22:00Z",
    totalSkills: 1,
    durationMillis: 60000,
    batches: [{
      index: 1,
      groupId: "development",
      groupName: "开发工具",
      status: "completed",
      skillNames: ["release-helper"],
      startedAt: "2026-07-28T10:21:00Z",
      completedAt: "2026-07-28T10:22:00Z"
    }],
    skillReviews: [{
      skillName: "release-helper",
      sourcePath: ".",
      status: "completed",
      verdict: "review-required",
      summary: "发布操作与 Skill 目标一致，但应人工确认网络目标和命令参数均保持在预期范围内。",
      confidence: 0.9,
      contextFileCount: 42,
      clusterIds: ["cluster-network", "cluster-shell"],
      concerns: [{
        title: "发布阶段包含网络请求",
        severity: "medium",
        confidence: 0.87,
        evidenceFiles: ["SKILL.md", "scripts/release.ps1"],
        rationale: "网络行为符合发布用途，但目标地址和上传范围需要人工确认。",
        recommendation: "仅允许访问预期的 GitHub API 地址。"
      }],
      clusterReviews: []
    }],
    reviews: [
      {
        clusterId: "cluster-network",
        verdict: "review",
        effectiveSeverity: "medium",
        confidence: 0.87,
        rationale: "请求目标与发布功能一致，但仍应核对域名和上传范围。",
        recommendation: "确认仅访问预期的 GitHub API 地址。"
      },
      {
        clusterId: "cluster-shell",
        verdict: "acceptable",
        effectiveSeverity: "low",
        confidence: 0.94,
        rationale: "命令用于仓库已有的测试和构建流程。",
        recommendation: "保留当前人工忽略记录。"
      }
    ]
  }
};

const demoHistory: Transaction[] = [
  {
    id: "demo-tx-assisted-install",
    type: "assisted-install",
    status: "partial",
    targets: ["build-graph", "debug-issue", "explore-codebase", "review-pr"],
    projectRoot: "D:\\projects\\demo-codebase",
    recoveryStatus: "available",
    startedAt: "2026-07-29T10:14:00Z",
    completedAt: "2026-07-29T10:14:35Z",
    steps: [
      {
        id: "install-skills",
        kind: "install-skills",
        title: "安装 Skills",
        description: "安装选中的 Skills",
        status: "completed",
        required: true,
        supported: true,
        reversible: true
      },
      {
        id: "install-managed-tool",
        kind: "managed-python-tool",
        title: "安装受管代码关系工具",
        description: "安装已锁定并校验的 Wheel",
        status: "completed",
        required: true,
        supported: true,
        reversible: true
      },
      {
        id: "configure-mcp",
        kind: "configure-codex-mcp",
        title: "配置 Codex MCP",
        description: "备份后写入受管 MCP 配置",
        status: "completed",
        required: true,
        supported: true,
        reversible: true
      },
      {
        id: "initialize-project-index",
        kind: "manual",
        title: "首次建立项目索引",
        description: "按项目范围手动建立索引",
        status: "manual-pending",
        required: false,
        supported: false,
        reversible: false
      }
    ]
  },
  {
    id: "demo-tx-install",
    type: "install",
    status: "completed",
    targets: ["paper-review", "citation-checker"],
    startedAt: "2026-07-28T09:30:00Z",
    completedAt: "2026-07-28T09:31:00Z"
  },
  {
    id: "demo-tx-update",
    type: "update",
    status: "completed",
    targets: ["code-audit", "release-helper"],
    startedAt: "2026-07-27T12:00:00Z",
    completedAt: "2026-07-27T12:02:00Z"
  },
  {
    id: "demo-tx-group",
    type: "group-layout",
    status: "completed",
    targets: ["研究工作流", "开发工具", "内容效率"],
    startedAt: "2026-07-26T08:10:00Z",
    completedAt: "2026-07-26T08:10:30Z"
  },
  {
    id: "demo-tx-quarantine",
    type: "quarantine",
    status: "completed",
    targets: ["legacy-helper"],
    startedAt: "2026-07-25T15:20:00Z",
    completedAt: "2026-07-25T15:20:20Z"
  }
];

export const demoDashboard: Dashboard = {
  skills: [
    {
      name: "research-assistant",
      description: "学术检索、证据整理与引用检查",
      path: "C:\\Users\\demo\\.codex\\skills\\research-assistant",
      groupId: "research",
      groupName: "研究工作流",
      sourceGroupId: "research",
      sourceGroupName: "研究工作流",
      sourceProvider: "github",
      sourceConfidence: 0.98,
      sourceEvidence: "GitHub 来源与锁定 Commit",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "有新版本",
      installedCommit: "7a6c9d1",
      sourceRepository: "example/research-skills",
      sourcePath: "skills/research-assistant"
    },
    {
      name: "paper-review",
      description: "论文结构、方法和结论审阅",
      path: "C:\\Users\\demo\\.codex\\skills\\paper-review",
      groupId: "research",
      groupName: "研究工作流",
      sourceGroupId: "research",
      sourceGroupName: "研究工作流",
      sourceProvider: "github",
      sourceConfidence: 0.97,
      sourceEvidence: "同仓库 Skill 目录",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "最新",
      installedCommit: "7a6c9d1",
      sourceRepository: "example/research-skills",
      sourcePath: "skills/paper-review"
    },
    {
      name: "citation-checker",
      description: "核对引用格式、DOI 与参考文献完整性",
      path: "C:\\Users\\demo\\.codex\\skills\\citation-checker",
      groupId: "research",
      groupName: "研究工作流",
      sourceGroupId: "research",
      sourceGroupName: "研究工作流",
      sourceProvider: "github",
      sourceConfidence: 0.96,
      sourceEvidence: "同仓库 Skill 目录",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "有新版本",
      installedCommit: "7a6c9d1",
      sourceRepository: "example/research-skills",
      sourcePath: "skills/citation-checker"
    },
    {
      name: "code-audit",
      description: "代码变更影响与安全复核",
      path: "C:\\Users\\demo\\.codex\\skills\\code-audit",
      groupId: "development",
      groupName: "开发工具",
      sourceGroupId: "development",
      sourceGroupName: "开发工具",
      sourceProvider: "github",
      sourceConfidence: 0.99,
      sourceEvidence: "GitHub 来源与锁定 Commit",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "最新",
      installedCommit: "88b2f30",
      sourceRepository: "example/developer-skills",
      sourcePath: "code-audit"
    },
    {
      name: "release-helper",
      description: "生成发布说明并核对交付清单",
      path: "C:\\Users\\demo\\.codex\\skills\\release-helper",
      groupId: "development",
      groupName: "开发工具",
      sourceGroupId: "development",
      sourceGroupName: "开发工具",
      sourceProvider: "github",
      sourceConfidence: 0.98,
      sourceEvidence: "GitHub 来源与锁定 Commit",
      managed: true,
      system: false,
      localModified: true,
      securityStatus: "2 个待核查",
      updateStatus: "最新",
      installedCommit: "88b2f30",
      sourceRepository: "example/developer-skills",
      sourcePath: "release-helper"
    },
    {
      name: "test-planner",
      description: "根据变更范围生成分层测试计划",
      path: "C:\\Users\\demo\\.codex\\skills\\test-planner",
      groupId: "development",
      groupName: "开发工具",
      sourceGroupId: "development",
      sourceGroupName: "开发工具",
      sourceProvider: "github",
      sourceConfidence: 0.95,
      sourceEvidence: "同仓库 Skill 目录",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "最新",
      installedCommit: "88b2f30",
      sourceRepository: "example/developer-skills",
      sourcePath: "test-planner"
    },
    {
      name: "meeting-notes",
      description: "把会议记录整理成决策、行动项和负责人",
      path: "D:\\Skills\\content-suite\\meeting-notes",
      groupId: "productivity",
      groupName: "内容效率",
      sourceGroupId: "productivity",
      sourceGroupName: "内容效率",
      sourceProvider: "local",
      sourceConfidence: 0.93,
      sourceEvidence: "本地目录快照",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "本地来源"
    },
    {
      name: "document-polisher",
      description: "优化中文文档结构、表达和可读性",
      path: "D:\\Skills\\content-suite\\document-polisher",
      groupId: "productivity",
      groupName: "内容效率",
      sourceGroupId: "productivity",
      sourceGroupName: "内容效率",
      sourceProvider: "local",
      sourceConfidence: 0.92,
      sourceEvidence: "本地目录快照",
      managed: true,
      system: false,
      localModified: false,
      securityStatus: "安全",
      updateStatus: "本地来源"
    },
    {
      name: "diagram-helper",
      description: "根据说明生成结构化流程图",
      path: "D:\\Skills\\content-suite\\diagram-helper",
      groupId: "productivity",
      groupName: "内容效率",
      sourceGroupId: "productivity",
      sourceGroupName: "内容效率",
      sourceProvider: "local",
      sourceConfidence: 0.74,
      sourceEvidence: "检测到本地目录，尚未确认来源",
      managed: false,
      system: false,
      localModified: false,
      securityStatus: "未扫描",
      updateStatus: "未管理"
    },
    {
      name: "skill-creator",
      description: "Codex 内置 Skill 创建工具",
      path: "C:\\Users\\demo\\.codex\\skills\\.system\\skill-creator",
      groupId: "system",
      groupName: "系统 Skills",
      sourceGroupId: "system",
      sourceGroupName: "系统 Skills",
      sourceProvider: "system",
      sourceConfidence: 1,
      sourceEvidence: "Codex 系统目录",
      managed: false,
      system: true,
      localModified: false,
      securityStatus: "系统维护",
      updateStatus: "系统维护"
    }
  ],
  groups: [
    {
      id: "research",
      name: "研究工作流",
      provider: "github",
      repository: "example/research-skills",
      readOnly: false,
      manual: false,
      position: 1,
      skillNames: ["research-assistant", "paper-review", "citation-checker"],
      status: "managed"
    },
    {
      id: "development",
      name: "开发工具",
      provider: "github",
      repository: "example/developer-skills",
      readOnly: false,
      manual: false,
      position: 2,
      skillNames: ["code-audit", "release-helper", "test-planner"],
      status: "managed"
    },
    {
      id: "productivity",
      name: "内容效率",
      provider: "local",
      repository: "D:\\Skills\\content-suite",
      readOnly: false,
      manual: true,
      position: 3,
      skillNames: ["meeting-notes", "document-polisher", "diagram-helper"],
      status: "managed"
    }
  ],
  sourceGroups: [],
  relations: [
    { from: "research-assistant", to: "paper-review", type: "workflow", confidence: 0.94, evidence: "研究流程" },
    { from: "paper-review", to: "citation-checker", type: "workflow", confidence: 0.91, evidence: "引用核查" },
    { from: "code-audit", to: "test-planner", type: "related", confidence: 0.88, evidence: "测试范围" },
    { from: "release-helper", to: "code-audit", type: "related", confidence: 0.82, evidence: "发布复核" },
    { from: "meeting-notes", to: "document-polisher", type: "related", confidence: 0.79, evidence: "文档整理" }
  ],
  recentReports: [demoScanReport],
  recentHistory: demoHistory,
  updateStatuses: [
    {
      groupId: "research",
      groupName: "研究工作流",
      provider: "github",
      repository: "example/research-skills",
      status: "update-available",
      currentCommits: {
        "research-assistant": "7a6c9d1",
        "paper-review": "7a6c9d1",
        "citation-checker": "7a6c9d1"
      },
      remoteCommit: "be72d09",
      outdatedSkills: ["research-assistant", "citation-checker"],
      checkedAt: "2026-07-28T11:00:00Z",
      lastSuccessStatus: "update-available",
      lastSuccessAt: "2026-07-28T11:00:00Z",
      lastSuccessRemoteCommit: "be72d09",
      rateLimitRemaining: 4870,
      rateLimitLimit: 5000,
      fromCache: false
    },
    {
      groupId: "development",
      groupName: "开发工具",
      provider: "github",
      repository: "example/developer-skills",
      status: "up-to-date",
      currentCommits: {
        "code-audit": "88b2f30",
        "release-helper": "88b2f30",
        "test-planner": "88b2f30"
      },
      remoteCommit: "88b2f30",
      outdatedSkills: [],
      checkedAt: "2026-07-28T11:00:00Z",
      lastSuccessStatus: "up-to-date",
      lastSuccessAt: "2026-07-28T11:00:00Z",
      lastSuccessRemoteCommit: "88b2f30",
      rateLimitRemaining: 4870,
      rateLimitLimit: 5000,
      fromCache: false
    },
    {
      groupId: "productivity",
      groupName: "内容效率",
      provider: "local",
      repository: "D:\\Skills\\content-suite",
      status: "unsupported",
      outdatedSkills: [],
      checkedAt: "2026-07-28T11:00:00Z",
      fromCache: false
    }
  ],
  lastUpdateCheck: "2026-07-28T11:00:00Z",
  managedCount: 8,
  unmanagedCount: 1,
  systemCount: 1,
  riskCount: 2,
  updateCount: 2
};

export const demoConfig = {
  locale: "zh-CN",
  defaultRootId: "codex-default",
  roots: [
    { rootId: "codex-default", rootKind: "codex", rootName: "Codex Skills", path: "C:\\Users\\demo\\.codex\\skills", enabled: true },
    { rootId: "agents", rootKind: "agents", rootName: "Agents Skills", path: "C:\\Users\\demo\\.agents\\skills", enabled: true }
  ],
  paths: {
    skillsRoot: "C:\\Users\\demo\\.codex\\skills",
    dataRoot: "D:\\CodexSkillManager\\data",
    logsRoot: "D:\\CodexSkillManager\\data\\logs",
    reportsRoot: "D:\\CodexSkillManager\\data\\reports",
    backupsRoot: "D:\\CodexSkillManager\\data\\backups",
    quarantineRoot: "D:\\CodexSkillManager\\data\\quarantine",
    cacheRoot: "D:\\CodexSkillManager\\data\\cache",
    stagingRoot: "D:\\CodexSkillManager\\data\\staging"
  },
  schedule: { enabled: true, frequency: "weekly", time: "09:00" },
  codexReview: {
    enabled: true,
    cliPath: "",
    model: "gpt-5.6-luna",
    reasoningEffort: "xhigh",
    timeoutSeconds: 300,
    maxSamplePerRisk: 8,
    maxParallelBatches: 1
  }
};

export const demoCodexStatus: CodexCLIStatus = {
  available: true,
  authenticated: true,
  compatible: true,
  path: "codex",
  version: "codex-cli · 演示状态",
  authStatus: "authenticated",
  checkedAt: "2026-07-28T11:05:00Z",
  models: [
    {
      slug: "gpt-5.6-luna",
      displayName: "GPT-5.6 Luna",
      description: "Balanced frontier model for deep repository reviews.",
      defaultReasoningLevel: "medium",
      reasoningLevels: [
        { effort: "low", description: "Fast review" },
        { effort: "medium", description: "Balanced speed and depth" },
        { effort: "high", description: "Deeper semantic review" },
        { effort: "xhigh", description: "Most complete standard review" },
        { effort: "max", description: "Highest available review effort" }
      ]
    }
  ]
};

const demoInstallSkills = [
  {
    name: "build-graph",
    description: "构建或更新代码关系图",
    sourcePath: "skills/build-graph"
  },
  {
    name: "debug-issue",
    description: "结合代码关系定位问题原因",
    sourcePath: "skills/debug-issue"
  },
  {
    name: "explore-codebase",
    description: "使用结构关系理解代码库",
    sourcePath: "skills/explore-codebase"
  },
  {
    name: "review-pr",
    description: "结合影响范围复核 Pull Request",
    sourcePath: "skills/review-pr"
  }
];

export const demoInstallPreview: InstallPreview = {
  id: "demo-install-plan",
  repository: {
    provider: "github",
    fullName: "example/code-review-skills",
    private: false,
    defaultBranch: "main",
    license: "MIT",
    resolvedRef: "main",
    commitSha: "8c2b57d37b8d96f47e5a4d41cc4d2f117c8296a1"
  },
  skills: demoInstallSkills,
  scan: {
    id: "demo-install-scan",
    target: "example/code-review-skills",
    highestSeverity: "informational",
    activeHighestSeverity: "informational",
    findings: [],
    filesScanned: 186,
    activeFindingCount: 0,
    ignoredFindingCount: 0,
    status: "passed",
    completedAt: "2026-07-28T11:10:00Z",
    clusters: [],
    skills: demoInstallSkills.map(skill => ({
      skillName: skill.name,
      sourcePath: skill.sourcePath,
      groupId: "demo-code-review",
      groupName: "代码复核工具",
      filesScanned: 1,
      highestSeverity: "informational" as const,
      activeFindingCount: 0,
      ignoredFindingCount: 0
    }))
  },
  createdAt: "2026-07-28T11:10:00Z",
  expiresAt: "2026-07-28T12:10:00Z"
};

export const demoAssistedInstallPlan: AssistedInstallPlan = {
  id: "demo-assisted-plan",
  sourcePlanId: demoInstallPreview.id,
  status: "manual-required",
  repository: demoInstallPreview.repository,
  summary: "仓库包含 4 个代码复核 Skills，并提供一个可选的 Python MCP 服务。",
  approach: "安装选中的 Skills，将已锁定的 Wheel 安装到隔离环境，再备份并写入 MCP 配置。仓库脚本不会执行。",
  complexity: "complex",
  requirements: [
    {
      id: "codex-cli",
      kind: "tool",
      title: "Codex CLI 已登录",
      description: "用于分析仓库说明并生成受约束的安装计划。",
      required: true,
      satisfied: true
    },
    {
      id: "python",
      kind: "runtime",
      title: "Python 3.11 或更高版本",
      description: "MCP 服务运行所需。",
      required: true,
      satisfied: true
    }
  ],
  steps: [
    {
      id: "install-skills",
      kind: "install-skills",
      title: "安装 Skills",
      description: "将选中的 Skill 目录安装到 Codex Skills 目录并记录来源。",
      status: "queued",
      required: true,
      supported: true,
      skillNames: demoInstallPreview.skills.map(skill => skill.name),
      permissionIds: ["skills-write"],
      reversible: true,
      recovery: "可从操作历史回滚到安装前状态。"
    },
    {
      id: "install-managed-tool",
      kind: "managed-python-tool",
      title: "安装受管代码关系工具",
      description: "仅使用分析阶段锁定并校验哈希的 Wheel 创建隔离 Python 环境。",
      status: "planned",
      required: true,
      supported: true,
      pythonPackage: "example-code-graph",
      versionSpec: "==2.3.7",
      pythonWheels: [
        {
          name: "example-code-graph",
          version: "2.3.7",
          filename: "example_code_graph-2.3.7-py3-none-any.whl",
          sha256: "1111111111111111111111111111111111111111111111111111111111111111",
          tags: ["py3-none-any"]
        },
        {
          name: "tree-sitter",
          version: "0.25.2",
          filename: "tree_sitter-0.25.2-cp39-abi3-win_amd64.whl",
          sha256: "2222222222222222222222222222222222222222222222222222222222222222",
          native: true,
          tags: ["cp39-abi3-win_amd64"]
        },
        {
          name: "tree-sitter-language-pack",
          version: "0.9.0",
          filename: "tree_sitter_language_pack-0.9.0-cp310-abi3-win_amd64.whl",
          sha256: "3333333333333333333333333333333333333333333333333333333333333333",
          native: true,
          tags: ["cp310-abi3-win_amd64"]
        },
        {
          name: "networkx",
          version: "3.5",
          filename: "networkx-3.5-py3-none-any.whl",
          sha256: "4444444444444444444444444444444444444444444444444444444444444444",
          tags: ["py3-none-any"]
        }
      ],
      entrypoint: "example-code-graph",
      permissionIds: ["pypi-wheel-lock", "managed-tool-write", "managed-tool-run", "managed-native-code"],
      reversible: true,
      recovery: "受管环境会整体移入隔离区。"
    },
    {
      id: "configure-mcp",
      kind: "configure-codex-mcp",
      title: "配置 Codex MCP",
      description: "备份现有配置后，新增指向受管工具的 MCP 条目。",
      status: "planned",
      required: true,
      supported: true,
      entrypoint: "example-code-graph",
      mcpServerName: "example_code_graph",
      mcpArgs: ["serve"],
      permissionIds: ["codex-mcp-config"],
      reversible: true,
      recovery: "恢复配置备份并隔离应用创建的所有权记录。"
    },
    {
      id: "initialize-project-index",
      kind: "manual",
      title: "首次建立项目索引",
      description: "该步骤需要结合项目实际情况选择索引范围，应用不会自动执行。",
      status: "manual",
      required: true,
      supported: false,
      permissionIds: [],
      reversible: false,
      recovery: "按照仓库说明在确认目标项目后手动执行。"
    }
  ],
  permissions: [
    {
      id: "skills-write",
      kind: "filesystem-write",
      title: "写入 Skills 目录",
      description: "仅写入本计划列出的 Skill 目录。",
      target: demoConfig.paths.skillsRoot,
      risk: "standard",
      required: true,
      reversible: true
    },
    {
      id: "pypi-wheel-lock",
      kind: "package-approval",
      title: "使用已锁定的 PyPI Wheel",
      description: "分析阶段已从官方 PyPI 下载；执行阶段只接受列表中的文件名和 SHA-256。",
      targets: ["example-code-graph==2.3.7 · 4 Wheels"],
      risk: "standard",
      required: true,
      reversible: true
    },
    {
      id: "managed-tool-write",
      kind: "filesystem-write",
      title: "创建受管工具环境",
      description: "只在应用数据目录中创建隔离 Python 环境。",
      targets: ["example-code-graph==2.3.7"],
      risk: "standard",
      required: true,
      reversible: true
    },
    {
      id: "managed-tool-run",
      kind: "process",
      title: "允许受管工具运行",
      description: "允许 Codex 后续通过 MCP 启动批准的入口。",
      targets: ["example-code-graph"],
      risk: "standard",
      required: true,
      reversible: true
    },
    {
      id: "managed-native-code",
      kind: "high-risk-process",
      title: "运行本机代码（高风险）",
      description: "两个平台 Wheel 包含本机代码；文件名、平台标签和 SHA-256 已固定。",
      targets: ["tree-sitter · win_amd64", "tree-sitter-language-pack · win_amd64"],
      risk: "high",
      required: true,
      reversible: true
    },
    {
      id: "codex-mcp-config",
      kind: "configuration",
      title: "配置 Codex MCP",
      description: "修改前自动备份，且只新增计划中的受管 MCP 服务。",
      target: "Codex config.toml",
      risk: "standard",
      required: true,
      reversible: true
    }
  ],
  warnings: [
    "仓库脚本不会被直接执行；无法安全自动化的步骤会保留为人工步骤。",
    "原生 Wheel 需要单独批准高风险权限。"
  ],
  needsProjectRoot: true,
  projectRootReason: "MCP 服务需要一个明确的项目目录作为默认分析范围。",
  codexModel: "gpt-5.6-luna",
  reasoningEffort: "xhigh",
  contextFileCount: 348,
  createdAt: "2026-07-28T11:12:00Z",
  expiresAt: "2026-07-28T12:12:00Z",
  skills: demoInstallPreview.skills,
  scan: demoInstallPreview.scan
};

export const demoAssistedInstallProgress: AssistedInstallProgress = {
  referenceId: demoAssistedInstallPlan.id,
  runId: "demo-assisted-run",
  sequence: 6,
  phase: "running",
  message: "正在创建受管工具环境",
  currentStepId: "install-managed-tool",
  completedSteps: 1,
  totalSteps: 4,
  activityCount: 8,
  steps: [
    { id: "install-skills", title: "安装 Skills", status: "completed", completedAt: "2026-07-28T11:14:10Z" },
    { id: "install-managed-tool", title: "安装受管代码关系工具", status: "running", startedAt: "2026-07-28T11:14:10Z" },
    { id: "configure-mcp", title: "配置 Codex MCP", status: "queued" },
    { id: "initialize-project-index", title: "首次建立项目索引", status: "manual-pending", message: "待手动处理" }
  ],
  startedAt: "2026-07-28T11:14:00Z",
  updatedAt: "2026-07-28T11:14:18Z",
  terminal: false
};

export const demoAssistedInstallResult: AssistedInstallResult = {
  plan: { ...demoAssistedInstallPlan, status: "partial", recoveryStatus: "available" },
  transaction: {
    id: "demo-tx-assisted-install",
    type: "assisted-install",
    status: "partial",
    targets: demoInstallPreview.skills.map(skill => skill.name),
    startedAt: "2026-07-28T11:14:00Z",
    completedAt: "2026-07-28T11:14:35Z"
  },
  referenceId: demoAssistedInstallPlan.id,
  runId: "demo-assisted-run",
  progress: {
    ...demoAssistedInstallProgress,
    sequence: 8,
    phase: "partial",
    message: "自动步骤已完成，1 个人工步骤仍待处理",
    completedSteps: 4,
    currentStepId: "",
    terminal: true,
    updatedAt: "2026-07-28T11:14:35Z",
    steps: demoAssistedInstallPlan.steps.map(step => ({
      id: step.id,
      title: step.title,
      kind: step.kind,
      status: step.kind === "manual" ? "manual-pending" : "completed",
      message: step.kind === "manual" ? "待手动处理" : "步骤已完成"
    }))
  }
};

export const demoQuarantine = [
  {
    skill: "legacy-helper",
    transactionId: "demo-tx-quarantine",
    path: "D:\\CodexSkillManager\\data\\quarantine\\legacy-helper"
  }
];
