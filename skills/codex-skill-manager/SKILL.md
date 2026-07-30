---
name: codex-skill-manager
description: Safely inspect, install, manage, group, update, audit, quarantine, restore, or roll back Codex skills with Codex Skill Manager. Use when a user asks about installed skill provenance, GitHub or local skill installation, grouped updates, security findings, backups, uninstall, recovery, or scheduled update checks.
---

# Codex Skill Manager

Use the `csm` CLI as the stable automation surface. Prefer `--json`, preserve
transaction IDs, and keep the user informed before any filesystem mutation.

## Workflow

1. Run `csm doctor --json` and `csm dashboard --json`.
2. For read-only requests, use `audit`, `check`, `history`, or `reports`.
   Report the structured update status, checked time, remote Commit, and exact
   outdated Skill names; never present a bare Commit as the status.
3. For existing unmanaged Skills, create a `manage` plan, show detected source
   evidence, confidence, file snapshot and scan findings, then apply only the
   exact confirmed names.
4. For GitHub or local installation, create a plan first. Show the resolved
   repository commit, discovered skills and scan findings.
5. When the user explicitly requests assisted installation for a complex
   repository, use `install --assist` to create a separate Codex plan. Present
   its repository summary, requirements, warnings, typed steps, exact Skills,
   permission IDs, manual work, and project-root requirement. Do not apply it in
   the same command.
6. Ask for confirmation of the exact selected skill names. Keep active High and
   Critical clusters blocked until the user explicitly ignores them. Offer one
   cluster or report-wide human decisions for every severity and deterministic
   rule. Reasons are optional; do not add a separate High acceptance or
   deterministic confirmation step.
7. Apply a confirmed assisted plan only with `--assist --plan-id ... --apply`,
   repeated exact `--skill` and `--grant` values, and `--project-root` only when
   the plan requires MCP. Otherwise apply the standard plan. Return the
   transaction ID, recovery status, and report location.
8. For removal, use `csm remove` with explicit names. Explain that content is
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
- Never modify `.system`.
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
  assisted-plan snapshots.
- A GUI one-click decision may atomically ignore all active clusters without
  Codex review or a mandatory reason. Preserve explicit cluster targets and a
  recovery path in the transaction journal.
- Use an isolated configuration for tests.

Read [references/cli-contract.md](references/cli-contract.md) before executing
mutating commands.
