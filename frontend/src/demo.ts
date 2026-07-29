import type {
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
    ignored: false
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
    ignoreReason: "匿名演示：已核对为预期的本地构建步骤"
  }
];

export const demoScanReport: ScanReport = {
  id: "demo-scan-release-helper",
  target: "release-helper",
  highestSeverity: "medium",
  activeHighestSeverity: "medium",
  findings: [...networkFindings, ...shellFindings],
  filesScanned: 42,
  activeFindingCount: networkFindings.length,
  ignoredFindingCount: shellFindings.length,
  status: "completed",
  completedAt: "2026-07-28T10:22:00Z",
  clusters: demoRiskClusters,
  codexReview: {
    id: "demo-codex-review",
    status: "completed",
    summary: "命中主要来自发布流程示例。未发现明确破坏行为，建议重点复核网络目标和命令参数。",
    overallVerdict: "人工复核后可接受",
    model: "gpt-5.6",
    reasoningEffort: "medium",
    contextMode: "full-target-read-only",
    contextFileCount: 128,
    startedAt: "2026-07-28T10:21:00Z",
    completedAt: "2026-07-28T10:22:00Z",
    totalSkills: 1,
    durationMillis: 60000,
    batches: [{
      index: 1,
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
    model: "gpt-5.6",
    reasoningEffort: "medium",
    timeoutSeconds: 300,
    maxSamplePerRisk: 8,
    skillsPerBatch: 4,
    maxParallelBatches: 2
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
      slug: "gpt-5.6",
      displayName: "GPT-5.6",
      description: "Latest frontier agentic coding model.",
      defaultReasoningLevel: "medium",
      reasoningLevels: [
        { effort: "medium", description: "平衡速度和分析深度" },
        { effort: "high", description: "更深入的语义复核" }
      ]
    }
  ]
};

export const demoInstallPreview: InstallPreview = {
  id: "demo-install-plan",
  repository: {
    provider: "github",
    fullName: "example/academic-research-skills",
    private: false,
    defaultBranch: "main",
    license: "MIT",
    resolvedRef: "main",
    commitSha: "be72d09a13f67de91a9b8748a46fa29b8b52dc10"
  },
  skills: [
    {
      name: "literature-search",
      description: "跨来源检索并整理学术证据",
      sourcePath: "skills/literature-search"
    },
    {
      name: "systematic-review",
      description: "系统综述筛选、提取与质量核查",
      sourcePath: "skills/systematic-review"
    },
    {
      name: "citation-checker",
      description: "核对引用格式、DOI 和参考文献",
      sourcePath: "skills/citation-checker"
    }
  ],
  scan: {
    ...demoScanReport,
    id: "demo-install-scan",
    target: "example/academic-research-skills",
    filesScanned: 57
  },
  createdAt: "2026-07-28T11:10:00Z",
  expiresAt: "2026-07-28T12:10:00Z"
};

export const demoQuarantine = [
  {
    skill: "legacy-helper",
    transactionId: "demo-tx-quarantine",
    path: "D:\\CodexSkillManager\\data\\quarantine\\legacy-helper"
  }
];
