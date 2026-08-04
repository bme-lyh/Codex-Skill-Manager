# JSON output schema

With `--json`, commands return one JSON object:

```json
{
  "schemaVersion": "1.0",
  "command": "dashboard",
  "status": "ok",
  "data": {}
}
```

On failure, `status` is `error`, `error` contains a user-readable message, and
the process exits non-zero. Consumers must ignore unknown fields, branch on
`schemaVersion`, and never parse human-readable output. Domain objects use the
JSON field names declared in `internal/model/model.go`.

Collection fields in dashboard, scan and quarantine responses are always JSON
arrays. Empty collections are encoded as `[]`, never `null`, so desktop and
Agent clients may safely iterate them.

Scan findings include `title`, `fingerprint`, `ignored`, `fileClass`,
`category`, `clusterId`, `deterministic` and optional `ignoreReason`. Reports
retain raw findings and expose user-facing `clusters`; active/ignored counts are
cluster counts. Optional `codexReview` data is advisory and schema validated;
`contextMode: "full-target-packaged-no-tools"` and `contextFileCount` record
which packaged repository context was available to the review. `skillReviews` contains one
stable entry per requested Skill with `summary`, `verdict`, `confidence`,
`concerns`, `clusterIds` and validated `clusterReviews`. `batches` records
the effective `groupId`, `groupName`, member Skills, task status and `attempts`;
`totalSkills` and `durationMillis` support
progress and performance reporting. The legacy flat `reviews` collection is
retained for cluster-oriented consumers.

Desktop `codex-review-progress` events include a monotonically increasing
`sequence` per review. Consumers must ignore an event whose sequence is not newer
than the last accepted event for the same `reviewId`.

Desktop planned installation returns domain objects directly. With
`csm --json install --assist`, `data` is first a `CodexProjectScanResult` with
`id`, `sourcePlanId`, `summary`, `security`, `installationMethods`, coverage and
redaction/truncation counts, focus-file paths, context/scan digests, timestamps,
and expiry. Security contains the advisory verdict, summary, confidence, local
risk baseline, and evidence-bound concerns.

`csm --json install --assist --project-scan-id ID --create-plan` returns an
`AssistedInstallPlan`. It binds `id`, `sourcePlanId`, `projectScanId`, and
`projectScanDigest` to a
source identity, `planDigest`, `contextDigest`, optional `configFingerprint`,
and expiry. Reader-facing fields include `summary`, `approach`, `complexity`,
`requirements`, `warnings`, `skills`, `scan`, `codexModel`,
`reasoningEffort`, `contextFileCount`, and optional project-root guidance.

Every `steps` item has an ID, typed `kind`, title, description, status,
`supported`, `required`, exact targets, `permissionIds`, reversibility and
recovery text. Runtime fields may add a child transaction ID, target and backup
paths, ownership-manifest path, output hashes, applied configuration hash,
timestamps, and error.
For `managed-python-tool`, `pythonWheels` is the complete approval-time
dependency lock. Each item contains `name`, `version`, `filename`, `sha256`,
optional `native`, and compatibility `tags`. These fields are included in
`planDigest`; consumers should show the full values on the approval surface.
`permissions` is derived locally and contains explicit IDs, descriptions,
`risk`, required state, and targets; model output cannot add executable
permission kinds. Existing bounded actions use explicit `standard` risk. A lock
containing native Wheels adds `managed-native-code` with `risk: "high"`, whose
targets list the affected Wheel filenames, platform tags, and hashes.

`assisted-install-progress` events contain `referenceId`, `runId`, monotonic
`sequence`, `phase`, `message`, current step, completed/total counts,
`activityCount`, step snapshots, timestamps, terminal state, and error. A
consumer must ignore an event whose sequence is not newer for the same run and
should refresh the backend snapshot after reopening the dialog or reconnecting.

`AssistedInstallResult` returns `referenceId`, execution `runId`, the finalized
plan, parent transaction, and latest progress snapshot. A running or completed
plan may also carry `transactionId` and `recoveryStatus`. An assisted transaction
carries `steps` and `recoveryStatus` in addition to the normal explicit targets,
backup paths, status, timestamps, and error. Partial or failed results retain
completed-step metadata so recovery does not depend on the front end's in-memory
state.

After apply begins, the plan also persists the exact `selectedSkills`,
canonical `projectRoot`, approved permissions, and `outputLocale`. An empty or
missing `selectedSkills` during recovery means “select nothing”; consumers must
never expand it to every candidate. Analysis progress uses the fixed stages
`inventory`, `codex-analysis`, `validation`, `dependency-lock`, and
`finalizing`; stage count and completed stages are monotonic for one stable
run ID.

Security scan reports include `skills`, a per-Skill summary with effective group,
source path, file count and active/ignored warning counts. Findings and clusters
also carry `skillName`, `groupId` and `groupName`, allowing readers to render
Group → Skill → Warning without reconstructing ownership from file paths.
Dashboard `riskCount` is the number of unique, active High/Critical clusters
from the latest in-scope report per target.

A management preview contains `id`, explicit `skills`, detected `sources`, a
`scan`, `createdAt` and `expiresAt`. Each source includes provider, repository,
source path, effective source group, confidence and evidence. Applying it
returns the standard transaction object with `type: "manage"`.

Dashboard Skills contain both effective `groupId`/`groupName` and immutable
source-provenance `sourceGroupId`/`sourceGroupName`. `groups` is the editable
layout; `sourceGroups` is the update authority. Group records also expose
`manual` and `position`.

Dashboard `updateStatuses` contains one latest record per live source group,
with `status`, `checkedAt`, `remoteCommit`, `currentCommits`,
`outdatedSkills`, and optional `error`. A status may be `rate-limited` and carry
`retryAt`, rate-limit values, cache provenance, and last-success fields.
`lastUpdateCheck` is the newest check
timestamp and `updateCount` counts source groups with available updates.

Source-group operations in the 0.14 contract expose additive `sourceGroupId`
and `sourceGroupName` fields on install previews. `ApplyGroupInstall` and
`ApplyGroupUpdate` require the complete set of valid preview Skills; a partial
set is rejected before a transaction is created. Their parent transaction
(`type: "group-install"` or `"group-update"`) carries `groupId`, `groupName`,
`operationId`, and per-Skill `itemResults`; a reusable `GroupOperation` record
stores the same authoritative status plus child transaction, backup, and
recovery diagnostics. Existing selective `ApplyInstall` remains available as
a compatibility API.

`SourceTrustPolicy` is repository-wide and keyed by canonical lower-case
GitHub `owner/repository`. Set/revoke actions return ordinary transactions and
append `SourceTrustAudit` records. Trust is advisory only: immutable commits,
staged hashes, path containment, scanner findings, and technical recovery
checks remain mandatory. A critical group finding may be approved with an
empty reason after an explicit persisted decision; the approval does not
authorize any additional command or bypass those integrity checks.

Reusable source analysis and security records are represented by
`SourceAnalysis` and `GroupSecurityReport`; summaries use `LocalizedText`
(`en`/`zh`) with locale fallback. Detailed scanner `ScanReport` and legacy
install/update objects remain unchanged and continue to be accepted.

The group facade exposes `GetOrCreateSourceGroupAnalysis`,
`RunGroupSecurityCheck`, `ApproveGroupSecurity`, `ApproveGroupRisk`,
`PrepareGroupUpdate`, `ApplyGroupInstall`, and `ApplyGroupUpdate`. Group
approval is persisted against the report/plan and repository group; it does
not authorize arbitrary commands.

`codex status` returns `available`, `authenticated`, `compatible`, `path`,
`version`, `authStatus`, `checkedAt`, optional `missingCapabilities`, and an
optional user-readable `error`. After successful authentication, `models`
contains the CLI's visible model catalog. Each entry provides `slug`,
`displayName`, optional `description`, `defaultReasoningLevel`, and supported
`reasoningLevels`. A non-fatal catalog failure is exposed separately as
`modelCatalogError`. Consumers must branch on capabilities rather than matching
a CLI version string and must not replace the dynamic catalog with a hard-coded
model list.
