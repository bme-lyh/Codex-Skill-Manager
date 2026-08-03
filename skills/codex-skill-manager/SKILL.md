---
name: codex-skill-manager
description: Safely inspect, install, manage, group, update, audit, quarantine, restore, or roll back Codex skills with Codex Skill Manager. Use when a user asks about installed skill provenance, GitHub or local skill installation, grouped updates, security findings, backups, uninstall, recovery, or scheduled update checks.
---

# Codex Skill Manager

Use the `csm` CLI as the stable automation surface. Prefer `--json`, preserve
transaction IDs, and keep the user informed before any filesystem mutation.

## Workflow

1. Run `csm doctor --json` and `csm dashboard --json`.
   Read `roots` and `defaultRootId`. Before any root-scoped action, choose one
   explicit `rootId`; new installations default to `codex-default` only when
   the user has not requested another target.
2. For read-only requests, use `audit`, `check`, `history`, or `reports`.
   Report the structured update status, checked time, remote Commit, and exact
   outdated Skill names; never present a bare Commit as the status.
3. For existing unmanaged Skills, create a root-qualified `manage` plan, show detected source
   evidence, confidence, file snapshot and scan findings, then apply only the
   exact confirmed names.
4. For GitHub or local installation, create a plan first. The manager pins a
   full GitHub Commit or creates a bounded local snapshot, understands the
   project type and installation route, then enforces the mandatory persisted
   layered assessment before any apply or optional Codex phase. Run
   `install --plan-id PLAN --assess` and show the resolved source, discovered
   Skills, project classification, assessment gate, target scope, and scan
   findings. If different Skill contents share one name, report every
   conflicting source path and select a specific repository subtree before
   creating a plan. Prefer an explicitly identified `skills-codex` mirror for
   Codex only after showing that scope.
5. Codex is a semantic check provider, not an installation mode. When the user
   explicitly requests deeper checks for a complex repository, use
   `install --assist` to create a read-only Codex project scan. Present its
   overview, advisory security conclusion, evidence limitations, and
   declarative installation methods. Only after the user explicitly agrees,
   use `--assist --project-scan-id ... --create-plan`. Present the resulting
   requirements, warnings, typed steps, exact Skills, permission IDs, manual
   work, and project-root requirement. Do not apply it in the same command.
6. Ask for confirmation of the exact selected Skill names. Critical clusters
   cannot be ignored and must remain blocked. High clusters require individual
   explicit confirmation and a non-empty reason; applying a standard plan with
   an accepted High cluster also requires `--accept-high-risk`. Report-wide
   decisions may cover only Medium and lower severities and cannot bypass these
   rules.
7. Apply a confirmed assisted plan only with `--assist --plan-id ... --apply`,
   repeated exact `--skill` and `--grant` values, and `--project-root` only when
   the plan requires MCP. Otherwise apply the standard plan. Return the
   transaction ID, recovery status, and report location.
8. For removal, use `csm remove --root ROOT_ID` with explicit names. Explain that content is
   moved to quarantine and can be restored.
9. Prefer risk-cluster decisions over individual findings. Preserve every
   member fingerprint and use `warning --report SCAN_ID --dry-run` before a
   report-wide CLI decision. Explain that cluster decisions affect current-risk
   counting. Reload decisions at apply time.
10. Use `group` commands for layout-only organization. Explain that this does
   not alter the source used for updates or security.
11. When optional Codex risk review is requested, repeat `--skill` for the exact
    selected Skills when possible. Present the stable per-Skill summaries,
    concerns and group-task failures separately; do not collapse them into a single
    repository verdict.

## Safety rules

- Never use recursive deletion or wildcard mutation.
- Never modify either root's `.system` directory.
- Never combine same-name Skills from different roots in one mutation. Pass
  `--root` for install, manage, group, audit, update, quarantine, and restore.
- Never edit `sources.lock.json` or `state.db` manually.
- Never install directly from an unresolved or unscanned working tree.
- Never expose GitHub tokens in prompts, arguments, logs or reports.
- Treat `check` as read-only; scheduled jobs may check but must not update.
- Use `github-auth` before diagnosing credential or quota failures. Respect
  `retryAt`, while preserving the last successful update status.
- Treat optional Codex CLI review as advisory. It must be enabled, authenticated,
  ephemeral, read-only, schema validated, and rooted at the complete trusted
  scan target. Skills from the same application group must remain in one review
  task. Treat the compact local-rule overview as supplemental leads. Preserve
  `skillReviews`, group-task status, retry attempts and partial results.
- Treat `install --assist` as a separate per-invocation opt-in; it does not
  require the Security Center review toggle but still uses the configured CLI,
  model, reasoning effort, and account usage. Codex output is untrusted proposal
  data, never shell authority. Only locally finalized typed steps may run.
  Approved automatic steps may run even when required manual work remains. Treat
  a `partial` result as incomplete, report every manual item, pass only
  permissions returned by that plan, and never edit or execute internal
  assisted-plan snapshots. If Codex cannot produce a valid assisted plan,
  preserve the source preview and offer the standard Skill-only plan instead
  of treating the source analysis as failed.
- A GUI batch decision may atomically ignore eligible Medium-or-lower clusters
  without Codex review. It must never include High or Critical clusters.
  Preserve explicit cluster targets and a recovery path in the transaction journal.
- Use an isolated configuration for tests.

Read [references/cli-contract.md](references/cli-contract.md) before executing
mutating commands.
