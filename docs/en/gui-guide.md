# Desktop GUI guide

## Overview

Shows managed, unmanaged, and system Skill counts, active high-risk findings, source groups, and recent transactions.

## Skills

Search and inspect complete source, status, and version information. Hover or focus a cell to see untruncated values. Use select all, invert, or clear. Selected unmanaged Skills can be analyzed and managed; selected personal Skills can be moved to quarantine.

## Install Skill

The installation dialog keeps source input, analysis, permissions, execution
progress, and errors in one workflow. GitHub 403 responses, invalid input,
staging failures, and scan errors appear inside the dialog with an appropriate
retry path; you do not need to close it to find a global notification. A rate
limit shows its reset time and countdown, blocks premature retries, and links
to credential settings.

If different same-name Skills are present in separate repository directories,
the dialog lists the conflicting paths. When a `skills-codex` mirror is
detected, **Use suggested Codex directory** retries with that subtree without
requiring the user to rebuild the URL.

**Standard installation** writes only the explicitly selected Skills.
**Codex assisted installation** first performs the same GitHub commit pinning
or local-source validation and local scan. It then runs a reusable read-only
project scan: bounded summaries cover the eligible text inventory and a
deterministic focus set receives deeper analysis. Credential-like files are
metadata-only and large text is truncated. Codex first returns a project
overview, security conclusion, evidence limitations, and declarative
installation methods. It uses
the configured model and reasoning effort, consumes Codex usage, and treats the
local risk overview only as supplemental context.

The scan does not create a plan, download dependencies, or authorize
installation. The user must review it and choose **Approve and create
installation plan** before Codex can propose typed steps and permissions.

Codex text is never executed as a command. Automatic steps are restricted to
installing selected Skills, installing a repository-matched exact-version Python
tool from official PyPI, and adding a Codex MCP entry for that managed tool.
Managed-tool and MCP automation requires a GitHub source so package ownership
can be verified. Everything else remains a manual instruction. Approved
automatic steps run first, and the result reports `partial` with the remaining
manual work. MCP setup requires a real Git or SVN target project and will not
replace an existing same-name MCP entry.

When a Python tool is proposed, plan creation downloads its complete Wheel closure
from official PyPI into isolated staging and displays the locked filenames and
SHA256 values. Nothing is installed or run during analysis. Source
distributions are rejected; native-code Wheels are marked high risk and need a
separate approval. Apply installs only those locked files offline.

Every required permission must be approved before execution. The timeline shows
the current step, completed count, activity, and errors. Use the top-right
button to hide the dialog while work continues in the background; reopening it
restores the latest progress. A process interrupted by application exit is
reported explicitly instead of treating a source preview as an install plan.
When Codex CLI reports a failure event, the dialog shows its actual error
without storing normal model output or repository text. If no reliable plan can
be generated, the source check, risk result, and Skill selection remain
available for standard installation.
After a failed run, the approval view restores only the exact prior Skill
subset, permissions, and project root and requires another review before retry.
Failure or cancellation recovers reversible steps in reverse order. Partial
results keep their manual steps and rollback entry in History until recovery
finishes. If Codex is unavailable or cannot produce a reliable plan, preserve
the source analysis and Skill selection and switch to standard installation.

## Groups

Source groups are detected automatically and remain the authority for updates. Display groups may be created, renamed, reordered, and populated by dragging Skills. These changes are journaled and reversible.

## Update Center

The page shows the last check time and a clear status for every source. Select one or many available sources, then review the immutable commit, exact target files, security results, and Skill selection for each source.

Each source is applied as an independent transaction with its own backup and rollback entry. A failure in one source is reported without hiding the result of completed sources.

## Security Center

Skills with no trusted scan record or changed content are selected by default. Checked and unchanged Skills are skipped by default, but can be added with checkboxes, Select all, or Invert. Results and counts follow Group → Skill → Warning. Groups, Skill details, and Codex conclusions are collapsed by default so large reports remain readable.

Repeated scans do not inflate the badge: it counts unique active High/Critical findings from the latest effective report per target. Every severity and deterministic rule can be ignored individually or all at once, without running Codex first or entering a mandatory reason.

Active High/Critical findings block installation and update. A human ignore action clears the applicable gate; restoring a cluster immediately reactivates it. Optional Codex review packages the complete selected group with shell access disabled and treats local rule hits as supplemental leads.

## History, quarantine, and reports

History lists mutations and offers rollback when a backup exists. Quarantine restores removed Skills without permanent deletion. Reports provide human-readable and structured audit records.

## Settings

Language supports Simplified Chinese and English. Chinese is used on first run
and for older configurations with no saved language. Choosing a language updates
the interface immediately, saves it automatically, and restores it on the next
launch. Paths, rule names, logs, and user-defined Skill or group content remain
unchanged.

Configure absolute paths, scheduled read-only checks, private GitHub
credentials, optional Codex review, and diagnostics. The enable toggle controls
Security Center risk review only. Choosing Codex assisted installation in the
Install Skill dialog is the opt-in for that installation; it still uses the CLI
path, model, and reasoning effort configured here.

When the CLI path is blank, the application skips unusable WindowsApps
candidates and probes PATH plus the current user's npm directory for a working
independent CLI. It checks capabilities instead of pinning a CLI version and
refreshes status when focus returns after browser authentication. Once
authenticated, the model picker loads the current CLI's visible model catalog
instead of using a hard-coded list.

Each application group is one risk-review task, so Skills in the same group keep
shared context. Groups run serially by default; higher concurrency can cause CLI
model-refresh contention or rate limiting. Local rule input is a count-only
overview; Codex reads evidence from the repository. Reviews return a separate
summary for every Skill with live progress. Switching pages keeps the task and
result alive. A failed or incomplete group is retried once serially. Codex
requires an installed, signed-in CLI and consumes account usage. Background
probes run without flashing console windows. Tokens are stored in Windows
Credential Manager and never written to logs or reports.
