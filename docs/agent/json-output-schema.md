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
`contextMode: "full-target-read-only"` and `contextFileCount` prove which
repository context was available to the review. `skillReviews` contains one
stable entry per requested Skill with `summary`, `verdict`, `confidence`,
`concerns`, `clusterIds` and validated `clusterReviews`. `batches` records
the effective `groupId`, `groupName`, member Skills, task status and `attempts`;
`totalSkills` and `durationMillis` support
progress and performance reporting. The legacy flat `reviews` collection is
retained for cluster-oriented consumers.

Desktop `codex-review-progress` events include a monotonically increasing
`sequence` per review. Consumers must ignore an event whose sequence is not newer
than the last accepted event for the same `reviewId`.

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

`codex status` returns `available`, `authenticated`, `compatible`, `path`,
`version`, `authStatus`, `checkedAt`, optional `missingCapabilities`, and an
optional user-readable `error`. After successful authentication, `models`
contains the CLI's visible model catalog. Each entry provides `slug`,
`displayName`, optional `description`, `defaultReasoningLevel`, and supported
`reasoningLevels`. A non-fatal catalog failure is exposed separately as
`modelCatalogError`. Consumers must branch on capabilities rather than matching
a CLI version string and must not replace the dynamic catalog with a hard-coded
model list.
