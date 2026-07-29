# Desktop GUI guide

## Overview

Shows managed, unmanaged, and system Skill counts, active high-risk findings, source groups, and recent transactions.

## Skills

Search and inspect complete source, status, and version information. Hover or focus a cell to see untruncated values. Use select all, invert, or clear. Selected unmanaged Skills can be analyzed and managed; selected personal Skills can be moved to quarantine.

## Groups

Source groups are detected automatically and remain the authority for updates. Display groups may be created, renamed, reordered, and populated by dragging Skills. These changes are journaled and reversible.

## Update Center

The page shows the last check time and a clear status for every source. Select one or many available sources, then review the immutable commit, exact target files, security results, and Skill selection for each source.

Each source is applied as an independent transaction with its own backup and rollback entry. A failure in one source is reported without hiding the result of completed sources.

## Security Center

Skills with no trusted scan record or changed content are selected by default. Checked and unchanged Skills are skipped by default, but can be added with checkboxes, Select all, or Invert. Results and counts follow Group → Skill → Warning. Groups, Skill details, and Codex conclusions are collapsed by default so large reports remain readable.

Repeated scans do not inflate the badge: it counts unique active High/Critical findings from the latest effective report per target. Every severity and deterministic rule can be ignored individually or all at once, without running Codex first or entering a mandatory reason.

Active High/Critical findings block installation and update. A human ignore action clears the applicable gate; restoring a cluster immediately reactivates it. Optional Codex review reads the complete target directory in a read-only session and treats local rule hits as supplemental leads.

## History, quarantine, and reports

History lists mutations and offers rollback when a backup exists. Quarantine restores removed Skills without permanent deletion. Reports provide human-readable and structured audit records.

## Settings

Language supports Simplified Chinese and English. Chinese is used on first run
and for older configurations with no saved language. Choosing a language updates
the interface immediately, saves it automatically, and restores it on the next
launch. Paths, rule names, logs, and user-defined Skill or group content remain
unchanged.

Configure absolute paths, scheduled read-only checks, private GitHub
credentials, optional Codex review, and diagnostics. When the CLI path is
blank, the application skips unusable WindowsApps candidates and probes PATH
plus the current user's npm directory for a working independent CLI. It checks
required capabilities instead of pinning a CLI version and refreshes status
when focus returns after browser authentication. Once authenticated, the model
picker loads the current CLI's visible model catalog instead of using a
hard-coded list. Each application group is one review task, so Skills in the same
group keep shared context. Groups run serially by default; higher concurrency can
cause CLI model-refresh contention or rate limiting. Local rule input is a count-only
overview; Codex reads evidence from the repository. Reviews return a separate summary
for every Skill, with live group and Skill progress. Switching pages keeps the task
and result alive. A failed or incomplete group is retried once serially. Codex review
requires an installed, signed-in Codex CLI and consumes account usage. Background CLI probes run
without flashing console windows. Tokens
are stored in Windows Credential Manager and are never written to logs or
reports.
