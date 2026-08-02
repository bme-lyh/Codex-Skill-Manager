# Layered Security Workflow Refactor Plan

Status: approved implementation plan  
Baseline: `1f1e370` (`Release 0.9.0: add layered Codex project scanning`)  
Target branch: `codex/layered-security-refactor`

## 1. Outcome

Refactor Codex Skill Manager from two user-selected installation modes into one
progressive project intake workflow:

1. select a GitHub or local source;
2. pin and stage the source safely;
3. run the mandatory local assessment;
4. explain any triggered or optional enhanced checks;
5. review the supported installation targets and permissions;
6. apply changes transactionally;
7. verify the result or offer recovery.

The common low-risk Skill flow should require no knowledge of the terms
"standard installation" or "assisted installation". Codex analysis remains an
explicitly consented enhancement and never becomes an implicit network action.

## 2. Safety invariants

The refactor must preserve these non-negotiable properties:

- never write to `.codex/skills/.system`;
- never execute repository scripts;
- pin GitHub refs to immutable commit SHAs before assessment or installation;
- treat staged files and model output as untrusted input;
- keep Codex/cloud analysis opt-in;
- restrict executable plan actions to the existing local allowlist;
- recompute and verify source, scan, plan, configuration, and output digests at
  the existing trust boundaries;
- require explicit targets and per-capability approval before mutation;
- journal every mutation and retain cancel, failure, recovery, and rollback
  behavior;
- move explicit Skills to quarantine instead of permanently deleting them;
- preserve structured CLI and Wails contracts during migration.

Critical deterministic blockers cannot be bypassed by a generic "ignore"
action. A future exception workflow must distinguish false-positive review from
risk acceptance and must remain auditable.

## 3. Current constraints

- `frontend/src/install/InstallDialog.tsx` owns source intake, restoration,
  project scanning, plan review, installation, cancellation, and rollback.
- `frontend/src/App.tsx` contains most pages and navigation in one module.
- `internal/manager/assisted_install.go` mixes orchestration, progress,
  preflight, execution, persistence, and recovery.
- `internal/model/model.go` exposes multiple domains from one file.
- Release 0.9.0 already provides a reusable read-only `CodexProjectScanResult`
  bound to a verified source digest. The refactor must build on this capability.

The knowledge graph flags these files as high-concentration decomposition
targets. Automated dead-code output is advisory only and must not be used to
remove Go data types or Wails entry points without direct verification.

## 4. Product information architecture

The long-term navigation is reduced to five destinations:

- Home: current posture, active work, and required attention;
- Assets: Skills, plugins, and projects as filtered views;
- Security: assessments, findings, decisions, and coverage;
- Activity: transactions, rollback, quarantine, and reports;
- Settings: paths, providers, consent, policy, and language.

This change is staged. The first implementation changes the add/install dialog
and its terminology without removing existing pages or stored data.

## 5. Unified workflow states

The frontend presents four user-facing stages while the backend retains finer
grained resumable states.

| User stage | Backend states | Primary question |
| --- | --- | --- |
| Source | draft, preparing | What source should be assessed? |
| Assess | staging, local-assessment, enhanced-scan | What is this project and what checks apply? |
| Review | review-required, ready, blocked | What will change and is it allowed? |
| Apply | applying, verifying, completed, partial, recovery-required | What changed and what should happen next? |

The UI derives the current stage from persisted backend artifacts. Browser
storage may retain a draft or active reference ID, but it is never the authority
for a source, scan, plan, transaction, or recovery state.

## 6. Layered assessment model

Introduce a deterministic local assessment associated with every
`InstallPreview`.

### 6.1 Required local checks

- immutable source identity or explicit local-source identity;
- safe staging and bounded inventory;
- project marker and documentation discovery;
- Codex Skill discovery and target validation;
- built-in secrets, dangerous-command, path, link, and dependency scan;
- explicit install target and rollback capability;
- expiry and digest validation before plan generation and again before apply.

### 6.2 Triggered checks

The assessment records a reason when an enhanced check becomes necessary:

- executables, native packages, installers, or elevation markers;
- container or infrastructure-as-code files;
- MCP configuration or managed tool installation;
- prompt/tool instructions in a Codex Skill;
- mixed or ambiguous repository classification;
- incomplete installation documentation or unsupported installation methods;
- high-risk local findings or materially incomplete coverage;
- unusually large repositories requiring bounded partitioning.

Codex semantic analysis is an available provider for project understanding and
installation-method review. If policy marks it required and the user declines
consent, the result is `incomplete` or `blocked`, never silently downgraded.

### 6.3 Optional deep checks

Optional checks are collapsed in the default UI and never run automatically:

- additional Codex deep analysis;
- external SAST, dependency, container, IaC, SBOM, or license providers;
- sandboxed dynamic observation;
- extended provenance and history review.

The provider interface is designed now; external provider execution is not part
of this implementation.

## 7. Backend contract

Add model types with JSON-compatible fields:

- `ProjectAssessment`
  - assessment/source IDs, repository, classification, evidence;
  - required/triggered/optional checks;
  - gate status, summary, highest risk, coverage;
  - supported targets and enhanced-scan recommendation;
  - creation and expiry timestamps.
- `LayeredSecurityCheck`
  - ID, layer, requirement, status, title, summary, reason, provider, evidence;
  - values use bounded enums with unknown-value-safe clients.
- `InstallTargetPreview`
  - kind, display name, path, supported, reason, permission IDs, reversible.

Add a read-only Wails/manager operation:

```text
AssessInstallSource(sourcePlanID) -> ProjectAssessment
```

The operation loads the persisted preview, verifies expiry and identity, walks
only the verified staging root, derives deterministic markers, summarizes the
existing local scan, and returns no mutation permissions.

The existing operations remain compatible during migration:

```text
PrepareGitHub / PrepareLocal
ScanProjectWithCodex
AnalyzeInstallFromProjectScan
ApplyInstall / ApplyAssistedInstall
CancelAssistedInstall / Rollback
```

No arbitrary-project execution path is added. Supported actions remain Skill
installation, locked official PyPI Wheels, Codex MCP configuration, and manual
instructions under the existing finalizer.

## 8. Frontend contract

Replace the initial install-mode selector with a source-only step. After source
preparation, always request and render the local assessment.

The assessment summary uses four plain-language outcomes:

- `ready`: local checks passed and the supported target can be installed;
- `attention`: review findings or an enhanced check recommendation;
- `blocked`: a deterministic gate prevents installation;
- `incomplete`: required coverage or consent is unavailable.

Each finding answers:

1. what was found;
2. why it matters;
3. which file, target, or permission is affected;
4. the safest next action.

Only one primary action appears per state. Enhanced Codex scanning is offered
after local assessment with an explicit consent description. Users can choose
the local Skill installation path only when the local gate and target support it.

### 8.1 Component boundaries

Keep `InstallDialog` as an orchestration shell and extract presentation into:

- `install/components/WorkflowStepper.tsx`;
- `install/components/SourceStep.tsx`;
- `install/components/AssessmentView.tsx`;
- `install/components/ProjectScanView.tsx`;
- `install/components/PlanReview.tsx`;
- `install/components/ExecutionView.tsx`;
- `install/components/OutcomeBanner.tsx`.

Pure workflow derivation and persistence helpers remain in `install/state.ts`
and receive unit tests. API normalization remains in `frontend/src/api.ts`.

## 9. Implementation phases

### Phase A - plan and characterization

- save this plan;
- record baseline Go, frontend, Wails, and Skill validation results;
- add contract tests for existing source, scan, plan, apply, cancel, and recovery
  behavior where coverage is missing.

### Phase B - local assessment backend

- add the bounded assessment model and local classifier;
- expose the read-only manager and Wails methods;
- verify staging-root containment, symlink handling, expiry, and stable output;
- add table-driven tests for Skill, plugin marker, application/library marker,
  mixed, ambiguous, high-risk, and expired-plan cases.

### Phase C - unified frontend

- add TypeScript assessment types and API normalization;
- remove the initial standard/assisted choice;
- always prepare and locally assess a source first;
- render the four-state assessment summary and workflow stepper;
- retain explicit Codex consent, background progress, restoration, cancel,
  retry, apply, and rollback behavior;
- extract view components without changing security decisions.

### Phase D - documentation and migration

- update Chinese and English GUI/getting-started/security documentation;
- document JSON/Wails additions for agent consumers;
- retain existing saved draft keys with a compatibility reader and write the
  new schema only after successful parsing;
- do not bulk-delete legacy artifacts or user data.

### Phase E - validation and release handoff

- format changed Go files;
- run Go tests, frontend tests, TypeScript/Vite build, Wails build, and the
  bundled Skill validator;
- inspect the final diff, dependency impact, generated bindings, and security
  invariants;
- commit coherent changes and push a review branch to GitHub;
- do not merge to `main` unless separately authorized.

## 10. Test matrix

| Area | Required cases |
| --- | --- |
| Source | GitHub SHA pin, local source, expiry, staging containment |
| Classification | Skill, plugin marker, app/library marker, mixed, unknown |
| Local gate | clean, ignored low finding, active high/critical, incomplete coverage |
| Consent | no Codex call before consent, decline behavior, retry and resume |
| Plan | scan/source digest mismatch, stale configuration, permission dependencies |
| Apply | no mutation before approval, successful journal, partial/manual result |
| Recovery | cancellation, orphan recovery, rollback success and failure |
| UI | keyboard/focus, one primary action, bilingual copy, narrow viewport |
| Compatibility | existing CLI JSON, existing Wails methods, saved active references |

## 11. Acceptance criteria

- A clean Skill repository follows Source -> Assess -> Review -> Apply without
  choosing an implementation mode.
- The first visible feedback appears immediately and progress remains resumable.
- No Codex/network analysis starts without explicit consent.
- The assessment visibly distinguishes required, triggered, and optional checks.
- A blocked result names the blocker and performs no mutation.
- Every apply view shows target, permissions, rollback status, and final state.
- Existing CLI behavior and structured output remain compatible.
- All repository-required checks pass, or the final handoff documents an
  independently reproducible environment blocker.

## 12. Rollback strategy

Implementation commits are separated into plan, backend contract, frontend
workflow, and documentation/verification. The existing APIs remain available,
so any implementation commit can be reverted independently without changing
stored transactions or requiring user-data deletion.
