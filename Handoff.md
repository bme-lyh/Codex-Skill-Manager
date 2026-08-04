# Codex Skill Manager handoff

## Project snapshot

- Product: Windows desktop and CLI manager for Codex-compatible Skills.
- Release line: `0.15.0`.
- Desktop: Wails v2 with a React/Vite/TypeScript frontend.
- CLI: `cmd/csm`, with stable JSON envelopes for automation.
- Default data directory: `%USERPROFILE%\.codex\skill-manager`.
- Managed Skill roots:
  - `codex-default` → `%USERPROFILE%\.codex\skills` (default install target)
  - `agents` → `%USERPROFILE%\.agents\skills`

## Architecture

```text
Wails UI / csm CLI
        │
        ▼
internal/manager        use cases, plans, gates, transactions, recovery
   ├─ config            schema migration and path/root validation
   ├─ inventory         root-aware discovery and content hashes
   ├─ scanner           deterministic local security checks
   ├─ githubsource      immutable GitHub resolution and safe staging
   ├─ provenance        source detection for existing Skills
   ├─ codexreview       optional semantic review through one Runner
   ├─ state             SQLite operational state
   └─ reporting         Markdown/JSON audit records
        │
        ├─ sources.lock.json   portable source truth (schema 2)
        └─ state.db            scans, approvals, groups, updates, history
```

The GUI and CLI call the same manager. UI code must not reproduce authorization
rules: previews, risk gates, target validation, and transaction decisions belong
to the Go backend.

### 0.14 source-group authority

`SourceGroup` is the management unit. A GitHub repository maps to one source
group, and the group includes every valid Skill discovered from the immutable
commit. `SourceTrustPolicy` is repository-wide. `SourceAnalysis` and
`GroupSecurityReport` keep reusable bilingual summaries (`en` and `zh`) while
retaining Skill evidence for diagnosis. `GroupOperation` and its parent
`Transaction` own install, update, and security status; child Skill records
only describe execution, backups, failures, and recovery.

`ApplyGroupInstall` and `ApplyGroupUpdate` reject incomplete target sets before
creating a mutation. Internal steps continue after a failure and the parent
ends as `completed`, `partial`, or `failed`. A persisted human approval can
cover Critical, High, Medium, and Low group findings without a typed reason;
immutable commit, path containment, staged hashes, snapshot completeness,
expiry, and recovery checks remain non-bypassable. Trusted repositories skip
the foreground risk prompt but still produce a background report.

Mutating operations use a process-wide, root-scoped writer lease. The desktop
shell rejects overlapping page operations and ignores stale dashboard refresh
responses. The lease is a UI/process fence, not a substitute for external file
change verification; apply paths still recheck hashes and plan digests.

### 0.15 group hardening

Group approvals are now bound to the exact persisted report plus root, group,
repository, commit, and `policyVersion`. The security-center and update-dialog
one-click flows reuse a report only while the current plan matches that report;
a newer report, changed commit, or different repository invalidates the stored
decision and requires a fresh approval. Legacy v0.14 reports without a policy
version keep their group-prefix approval key only after the same binding
checks pass.

Source-group parent transactions left `running` by an application exit are
reconciled on the next dashboard read: the parent and its `GroupOperation`
become `recovery-required`, queued/running steps become `interrupted`, and the
parent rollback entry remains the recovery authority. Completed child install
transactions keep their own journals and recover in reverse order through
`Rollback`.

`GetGroupOperation` and `GetGroupMetadata` are read-only desktop/CLI contracts
(`csm group operation --id ID`, `csm group metadata --group ID`). The install
dialog shows the complete source group without per-Skill selection controls;
the Groups page renders the latest analysis, security report, operation, and
update status for source groups. A book-to-skill-shaped regression covers
root-level and nested `SKILL.md` discovery, analysis, one-click approval, and
complete-group installation.

## Root and identity model

Names are unique only inside a root. Persisted or cross-root operations use
`rootId + name`; source packages are stored under `rootId + NUL + packageId`.
Install, adoption, grouping, scan, update, quarantine, restore, and rollback
carry `rootId`. Compatibility wrappers may default an omitted root to
`codex-default`, but new API and CLI calls should always send `--root` or
`rootId` explicitly.

Both roots reserve a top-level `.system` directory. Never create, install,
quarantine, restore, or roll back that target. Registered roots must be
absolute, enabled, non-overlapping, and separate from manager data, staging,
backup, report, cache, and quarantine paths.

## Installation and update flow

1. Resolve GitHub branches/tags to a full commit SHA, or snapshot a local source.
2. Discover candidate Skills and run the local scanner against exact targets.
3. Seal the preview, including `TargetRootID`, candidates, scan, source, and expiry.
4. Build the mandatory local assessment. The Codex-first Add project path then
   starts a bounded read-only project review automatically.
5. Generate a typed plan and require one human confirmation. The confirmation is
   persisted with source, report, assessment, plan, and permission digests.
6. Revalidate all digests and execute through the existing journaled assisted
   installer. No downloaded Skill script or dependency/publishing command runs.
7. Back up replacements, update the root-qualified lock and security state, then
   report a completed or partial transaction with per-Skill recovery IDs.

Updates create a new immutable preview and remain bound to the package's original
root. Removal always means moving one explicit Skill directory to quarantine.

## Codex review module

`internal/codexreview/runner.go` is the single process boundary for Codex CLI
work. It centralizes CLI discovery, filtered environment, timeouts, output
limits, retries, diagnostics, and schema validation. Project scanning,
installation analysis, Skill security review, and related update work reuse it.
Codex output is untrusted proposal data. Shell access remains disabled and all
paths, permissions, package locks, and actions are revalidated locally.

## Desktop UI

The shell uses a collapsible sidebar, a root selector, system or explicit light/dark appearance,
Windows accent color, visible focus, and reduced-motion/high-contrast support. Semantic status,
review, and assisted-install surfaces must remain theme-aware; do not add a hard-coded white
panel without a corresponding token.
Primary sections are Home, Assets, Security, Activity, and Settings. English and
Chinese strings live in `frontend/src/i18n.tsx`; keep wording short, literal,
and task-focused. Avoid promotional copy, vague assurances, and invented jargon.

The add-project dialog selects a root before analysis. Once a plan exists, the
target is locked. Same-name Skills in different roots are valid; ambiguous or
cross-root bulk actions must stop and ask for a single root.

## Important files

- `app.go`: Wails facade and desktop-only helpers.
- `cmd/csm/main.go`: CLI parsing and JSON command surface.
- `internal/model/model.go`: shared domain and API schema.
- `internal/manager/manager.go`: standard install/manage/update/recovery flows.
- `internal/manager/group_operations.go`: source-group trust, analysis,
  security approval, complete-group apply, and parent transaction recovery.
- `internal/manager/assisted_install.go`: typed assisted-install execution.
- `internal/config/config.go`: schema 2 root defaults and path validation.
- `internal/state/state.go`: SQLite schemas and v1 migrations.
- `frontend/src/App.tsx`: application state and page composition.
- `frontend/src/api.ts`: Wails/demo API adapter.
- `frontend/src/grouping.ts`: source-group identity and complete selection
  normalization shared by install and update views.
- `frontend/src/theme.ts`: persisted appearance normalization and DOM theme application.
- `scripts/build.ps1`: verified production build into `build/bin`.
- `scripts/dev.ps1`: local Wails development launcher.
- `scripts/deploy-local.ps1`: dry-run-first, manifest-verified local deployment.
- `scripts/package-release.ps1`: standard/portable release archives and checksums.

## Development workflow

```powershell
# Frontend
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend test
pnpm --dir frontend run build

# Go (the repository can use .toolchains/go when system Go is unavailable)
go test ./...
go vet ./...

# Desktop + CLI; this refreshes the project-local applications
.\scripts\build.ps1

# Optional installed copy; dry run first
.\scripts\deploy-local.ps1
.\scripts\deploy-local.ps1 -Apply
```

Use `wails build` through `scripts/build.ps1`; plain `go build` is not a valid
desktop build. After changing `skills/codex-skill-manager`, run the bundled
validator. Windows path-security tests may require running outside the Codex
sandbox, but test fixtures must remain under `t.TempDir()` and must never point
at the real user Skill roots.

## Release checklist

1. Update versions in `internal/model/model.go`, `frontend/package.json`,
   `wails.json`, and the first `CHANGELOG.md` entry.
2. Update README download links, user/agent docs, and this handoff when contracts
   or architecture change.
3. Refresh English and Chinese screenshots/carousels from fictional demo data. Keep every
   screen at 1440×900; split long screens into scroll-positioned frames and update
   `scripts/screenshot-frames.json`.
4. Run `gofmt`, frontend tests/build, `go test ./...`, `go vet ./...`, validator,
   and `git diff --check`.
5. Run `scripts/build.ps1`; verify `build/bin/build-manifest.json` and launch the
   desktop executable briefly.
6. Commit the clean release state, package with
   `scripts/package-release.ps1 -Version <version>`, verify checksums, tag, push,
   and publish the GitHub Release.

## Safety invariants

- Never execute scripts from a Skill repository.
- Never write to either `.system` directory.
- Never permanently delete Skill directories; quarantine explicit targets.
- Never log tokens, cookies, authorization headers, or credential-helper output.
- Cloud/Codex scanning stays opt-in.
- Preserve local edits unless replacement is explicitly approved.
- Resolve mutable GitHub refs to immutable commits before showing an apply plan.
- Every mutation needs an explicit target, preview, journal, backup/quarantine,
  structured result, failure status, and recovery path.

## Known maintenance notes

- `Paths.SkillsRoot` remains as the schema 1 compatibility alias for the default
  Codex root. New code should use configured roots and `rootId`.
- Old persisted plans without root-bound digests should be regenerated instead
  of silently upgraded.
- Release binaries are currently unsigned; users should verify published
  SHA-256 checksums.
- Update status storage is keyed by `(root_id, group_id)`; do not reintroduce a
  global group-only key. Root-level candidates use `SourcePath: "."` and must
  remain explicitly contained within their staging root.
- Codex chunk summaries treat the local manifest as authoritative. A
  `coverageMismatch` is a lower-confidence warning, never evidence of complete
  semantic coverage.
- Local Skills without verifiable provenance are stored as
  `sourceAssociation: unlinked`. The UI can explicitly link one Skill to a
  GitHub URL/ref; linking requires an immutable commit and an exact local/remote
  tree hash match.
- `ApplyAdoptionBestEffort`, `ApplyInstallBestEffort`, and multi-Skill audit
  results use child outcomes. A parent transaction may be `partial`; retry the
  failed child instead of repeating successful targets.
- Release `0.15.0` keeps source groups as the authoritative install, update,
  and security unit, adds strict approval binding (report + repository +
  commit + policy version), restart recovery for interrupted group operations,
  group metadata/operation read interfaces, and complete-group-only install
  controls while retaining standard and legacy APIs for compatibility.
